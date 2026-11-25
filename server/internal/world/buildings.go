package world

import (
	"sort"
	"strings"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// Building kinds with rules of their own. Every other building only blocks.
const (
	BuildingWorkshop  = "atelier"
	BuildingWell      = "puits"
	BuildingRack      = "sechoir"
	BuildingScarecrow = "epouvantail"
	BuildingFence     = "cloture"
)

// Reasons specific to building.
const (
	ReasonCannotBuild = "cannot_build" // the footprint is not free, or outside the domain
	ReasonNeedsPlan   = "needs_plan"   // another building must stand first (the workshop)
	ReasonCost        = "missing_materials"
	ReasonBadKind     = "unknown_building"
	ReasonOccupied    = "occupied" // a tamed creature lives there
)

// Building is something a player built in the domain.
type Building struct {
	ID    int
	Kind  string
	At    Point // top-left tile
	W, H  int
	HP    int    // fences: creatures break them
	Owner string // who built it; checked once visitors arrive (phase 4)
	Load  int    // drying rack: crops drying tonight
	Ready int    // drying rack: rations waiting
}

func (b *Building) tiles() []Point {
	out := make([]Point, 0, b.W*b.H)
	for y := b.At.Y; y < b.At.Y+b.H; y++ {
		for x := b.At.X; x < b.At.X+b.W; x++ {
			out = append(out, Point{x, y})
		}
	}
	return out
}

func (b *Building) view() protocol.BuildingView {
	return protocol.BuildingView{ID: b.ID, Kind: b.Kind, X: b.At.X, Y: b.At.Y, W: b.W, H: b.H, HP: b.HP, Load: b.Load, Ready: b.Ready}
}

func (d *Domain) buildingEvent(b *Building) Event {
	return Event{Msg: protocol.Building{T: protocol.TypeBuilding, Building: b.view()}}
}

func (d *Domain) addBuilding(b *Building) {
	if b.ID == 0 {
		d.nextBuildingID++
		b.ID = d.nextBuildingID
	} else if b.ID > d.nextBuildingID {
		d.nextBuildingID = b.ID
	}
	d.buildings[b.ID] = b
	for _, t := range b.tiles() {
		d.bAt[t] = b
	}
}

func (d *Domain) removeBuilding(b *Building) Event {
	delete(d.buildings, b.ID)
	for _, t := range b.tiles() {
		delete(d.bAt, t)
	}
	return Event{Msg: protocol.BuildingGone{T: protocol.TypeBuildingGone, ID: b.ID}}
}

func (d *Domain) hasBuilding(kind string) bool {
	for _, b := range d.buildings {
		if b.Kind == kind {
			return true
		}
	}
	return false
}

func (d *Domain) sortedBuildings() []*Building {
	out := make([]*Building, 0, len(d.buildings))
	for _, b := range d.buildings {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *Domain) buildingViews() []protocol.BuildingView {
	out := []protocol.BuildingView{}
	for _, b := range d.sortedBuildings() {
		out = append(out, b.view())
	}
	return out
}

// ---------- the domain's land ----------

// initClaim marks the starting domain: the map's "domaine" rectangle, or a
// 12 × 12 square around the bed (or the spawn).
func (d *Domain) initClaim() {
	m := d.Map
	d.claim = make([]bool, m.Width*m.Height)
	x0, y0, w, h := m.Domain.X, m.Domain.Y, m.DomainW, m.DomainH
	if w <= 0 || h <= 0 {
		c := m.Spawn
		if m.HasBed {
			c = m.Bed
		}
		x0, y0, w, h = c.X-6, c.Y-6, 12, 12
	}
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			d.setClaim(Point{x, y})
		}
	}
}

func (d *Domain) setClaim(p Point) {
	if d.Map.InBounds(p) {
		d.claim[p.Y*d.Map.Width+p.X] = true
	}
}

// Claimed reports whether p is part of the domain, where players may build.
func (d *Domain) Claimed(p Point) bool {
	return d.Map.InBounds(p) && d.claim[p.Y*d.Map.Width+p.X]
}

