// Package world holds the authoritative rules of a domain: who stands where,
// what can be walked on, what tools do, how crops grow and how time passes.
// It has no goroutines and no I/O, and takes the current time as an
// argument, so it is easy to test.
package world

import (
	"math/rand"
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
	MoveCooldown = 120 * time.Millisecond
	UseCooldown  = 250 * time.Millisecond
)

// Reasons an action is refused, sent back in protocol.Ack.
const (
	ReasonBlocked     = "blocked"
	ReasonTooFast     = "too_fast"
	ReasonNothing     = "nothing_there"
	ReasonBadDir      = "bad_direction"
	ReasonNotInDomain = "not_in_domain"
	ReasonWrongTool   = "wrong_tool" // e.g. the pickaxe on a tree
	ReasonExhausted   = "exhausted"  // not enough energy left
	ReasonEmptyCan    = "empty_can"  // the watering can needs refilling
	ReasonNoItem      = "no_item"    // the player does not carry that item
	ReasonAsleep      = "asleep"     // in bed: move to get up first
)

// Player is a character in the domain.
type Player struct {
	ID       string
	Name     string
	Pos      Point
	Facing   string
	Tools    []string       // equipped: kept on death
	Bag      map[string]int // lost on death
	Energy   int
	Water    int // in the watering can
	Online   bool
	Sleeping bool
	readyAt  time.Time
}

func (p *Player) view() protocol.PlayerView {
	return protocol.PlayerView{ID: p.ID, Name: p.Name, X: p.Pos.X, Y: p.Pos.Y, Facing: p.Facing, Sleeping: p.Sleeping}
}

func (p *Player) bagCopy() map[string]int {
	out := make(map[string]int, len(p.Bag))
	for k, v := range p.Bag {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

func (p *Player) hasTool(name string) bool {
	for _, t := range p.Tools {
		if t == name {
			return true
		}
	}
	return false
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
	content   *Content
	rng       *rand.Rand
	players   map[string]*Player
	resources []*ResourceNode
	resAt     map[Point]*ResourceNode
	plots     map[Point]*Plot

	minutes   float64   // game minutes since midnight before day 1
	weather   string    // today's weather
	lastTick  time.Time // real time the clock was last advanced to
	mornings  int       // mornings run since this Domain was created
	clockStep int       // last half-hour announced to clients
}

// NewDomain places the map's resources and returns an empty domain whose
// clock stands at the first morning. seed drives weather and mutations.
func NewDomain(m *Map, c *Content, seed int64) *Domain {
	d := &Domain{
		Map: m, content: c, rng: rand.New(rand.NewSource(seed)),
		players: map[string]*Player{}, resAt: map[Point]*ResourceNode{}, plots: map[Point]*Plot{},
		minutes: float64(c.Time.WakeHour * 60), weather: protocol.WeatherSun,
	}
	for i, s := range m.Resources {
		if _, taken := d.resAt[s.At]; taken {
			continue // two objects on one tile: keep the first
		}
		r := &ResourceNode{ID: i + 1, Kind: s.Kind, At: s.At, Hits: ResourceKinds[s.Kind].Hits}
		d.resources = append(d.resources, r)
		d.resAt[s.At] = r
	}
	d.clockStep = d.halfHour()
	return d
}

// Player returns a player by ID, or nil.
func (d *Domain) Player(id string) *Player { return d.players[id] }

// Mornings counts the days that have ended since the domain was created or
// restored. The instance saves when it changes.
func (d *Domain) Mornings() int { return d.mornings }

func (d *Domain) newPlayer(id string) *Player {
	p := &Player{ID: id, Pos: d.Map.Spawn, Facing: protocol.Down,
		Tools: append([]string(nil), d.content.StarterTools...), Bag: map[string]int{},
		Energy: d.content.Energy.Max, Water: d.content.WateringCanSize}
	for k, v := range d.content.StarterBag {
		p.Bag[k] = v
	}
	return p
}

// Join brings a player online, creating the character on first visit.
// A returning player keeps their position, tools and bag.
func (d *Domain) Join(id, name string, now time.Time) []Event {
	events := d.advanceClock(now) // time so far ran at the pace of who was here
	p := d.players[id]
	if p == nil {
		p = d.newPlayer(id)
		d.players[id] = p
	}
	if name != "" {
		p.Name = name
	}
	if !d.Walkable(p.Pos) {
		p.Pos = d.Map.Spawn // the map changed under a saved player
	}
	p.Online = true
	p.Sleeping = false
	return append(events, Event{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}})
}

// Leave takes a player offline. Their character stays in the domain.
func (d *Domain) Leave(id string, now time.Time) []Event {
	p := d.players[id]
	if p == nil || !p.Online {
		return nil
	}
	events := d.advanceClock(now)
	p.Online = false
	p.Sleeping = false
	events = append(events, Event{Msg: protocol.PlayerLeft{T: protocol.TypePlayerLeft, ID: id}})
	if d.everyoneAsleep() {
		events = append(events, d.sleepThrough(now)...)
	}
	return events
}

// Welcome builds the full state sent to a player who just joined.
func (d *Domain) Welcome(id, mapName string) protocol.Welcome {
	w := protocol.Welcome{T: protocol.TypeWelcome, Version: protocol.Version, You: id, Map: mapName,
		Players: []protocol.PlayerView{}, Resources: make([]protocol.ResourceView, 0, len(d.resources)),
		Plots: d.plotViews(), Clock: d.ClockView(), Tools: []string{}, Inventory: map[string]int{}}
	for _, p := range d.sortedPlayers() {
		if p.Online {
			w.Players = append(w.Players, p.view())
		}
	}
	for _, r := range d.resources {
		w.Resources = append(w.Resources, resourceView(r))
	}
	if p := d.players[id]; p != nil {
		w.Tools = append(w.Tools, p.Tools...)
		w.Inventory = p.bagCopy()
		w.Stats = d.stats(p)
	}
	return w
}

func (d *Domain) stats(p *Player) protocol.StatsView {
	return protocol.StatsView{Energy: p.Energy, MaxEnergy: d.content.Energy.Max, Water: p.Water, WaterMax: d.content.WateringCanSize}
}

// Walkable reports whether a player may stand on p. Plots never block.
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
// A sleeping player gets out of bed.
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
	changed := p.Facing != dir || p.Sleeping
	p.Facing = dir
	p.Sleeping = false
	target := p.Pos.Step(dir)
	if !d.Walkable(target) {
		res := Result{Reason: ReasonBlocked}
		if changed {
			res.Events = []Event{{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}}}
		}
		return res
	}
	p.Pos = target
	p.readyAt = now.Add(MoveCooldown)
	return Result{OK: true, Events: []Event{{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}}}}
}

