// Package world holds the authoritative rules of a domain: who stands where,
// what can be walked on, and what gathering yields. It has no goroutines and
// no I/O, and takes the current time as an argument, so it is easy to test.
package world

import (
	"sort"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// Point is a tile coordinate.
type Point struct{ X, Y int }

// Step returns p moved one tile in dir.
func (p Point) Step(dir string) Point {
	switch dir {
	case protocol.Up:
		return Point{p.X, p.Y - 1}
	case protocol.Down:
		return Point{p.X, p.Y + 1}
	case protocol.Left:
		return Point{p.X - 1, p.Y}
	case protocol.Right:
		return Point{p.X + 1, p.Y}
	}
	return p
}

// Minimum time between two actions of the same player. The client paces
// itself slightly slower, so honest clients never hit these.
const (
	MoveCooldown   = 120 * time.Millisecond
	GatherCooldown = 250 * time.Millisecond
)

// Reasons an action is refused, sent back in protocol.Ack.
const (
	ReasonBlocked     = "blocked"
	ReasonTooFast     = "too_fast"
	ReasonNothing     = "nothing_there"
	ReasonBadDir      = "bad_direction"
	ReasonNotInDomain = "not_in_domain"
)

// Player is a character in the domain.
type Player struct {
	ID        string
	Name      string
	Pos       Point
	Facing    string
	Inventory map[string]int
	Online    bool
	readyAt   time.Time
}

func (p *Player) view() protocol.PlayerView {
	return protocol.PlayerView{ID: p.ID, Name: p.Name, X: p.Pos.X, Y: p.Pos.Y, Facing: p.Facing}
}

func (p *Player) inventoryCopy() map[string]int {
	out := make(map[string]int, len(p.Inventory))
	for k, v := range p.Inventory {
		out[k] = v
	}
	return out
}

// Event is a message to deliver after an action. To is a player ID, or empty
// for everyone online in the domain.
type Event struct {
	To  string
	Msg any
}

// Result is the outcome of a player action.
type Result struct {
	OK     bool
	Reason string
	Events []Event
}

// Domain is one valley instance and everything in it.
type Domain struct {
	Map       *Map
	players   map[string]*Player
	resources []*ResourceNode
	resAt     map[Point]*ResourceNode
}

// NewDomain places the map's resources and returns an empty domain.
func NewDomain(m *Map) *Domain {
	d := &Domain{Map: m, players: map[string]*Player{}, resAt: map[Point]*ResourceNode{}}
	for i, s := range m.Resources {
		if _, taken := d.resAt[s.At]; taken {
			continue // two objects on one tile: keep the first
		}
		r := &ResourceNode{ID: i + 1, Kind: s.Kind, At: s.At, Hits: ResourceKinds[s.Kind].Hits}
		d.resources = append(d.resources, r)
		d.resAt[s.At] = r
	}
	return d
}

// Player returns a player by ID, or nil.
func (d *Domain) Player(id string) *Player { return d.players[id] }

// Join brings a player online, creating the character on first visit.
// A returning player keeps their position and bag.
func (d *Domain) Join(id, name string) []Event {
	p := d.players[id]
	if p == nil {
		p = &Player{ID: id, Name: name, Pos: d.Map.Spawn, Facing: protocol.Down, Inventory: map[string]int{}}
		d.players[id] = p
	}
	if name != "" {
		p.Name = name
	}
	p.Online = true
	return []Event{{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}}}
}

// Leave takes a player offline. Their character stays in memory.
func (d *Domain) Leave(id string) []Event {
	p := d.players[id]
	if p == nil || !p.Online {
		return nil
	}
	p.Online = false
	return []Event{{Msg: protocol.PlayerLeft{T: protocol.TypePlayerLeft, ID: id}}}
}

