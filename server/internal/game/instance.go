// Package game runs domains: one goroutine per domain owns its state, and
// connections talk to it through a queue, so the rules never need locks.
package game

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
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

// Instance runs one domain.
type Instance struct {
	MapName string
	domain  *world.Domain
	cmds    chan func(time.Time)
	clients map[*Client]struct{}
	now     func() time.Time
	log     *slog.Logger
}

// NewInstance creates an instance for a loaded map. Call Run to start it.
func NewInstance(mapName string, m *world.Map, log *slog.Logger) *Instance {
	return &Instance{
		MapName: mapName,
		domain:  world.NewDomain(m),
		cmds:    make(chan func(time.Time), 256),
		clients: map[*Client]struct{}{},
		now:     time.Now,
		log:     log,
	}
}

// Run processes commands until ctx ends. The tick drives regrowth, which
// follows wall-clock time so the valley keeps living while nobody plays.
func (in *Instance) Run(ctx context.Context, tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-in.cmds:
			cmd(in.now())
		case <-t.C:
			in.deliver(in.domain.Tick(in.now()))
		}
	}
}

func (in *Instance) do(cmd func(time.Time)) { in.cmds <- cmd }

// Join registers a client and sends it the full state.
func (in *Instance) Join(c *Client, name string) {
	in.do(func(time.Time) {
		in.clients[c] = struct{}{}
		events := in.domain.Join(c.PlayerID, name)
		in.sendTo(c, in.domain.Welcome(c.PlayerID, in.MapName))
		in.deliver(events)
		in.log.Info("client joined", "player", c.PlayerID, "clients", len(in.clients))
	})
}

// Leave unregisters a client. The player goes offline with its last client.
func (in *Instance) Leave(c *Client) {
	in.do(func(time.Time) {
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
		in.deliver(in.domain.Leave(c.PlayerID))
		in.log.Info("player left", "player", c.PlayerID)
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
		case *protocol.Gather:
			seq, res = m.Seq, in.domain.Gather(c.PlayerID, now)
		default:
			in.sendTo(c, protocol.Error{T: protocol.TypeError, Message: "unexpected message"})
			return
		}
		in.sendTo(c, protocol.Ack{T: protocol.TypeAck, Seq: seq, OK: res.OK, Reason: res.Reason})
		in.deliver(res.Events)
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