// Use acts on the tile in front of the player with item (see protocol.Use).
func (d *Domain) Use(id, item string, now time.Time) Result {
	p := d.players[id]
	if p == nil || !p.Online {
		return Result{Reason: ReasonNotInDomain}
	}
	if p.Sleeping {
		return Result{Reason: ReasonAsleep}
	}
	if now.Before(p.readyAt) {
		return Result{Reason: ReasonTooFast}
	}
	front := p.Pos.Step(p.Facing)

	// Whatever is in hand, the bed and ripe crops answer first.
	if d.Map.HasBed && front == d.Map.Bed {
		return d.goToBed(p, now)
	}
	if plot := d.plots[front]; plot != nil && d.ripe(plot) {
		p.readyAt = now.Add(UseCooldown)
		return Result{OK: true, Events: d.harvest(p, plot)}
	}

	switch {
	case item == "":
		return Result{Reason: ReasonNothing}
	case p.hasTool(item):
		return d.useTool(p, item, front, now)
	case p.Bag[item] > 0 && d.content.CropForSeed(item) != "":
		return d.sow(p, item, front, now)
	default:
		return Result{Reason: ReasonNoItem}
	}
}

func (d *Domain) goToBed(p *Player, now time.Time) Result {
	p.Sleeping = true
	events := []Event{{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}}}
	if d.everyoneAsleep() {
		events = append(events, d.sleepThrough(now)...)
	} else {
		events = append(events, Event{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeWaiting}})
	}
	return Result{OK: true, Events: events}
}

func (d *Domain) useTool(p *Player, tool string, front Point, now time.Time) Result {
	cost := d.content.Tools[tool].Energy
	spend := func() []Event {
		p.Energy -= cost
		p.readyAt = now.Add(UseCooldown)
		return []Event{{To: p.ID, Msg: protocol.Stats{T: protocol.TypeStats, Stats: d.stats(p)}}}
	}
	// Refilling the can is free; everything else needs the energy for it.
	if tool == "arrosoir" && d.Map.Water(front) {
		p.Water = d.content.WateringCanSize
		p.readyAt = now.Add(UseCooldown)
		return Result{OK: true, Events: []Event{{To: p.ID, Msg: protocol.Stats{T: protocol.TypeStats, Stats: d.stats(p)}}}}
	}

	r := d.resAt[front]
	plot := d.plots[front]
	if r != nil && !r.Depleted {
		if r.def().Tool != tool {
			return Result{Reason: ReasonWrongTool}
		}
		if p.Energy < cost {
			return Result{Reason: ReasonExhausted}
		}
		return Result{OK: true, Events: append(spend(), d.strike(p, r, now)...)}
	}

	switch tool {
	case "houe":
		if !d.canTill(front) {
			return Result{Reason: ReasonNothing}
		}
		if p.Energy < cost {
			return Result{Reason: ReasonExhausted}
		}
		np := &Plot{At: front, State: protocol.PlotTilled}
		d.plots[front] = np
		return Result{OK: true, Events: append(spend(), d.plotEvent(np))}

	case "arrosoir":
		if plot == nil || plot.Watered || (plot.State != protocol.PlotTilled && plot.State != protocol.PlotCrop) {
			return Result{Reason: ReasonNothing}
		}
		if p.Water <= 0 {
			return Result{Reason: ReasonEmptyCan}
		}
		if p.Energy < cost {
			return Result{Reason: ReasonExhausted}
		}
		p.Water--
		plot.Watered = true
		return Result{OK: true, Events: append(spend(), d.plotEvent(plot))}

	case "faux":
		if plot == nil || (plot.State != protocol.PlotWild && plot.State != protocol.PlotWithered) {
			return Result{Reason: ReasonNothing}
		}
		if p.Energy < cost {
			return Result{Reason: ReasonExhausted}
		}
		events := spend()
		if plot.State == protocol.PlotWithered {
			// The dead crop is cut; the soil stays tilled.
			plot.State, plot.Crop, plot.Growth, plot.Neglect = protocol.PlotTilled, "", 0, 0
			return Result{OK: true, Events: append(events, d.plotEvent(plot))}
		}
		delete(d.plots, front)
		events = append(events, plotGoneEvent(front))
		return Result{OK: true, Events: append(events, d.mowYield(p)...)}
	}
	return Result{Reason: ReasonNothing}
}