// Welcome builds the full state sent to a player who just joined.
func (d *Domain) Welcome(id, mapName string) protocol.Welcome {
	w := protocol.Welcome{T: protocol.TypeWelcome, Version: protocol.Version, You: id, Map: mapName,
		Players: []protocol.PlayerView{}, Resources: make([]protocol.ResourceView, 0, len(d.resources))}
	for _, p := range d.players {
		if p.Online {
			w.Players = append(w.Players, p.view())
		}
	}
	sort.Slice(w.Players, func(i, j int) bool { return w.Players[i].ID < w.Players[j].ID })
	for _, r := range d.resources {
		w.Resources = append(w.Resources, resourceView(r))
	}
	if p := d.players[id]; p != nil {
		w.Inventory = p.inventoryCopy()
	}
	return w
}

// Walkable reports whether a player may stand on p.
func (d *Domain) Walkable(p Point) bool {
	if d.Map.Blocked(p) {
		return false
	}
	if r := d.resAt[p]; r != nil && r.blocks() {
		return false
	}
	return true
}

// Move steps the player one tile. Into an obstacle, the player only turns.
func (d *Domain) Move(id, dir string, now time.Time) Result {
	p := d.players[id]
	if p == nil || !p.Online {
		return Result{Reason: ReasonNotInDomain}
	}
	if !protocol.ValidDir(dir) {
		return Result{Reason: ReasonBadDir}
	}
	if now.Before(p.readyAt) {
		return Result{Reason: ReasonTooFast}
	}
	turned := p.Facing != dir
	p.Facing = dir
	target := p.Pos.Step(dir)
	if !d.Walkable(target) {
		res := Result{Reason: ReasonBlocked}
		if turned {
			res.Events = []Event{{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}}}
		}
		return res
	}
	p.Pos = target
	p.readyAt = now.Add(MoveCooldown)
	return Result{OK: true, Events: []Event{{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}}}}
}

// Gather strikes the resource in front of the player. When its last hit
// lands, the drop goes into the player's bag and the node starts regrowing.
func (d *Domain) Gather(id string, now time.Time) Result {
	p := d.players[id]
	if p == nil || !p.Online {
		return Result{Reason: ReasonNotInDomain}
	}
	if now.Before(p.readyAt) {
		return Result{Reason: ReasonTooFast}
	}
	r := d.resAt[p.Pos.Step(p.Facing)]
	if r == nil || r.Depleted {
		return Result{Reason: ReasonNothing}
	}
	p.readyAt = now.Add(GatherCooldown)
	r.Hits--
	events := []Event{}
	if r.Hits <= 0 {
		def := r.def()
		r.Hits = 0
		r.Depleted = true
		r.RegrowAt = now.Add(def.Regrow)
		p.Inventory[def.Item] += def.Amount
		events = append(events, Event{To: p.ID, Msg: protocol.Inventory{T: protocol.TypeInventory, Items: p.inventoryCopy()}})
	}
	events = append(events, Event{Msg: protocol.Resource{T: protocol.TypeResource, Resource: resourceView(r)}})
	return Result{OK: true, Events: events}
}

// Tick regrows depleted resources whose time has come. A node does not
// regrow under a player's feet; it waits until the tile is free.
func (d *Domain) Tick(now time.Time) []Event {
	var events []Event
	for _, r := range d.resources {
		if !r.Depleted || now.Before(r.RegrowAt) || d.occupied(r.At) {
			continue
		}
		r.Depleted = false
		r.Hits = r.def().Hits
		events = append(events, Event{Msg: protocol.Resource{T: protocol.TypeResource, Resource: resourceView(r)}})
	}
	return events
}

func (d *Domain) occupied(at Point) bool {
	for _, p := range d.players {
		if p.Online && p.Pos == at {
			return true
		}
	}
	return false
}

func resourceView(r *ResourceNode) protocol.ResourceView {
	return protocol.ResourceView{ID: r.ID, Kind: r.Kind, X: r.At.X, Y: r.At.Y, Hits: r.Hits, Depleted: r.Depleted}
}
