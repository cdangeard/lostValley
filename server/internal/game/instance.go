// Package game runs domains: one goroutine per domain owns its state, and
// connections talk to it through a queue, so the rules never need locks.
package game

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
	"github.com/cdangeard/lostvalley/server/internal/store"
	"github.com/cdangeard/lostvalley/server/internal/world"
)

// SendBuffer is how many messages may wait for a slow client before it is
// disconnected.
const SendBuffer = 128

// Client is one WebSocket connection. Several clients may drive the same
// player (two tabs of one browser share a player ID).
type Client struct {
	PlayerID string
	Send     chan []byte

	closeOnce sync.Once
	done      chan struct{}
}

// NewClient returns a client for playerID with an empty send queue.
func NewClient(playerID string) *Client {
	return &Client{PlayerID: playerID, Send: make(chan []byte, SendBuffer), done: make(chan struct{})}
}

// Done is closed when the instance drops the client (e.g. too slow).
func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) kick() { c.closeOnce.Do(func() { close(c.done) }) }

// AutosaveEvery is how often a valley that changed is saved, on top of the
// saves each morning, when the last player leaves and at shutdown.
const AutosaveEvery = time.Minute

// Config describes the domain an instance runs.
type Config struct {
	DomainID string
	MapName  string
	Map      *world.Map
	Content  *world.Content
	Store    store.Store
	Log      *slog.Logger
	Now      func() time.Time // defaults to time.Now
}

// Instance runs one domain.
type Instance struct {
	MapName  string
	domainID string
	domain   *world.Domain
	store    store.Store
	cmds     chan func(time.Time)
	clients  map[*Client]struct{}
	now      func() time.Time
	log      *slog.Logger

	dirty    bool // changed since the last save
	mornings int  // domain.Mornings() at the last save
	saves    chan *world.Snapshot
	saved    chan struct{}
}

// Open loads the domain from the store (or starts a new one) and returns an
// instance ready to Run. Time spent while the server was down is caught up
// at the offline pace on the first tick.
func Open(ctx context.Context, cfg Config) (*Instance, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	in := &Instance{
		MapName:  cfg.MapName,
		domainID: cfg.DomainID,
		domain:   world.NewDomain(cfg.Map, cfg.Content, cfg.Now().UnixNano()),
		store:    cfg.Store,
		cmds:     make(chan func(time.Time), 256),
		clients:  map[*Client]struct{}{},
		now:      cfg.Now,
		log:      cfg.Log,
		saves:    make(chan *world.Snapshot, 1),
		saved:    make(chan struct{}),
	}
	snap, err := cfg.Store.Load(ctx, cfg.DomainID)
	if err != nil {
		return nil, fmt.Errorf("open domain %s: %w", cfg.DomainID, err)
	}
	if snap != nil {
		in.domain.Restore(snap)
		in.log.Info("domain loaded", "domain", cfg.DomainID, "saved_at", snap.SavedAt, "players", len(snap.Players))
	} else {
		in.log.Info("new domain", "domain", cfg.DomainID)
	}
	return in, nil
}

// Run processes commands until ctx ends, then saves one last time. The tick
// drives the clock and regrowth, which follow wall-clock time so the valley
// keeps living while nobody plays.
func (in *Instance) Run(ctx context.Context, tick time.Duration) {
	go in.saveLoop()
	defer func() {
		in.save(in.now())
		close(in.saves)
		<-in.saved
	}()
	t := time.NewTicker(tick)
	defer t.Stop()
	autosave := time.NewTicker(AutosaveEvery)
	defer autosave.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-in.cmds:
			now := in.now()
			cmd(now)
			in.saveAfterMorning(now)
		case <-t.C:
			now := in.now()
			in.deliver(in.domain.Tick(now))
			in.saveAfterMorning(now)
		case <-autosave.C:
			if in.dirty {
				in.save(in.now())
			}
		}
	}
}

// saveAfterMorning saves when a day has ended: sleeping saves the game.
func (in *Instance) saveAfterMorning(now time.Time) {
	if in.domain.Mornings() != in.mornings {
		in.save(now)
	}
}