func (d *Domain) claimString() string {
	var b strings.Builder
	b.Grow(len(d.claim))
	for _, c := range d.claim {
		if c {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

func (d *Domain) restoreClaim(s string) {
	if len(s) != len(d.claim) {
		return // the map changed size: keep the starting domain
	}
	for i := range d.claim {
		d.claim[i] = d.claim[i] || s[i] == '1'
	}
}

// uproot pulls a tree stump out for good. When the stump is on the domain's
// border, the domain grows around it: clearing the forest is how it expands.
func (d *Domain) uproot(p *Player, r *ResourceNode) []Event {
	r.Removed = true
	delete(d.resAt, r.At)
	p.Bag["bois"]++
	events := []Event{
		{Msg: protocol.Resource{T: protocol.TypeResource, Resource: resourceView(r)}},
		d.bagEvent(p),
	}
	rad := d.content.Domain.UprootRadius
	near := false
	for y := r.At.Y - rad - 1; y <= r.At.Y+rad+1 && !near; y++ {
		for x := r.At.X - rad - 1; x <= r.At.X+rad+1; x++ {
			if d.Claimed(Point{x, y}) {
				near = true
				break
			}
		}
	}
	if !near {
		return events
	}
	grew := false
	for y := r.At.Y - rad; y <= r.At.Y+rad; y++ {
		for x := r.At.X - rad; x <= r.At.X+rad; x++ {
			if q := (Point{x, y}); d.Map.InBounds(q) && !d.Claimed(q) {
				d.setClaim(q)
				grew = true
			}
		}
	}
	events = append(events, d.gainXP(p, XPUproot)...)
	if grew {
		events = append(events,
			Event{Msg: protocol.Claim{T: protocol.TypeClaim, Claim: d.claimString()}},
			Event{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeUprooted}})
	}
	return events
}

// ---------- building ----------

// Build places a building whose top-left tile is at. The footprint must
// cover the tile the player faces, lie inside the domain and be free; the
// materials come out of the bag.
func (d *Domain) Build(id, kind string, at Point, now time.Time) Result {
	p := d.players[id]
	if res, ok := d.ready(p, now); !ok {
		return res
	}
	def, ok := d.content.Buildings[kind]
	if !ok {
		return Result{Reason: ReasonBadKind}
	}
	if def.Requires != "" && !d.hasBuilding(def.Requires) {
		return Result{Reason: ReasonNeedsPlan}
	}
	b := &Building{Kind: kind, At: at, W: def.W, H: def.H, HP: def.HP, Owner: p.ID}
	if b.HP > 0 && kind == BuildingFence {
		b.HP += p.mod(ModFence)
	}
	front := p.Pos.Step(p.Facing)
	covers := false
	for _, t := range b.tiles() {
		if t == front {
			covers = true
		}
		if !d.buildable(t) {
			return Result{Reason: ReasonCannotBuild}
		}
	}
	if !covers {
		return Result{Reason: ReasonCannotBuild}
	}
	cost := d.buildCost(p, kind)
	for item, n := range cost {
		if p.Bag[item] < n {
			return Result{Reason: ReasonCost}
		}
	}
	for item, n := range cost {
		p.Bag[item] -= n
	}
	d.addBuilding(b)
	events := []Event{d.buildingEvent(b), d.bagEvent(p)}
	events = append(events, d.gainXP(p, XPBuild)...)
	return d.afterAction(p, d.content.Combat.MoveTicks, now, UseCooldown, events)
}

// buildCost is what a building costs p: the Charpentier talents take a share
// off every material (at most half), never below one.
func (d *Domain) buildCost(p *Player, kind string) map[string]int {
	off := min(50, p.mod(ModBuildSaving))
	out := map[string]int{}
	for item, n := range d.content.Buildings[kind].Cost {
		out[item] = max(1, n-n*off/100)
	}
	return out
}

// buildable reports whether a building may cover t.
func (d *Domain) buildable(t Point) bool {
	if !d.Claimed(t) || d.Map.Blocked(t) || d.Map.Water(t) || d.resAt[t] != nil || d.plots[t] != nil || d.bAt[t] != nil {
		return false
	}
	if d.occupied(t) || d.creatureAt(t) != nil {
		return false
	}
	return true
}

// Dismantle takes down the building the player faces, giving its cost back.
func (d *Domain) Dismantle(id string, now time.Time) Result {
	p := d.players[id]
	if res, ok := d.ready(p, now); !ok {
		return res
	}
	b := d.bAt[p.Pos.Step(p.Facing)]
	if b == nil {
		return Result{Reason: ReasonNothing}
	}
	def := d.content.Buildings[b.Kind]
	if d.resident(b) != nil {
		return Result{Reason: ReasonOccupied}
	}
	if b.Kind == BuildingWorkshop {
		for _, other := range d.buildings {
			if other != b && d.content.Buildings[other.Kind].Requires == BuildingWorkshop {
				return Result{Reason: ReasonNeedsPlan} // the others depend on it
			}
		}
	}
	refund := def.Cost
	if owner := d.players[b.Owner]; owner != nil {
		refund = d.buildCost(owner, b.Kind) // what the builder paid, talents included
	}
	for item, n := range refund {
		p.Bag[item] += n
	}
	if b.Kind == BuildingRack {
		p.Bag["ration"] += b.Ready
	}
	events := []Event{d.removeBuilding(b), d.bagEvent(p)}
	return d.afterAction(p, d.content.Combat.MoveTicks, now, UseCooldown, events)
}

// useBuilding is what Use does facing a building, whatever is in hand.
// ok is false when the building has nothing to do with the item.
func (d *Domain) useBuilding(p *Player, b *Building, item string, now time.Time) (Result, bool) {
	switch b.Kind {
	case BuildingWorkshop:
		return Result{OK: true, Events: []Event{{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeWorkshop}}}}, true
	case BuildingWell:
		if item != "arrosoir" {
			return Result{}, false
		}
		p.Water = d.canSize(p)
		p.readyAt = now.Add(UseCooldown)
		return Result{OK: true, Events: []Event{d.statsEvent(p)}}, true
	case BuildingRack:
		return d.useRack(p, b, now), true
	}
	if d.isHabitat(b.Kind) && b.Ready > 0 {
		return d.collectProduce(p, b), true
	}
	return Result{}, false
}

// useRack collects the dried rations, or else hangs raw crops to dry tonight.
func (d *Domain) useRack(p *Player, b *Building, now time.Time) Result {
	if b.Ready > 0 {
		p.Bag["ration"] += b.Ready
		b.Ready = 0
		p.readyAt = now.Add(UseCooldown)
		return Result{OK: true, Events: []Event{d.buildingEvent(b), d.bagEvent(p),
			{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeDried}}}}
	}
	room := d.content.Buildings[b.Kind].Capacity - b.Load
	if room <= 0 {
		return Result{Reason: ReasonNothing}
	}
	// The plain crops go first, most plentiful first; rare ones are kept.
	type stack struct {
		item string
		n    int
	}
	var stacks []stack
	for _, c := range d.content.Crops {
		if c.Wild || c.Harvest == c.Seed || p.Bag[c.Harvest] <= 0 {
			continue
		}
		stacks = append(stacks, stack{c.Harvest, p.Bag[c.Harvest]})
	}
	sort.Slice(stacks, func(i, j int) bool {
		if stacks[i].n != stacks[j].n {
			return stacks[i].n > stacks[j].n
		}
		return stacks[i].item < stacks[j].item
	})
	moved := 0
	for _, s := range stacks {
		for p.Bag[s.item] > 0 && room > 0 {
			p.Bag[s.item]--
			room--
			moved++
		}
	}
	if moved == 0 {
		return Result{Reason: ReasonNoItem}
	}
	b.Load += moved
	p.readyAt = now.Add(UseCooldown)
	return Result{OK: true, Events: []Event{d.buildingEvent(b), d.bagEvent(p),
		{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeDrying}}}}
}