func (d *Domain) canTill(at Point) bool {
	if !d.Map.Cultivable(at) || d.resAt[at] != nil || d.plots[at] != nil {
		return false
	}
	for _, p := range d.players {
		if p.Online && p.Pos == at {
			return false
		}
	}
	return true
}

func (d *Domain) sow(p *Player, seed string, front Point, now time.Time) Result {
	plot := d.plots[front]
	if plot == nil || plot.State != protocol.PlotTilled {
		return Result{Reason: ReasonNothing}
	}
	p.Bag[seed]--
	plot.State = protocol.PlotCrop
	plot.Crop = d.content.CropForSeed(seed)
	plot.Growth = 0
	plot.Neglect = 0
	p.readyAt = now.Add(UseCooldown)
	return Result{OK: true, Events: []Event{d.plotEvent(plot), d.bagEvent(p)}}
}

func (d *Domain) harvest(p *Player, plot *Plot) []Event {
	def := d.crop(plot)
	p.Bag[def.Harvest] += def.Amount
	if def.SeedChance > 0 && d.rng.Float64() < def.SeedChance {
		p.Bag[def.Seed]++
	}
	plot.State, plot.Crop, plot.Growth, plot.Neglect = protocol.PlotTilled, "", 0, 0
	return []Event{d.plotEvent(plot), d.bagEvent(p)}
}

// mowYield is what cutting weeds or tall grass gives: fibre, sometimes a seed.
func (d *Domain) mowYield(p *Player) []Event {
	p.Bag["fibre"]++
	m := d.content.Mowing
	if len(m.Seeds) > 0 && d.rng.Float64() < m.SeedChance {
		p.Bag[m.Seeds[d.rng.Intn(len(m.Seeds))]]++
	}
	return []Event{d.bagEvent(p)}
}

// strike hits a resource node with the right tool. When its last hit lands,
// the drop goes into the player's bag and the node starts regrowing.
func (d *Domain) strike(p *Player, r *ResourceNode, now time.Time) []Event {
	r.Hits--
	var events []Event
	if r.Hits <= 0 {
		def := r.def()
		r.Hits = 0
		r.Depleted = true
		r.RegrowAt = now.Add(def.Regrow)
		if def.Tool == "faux" {
			events = append(events, d.mowYield(p)...) // fibre, and sometimes a seed
		} else {
			p.Bag[def.Item] += def.Amount
			events = append(events, d.bagEvent(p))
		}
	}
	return append(events, Event{Msg: protocol.Resource{T: protocol.TypeResource, Resource: resourceView(r)}})
}

func (d *Domain) bagEvent(p *Player) Event {
	return Event{To: p.ID, Msg: protocol.Inventory{T: protocol.TypeInventory, Items: p.bagCopy()}}
}

// Tick moves the clock to now (running any mornings on the way), regrows
// resources whose time has come, and tells clients the time now and then.
func (d *Domain) Tick(now time.Time) []Event {
	events := d.advanceClock(now)
	for _, r := range d.resources {
		// A node does not regrow under a player's feet; it waits until the tile is free.
		if !r.Depleted || now.Before(r.RegrowAt) || d.occupied(r.At) {
			continue
		}
		r.Depleted = false
		r.Hits = r.def().Hits
		events = append(events, Event{Msg: protocol.Resource{T: protocol.TypeResource, Resource: resourceView(r)}})
	}
	if step := d.halfHour(); step != d.clockStep {
		d.clockStep = step
		events = append(events, d.clockEvent())
	}
	return events
}

func (d *Domain) halfHour() int { return int(d.minutes / 30) }

func (d *Domain) occupied(at Point) bool {
	for _, p := range d.players {
		if p.Online && p.Pos == at {
			return true
		}
	}
	return false
}

func (d *Domain) sortedPlayers() []*Player {
	out := make([]*Player, 0, len(d.players))
	for _, p := range d.players {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func resourceView(r *ResourceNode) protocol.ResourceView {
	return protocol.ResourceView{ID: r.ID, Kind: r.Kind, X: r.At.X, Y: r.At.Y, Hits: r.Hits, Depleted: r.Depleted}
}