// save snapshots the domain and hands it to the save loop, which writes it
// without holding up the game.
func (in *Instance) save(now time.Time) {
	snap, events := in.domain.Snapshot(now)
	in.deliver(events)
	in.mornings = in.domain.Mornings()
	in.dirty = false
	// Keep only the newest snapshot if the store is slow.
	select {
	case in.saves <- snap:
		return
	default:
	}
	select {
	case <-in.saves:
	default:
	}
	in.saves <- snap
}

func (in *Instance) saveLoop() {
	defer close(in.saved)
	for snap := range in.saves {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := in.store.Save(ctx, in.domainID, snap)
		cancel()
		if err != nil {
			in.log.Error("save domain", "domain", in.domainID, "err", err)
			continue
		}
		in.log.Debug("domain saved", "domain", in.domainID, "minutes", snap.Domain.Minutes)
	}
}

func (in *Instance) do(cmd func(time.Time)) { in.cmds <- cmd }

// Join registers a client and sends it the full state.
func (in *Instance) Join(c *Client, name string) {
	in.do(func(now time.Time) {
		in.clients[c] = struct{}{}
		events := in.domain.Join(c.PlayerID, name, now)
		in.dirty = true
		in.sendTo(c, in.domain.Welcome(c.PlayerID, in.MapName))
		in.deliver(events)
		in.log.Info("client joined", "player", c.PlayerID, "clients", len(in.clients))
	})
}

// Leave unregisters a client. The player goes offline with its last client.
func (in *Instance) Leave(c *Client) {
	in.do(func(now time.Time) {
		if _, ok := in.clients[c]; !ok {
			return
		}
		delete(in.clients, c)
		c.kick()
		for other := range in.clients {
			if other.PlayerID == c.PlayerID {
				return // another tab still drives this player
			}
		}
		in.deliver(in.domain.Leave(c.PlayerID, now))
		in.log.Info("player left", "player", c.PlayerID)
		in.save(now)
	})
}

// Handle applies a decoded client message.
func (in *Instance) Handle(c *Client, msg any) {
	in.do(func(now time.Time) {
		if _, ok := in.clients[c]; !ok {
			return
		}
		var seq int
		var res world.Result
		switch m := msg.(type) {
		case *protocol.Move:
			seq, res = m.Seq, in.domain.Move(c.PlayerID, m.Dir, now)
		case *protocol.Use:
			seq, res = m.Seq, in.domain.Use(c.PlayerID, m.Item, now)
		case *protocol.Wait:
			seq, res = m.Seq, in.domain.Wait(c.PlayerID, now)
		case *protocol.Build:
			seq, res = m.Seq, in.domain.Build(c.PlayerID, m.Kind, world.Point{X: m.X, Y: m.Y}, now)
		case *protocol.Dismantle:
			seq, res = m.Seq, in.domain.Dismantle(c.PlayerID, now)
		case *protocol.ChestMove:
			seq, res = m.Seq, in.domain.ChestMove(c.PlayerID, m.Op, m.Item, m.Count, now)
		default:
			in.sendTo(c, protocol.Error{T: protocol.TypeError, Message: "unexpected message"})
			return
		}
		in.sendTo(c, protocol.Ack{T: protocol.TypeAck, Seq: seq, OK: res.OK, Reason: res.Reason})
		in.deliver(res.Events)
		if res.OK {
			in.dirty = true
		}
	})
}

// deliver sends events to their recipients.
func (in *Instance) deliver(events []world.Event) {
	for _, e := range events {
		raw, err := json.Marshal(e.Msg)
		if err != nil {
			in.log.Error("marshal event", "err", err)
			continue
		}
		for c := range in.clients {
			if e.To != "" && c.PlayerID != e.To {
				continue
			}
			in.push(c, raw)
		}
	}
}

func (in *Instance) sendTo(c *Client, msg any) {
	raw, err := json.Marshal(msg)
	if err != nil {
		in.log.Error("marshal message", "err", err)
		return
	}
	in.push(c, raw)
}

// push queues raw for c without ever blocking the domain loop. A client whose
// queue is full is dropped; its transport sees Done and closes the socket.
func (in *Instance) push(c *Client, raw []byte) {
	select {
	case c.Send <- raw:
	default:
		in.log.Warn("client too slow, dropping", "player", c.PlayerID)
		c.kick()
	}
}