// dryRacks turns each rack's load into rations; it runs every morning.
func (d *Domain) dryRacks() {
	for _, b := range d.buildings {
		if b.Kind == BuildingRack && b.Load > 0 {
			b.Ready += b.Load
			b.Load = 0
		}
	}
}

// ---------- the house chest ----------

func (d *Domain) chestEvent(p *Player, open bool) Event {
	items := make(map[string]int, len(d.chest))
	for k, v := range d.chest {
		if v > 0 {
			items[k] = v
		}
	}
	return Event{To: p.ID, Msg: protocol.Chest{T: protocol.TypeChest, Items: items, Open: open}}
}

// ChestMove moves items between the bag and the house chest. What is in the
// chest is never lost when a player falls.
func (d *Domain) ChestMove(id, op, item string, count int, now time.Time) Result {
	p := d.players[id]
	if p == nil || !p.Online {
		return Result{Reason: ReasonNotInDomain}
	}
	if !d.Map.HasChest || p.Pos.Step(p.Facing) != d.Map.Chest || p.Sleeping {
		return Result{Reason: ReasonNothing}
	}
	from, to := p.Bag, d.chest
	if op == protocol.ChestTake {
		from, to = d.chest, p.Bag
	} else if op != protocol.ChestPut {
		return Result{Reason: ReasonNothing}
	}
	have := from[item]
	if have <= 0 {
		return Result{Reason: ReasonNoItem}
	}
	if count <= 0 || count > have {
		count = have
	}
	from[item] -= count
	if from[item] == 0 {
		delete(from, item)
	}
	to[item] += count
	return Result{OK: true, Events: []Event{d.bagEvent(p), d.chestEvent(p, false)}}
}
