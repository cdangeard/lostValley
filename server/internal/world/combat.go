package world

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// Combat "au pas" (step by step).
//
// The domain keeps a combat clock d.ct counted in ticks. Every creature acts
// when the clock reaches its readyAt, and every action costs ticks (a step,
// a swing of the axe…), so fast creatures act more often than slow ones.
//
// While no player is au pas, the clock follows real time (one tick every
// tickTempsReelMs): creatures at the forest edge wander and the night's
// pests walk to the fields on their own. As soon as a hostile creature comes
// within rayonEngagement tiles of a player, that player is au pas: the clock
// only moves when every player au pas has used up their turn, then jumps
// straight to the next actor. A creature first marks the tiles it is about
// to hit (Warn) and strikes them on its next turn, which gives players time
// to step aside. With two players au pas, the one who takes longer than
// delaiTourSecondes to act passes their turn, so nobody blocks the other.

// ReasonNotYourTurn refuses an action from a player au pas before their turn.
const ReasonNotYourTurn = "not_your_turn"

// Creature is one live creature in the domain.
type Creature struct {
	ID       int
	Kind     string
	Pos      Point
	Facing   string
	HP       int
	Armor    int
	Burrowed bool
	Wave     bool // came with the night wave: goes for the crops, runs off at dawn
	Stunned  bool
	Warn     []Point // tiles the next blow will hit
	warnDmg  int
	readyAt  int

	// Taming (see taming.go).
	Fauna      bool    // wild creature that can be tamed
	Trust      int     // wild: trust won so far
	Submissive bool    // a hostile one that gave in: it stops fighting and can be fed
	Owner      string  // tamed: the player it is bound to
	Mode       string  // tamed: protocol.ModeFollow or ModeHome
	Home       int     // tamed: ID of its habitat building, 0 if it has none
	Resting    bool    // tamed and worn out: home until morning
	Fed        bool    // tamed: fed since the last morning
	ateAt      float64 // game minute of its last meal
}

// hostile reports whether c attacks players and puts them au pas.
func (d *Domain) hostile(c *Creature) bool {
	return c.Owner == "" && !c.Submissive && d.cdef(c).Behavior != BehaviorPeaceful
}

func (d *Domain) cdef(c *Creature) CreatureDef { return d.content.Creatures[c.Kind] }

func (d *Domain) creatureView(c *Creature) protocol.CreatureView {
	v := protocol.CreatureView{ID: c.ID, Kind: c.Kind, X: c.Pos.X, Y: c.Pos.Y, Facing: c.Facing,
		HP: c.HP, MaxHP: d.creatureMaxHP(c), Armor: c.Armor, Burrowed: c.Burrowed, Stunned: c.Stunned, Wave: c.Wave,
		Peaceful: d.cdef(c).Behavior == BehaviorPeaceful, Submissive: c.Submissive, Trust: c.Trust,
		Owner: c.Owner, Mode: c.Mode, Resting: c.Resting}
	if d.cdef(c).Taming != nil && c.Owner == "" {
		v.TrustMax = d.content.Taming.MaxTrust
	}
	if o := d.players[c.Owner]; o != nil {
		v.OwnerName = o.Name
	}
	for _, t := range c.Warn {
		v.Warn = append(v.Warn, protocol.Tile{X: t.X, Y: t.Y})
	}
	return v
}

func (d *Domain) creatureEvent(c *Creature) Event {
	return Event{Msg: protocol.Creature{T: protocol.TypeCreature, Creature: d.creatureView(c)}}
}

func (d *Domain) sortedCreatures() []*Creature {
	out := make([]*Creature, 0, len(d.creatures))
	for _, c := range d.creatures {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *Domain) creatureViews() []protocol.CreatureView {
	out := []protocol.CreatureView{}
	for _, c := range d.sortedCreatures() {
		out = append(out, d.creatureView(c))
	}
	return out
}

func (d *Domain) creatureAt(p Point) *Creature {
	for _, c := range d.creatures {
		if c.Pos == p {
			return c
		}
	}
	return nil
}

// spawn puts a new creature at p.
func (d *Domain) spawn(kind string, p Point, wave bool) (*Creature, Event) {
	def := d.content.Creatures[kind]
	d.nextCreatureID++
	c := &Creature{ID: d.nextCreatureID, Kind: kind, Pos: p, Facing: protocol.Down, HP: def.HP, Armor: def.Armor,
		Wave: wave, Burrowed: def.Trait == TraitBurrow, readyAt: d.ct + 1 + d.rng.Intn(def.MoveTicks+1)}
	d.creatures[c.ID] = c
	return c, d.creatureEvent(c)
}

func (d *Domain) removeCreature(c *Creature, how string) Event {
	delete(d.creatures, c.ID)
	return Event{Msg: protocol.CreatureGone{T: protocol.TypeCreatureGone, ID: c.ID, How: how}}
}

// cheb is the distance in king moves.
func cheb(a, b Point) int {
	dx, dy := a.X-b.X, a.Y-b.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

// ---------- who is au pas ----------

func (d *Domain) awake(p *Player) bool { return p.Online && !p.Sleeping }

// engaged reports whether p is au pas: a creature is close.
func (d *Domain) engaged(p *Player) bool {
	if !d.awake(p) {
		return false
	}
	r := d.content.Combat.EngageRadius
	for _, c := range d.creatures {
		if d.hostile(c) && cheb(c.Pos, p.Pos) <= r {
			return true
		}
	}
	return false
}

func (d *Domain) engagedPlayers() []*Player {
	var out []*Player
	for _, p := range d.sortedPlayers() {
		if d.engaged(p) {
			out = append(out, p)
		}
	}
	return out
}

// turnEvents tells each player whose combat state changed.
func (d *Domain) turnEvents() []Event {
	var events []Event
	for _, p := range d.sortedPlayers() {
		if !p.Online {
			continue
		}
		tv := d.turnView(p)
		if tv != p.turn {
			p.turn = tv
			events = append(events, Event{To: p.ID, Msg: protocol.Turn{T: protocol.TypeTurn, Turn: tv}})
		}
	}
	return events
}

func (d *Domain) turnView(p *Player) protocol.TurnView {
	if !d.engaged(p) {
		return protocol.TurnView{}
	}
	return protocol.TurnView{Engaged: true, Ready: p.turnAt <= d.ct, Tick: d.ct}
}

// ready checks that p may act now. engaged tells the caller the action is
// taken au pas and must spend combat ticks.
func (d *Domain) ready(p *Player, now time.Time) (res Result, ok bool) {
	switch {
	case p == nil || !p.Online:
		return Result{Reason: ReasonNotInDomain}, false
	case p.Sleeping:
		return Result{Reason: ReasonAsleep}, false
	case now.Before(p.readyAt):
		return Result{Reason: ReasonTooFast}, false
	case d.engaged(p) && p.turnAt > d.ct:
		return Result{Reason: ReasonNotYourTurn}, false
	}
	return Result{}, true
}

// afterAction finishes an accepted action: the real-time cooldown, the
// combat ticks if the player is au pas, and the creatures' answer.
func (d *Domain) afterAction(p *Player, ticks int, now time.Time, cooldown time.Duration, events []Event) Result {
	p.readyAt = now.Add(cooldown)
	if d.engaged(p) || p.turn.Engaged {
		p.turnAt = d.ct + ticks
		d.waitSince = now
	}
	return Result{OK: true, Events: append(events, d.runCombat(now)...)}
}

// Wait spends one combat tick doing nothing.
func (d *Domain) Wait(id string, now time.Time) Result {
	p := d.players[id]
	if res, ok := d.ready(p, now); !ok {
		return res
	}
	if !d.engaged(p) {
		return Result{Reason: ReasonNothing}
	}
	return d.afterAction(p, 1, now, MoveCooldown, nil)
}

// ---------- the combat clock ----------

// runCombat lets every creature whose time has come act, and moves the
// clock on while no player au pas is left to play.
func (d *Domain) runCombat(now time.Time) []Event {
	var events []Event
	for guard := 0; guard < 1000; guard++ {
		acted := false
		for _, c := range d.sortedCreatures() {
			if d.creatures[c.ID] == c && c.readyAt <= d.ct {
				events = append(events, d.act(c)...)
				acted = true
			}
		}
		if acted {
			continue
		}
		eng := d.engagedPlayers()
		if len(eng) == 0 {
			break // real time: Tick moves the clock
		}
		next := math.MaxInt
		waiting := false
		for _, p := range eng {
			if p.turnAt <= d.ct {
				waiting = true
			}
			next = min(next, p.turnAt)
		}
		if waiting {
			break
		}
		for _, c := range d.creatures {
			next = min(next, c.readyAt)
		}
		d.ct = max(next, d.ct+1)
		d.waitSince = now
	}
	if d.pendingSleep {
		d.pendingSleep = false
		if d.everyoneAsleep() {
			events = append(events, d.sleepThrough(now)...)
		}
	}
	return append(events, d.turnEvents()...)
}

// tickCombat runs the real-time side of combat from Tick: the clock while
// nobody is au pas, and the turn timer when two players are.
func (d *Domain) tickCombat(now time.Time) []Event {
	if len(d.creatures) == 0 {
		d.rtNext = time.Time{}
		return nil
	}
	eng := d.engagedPlayers()
	if len(eng) == 0 {
		step := time.Duration(d.content.Combat.RealTickMs) * time.Millisecond
		if d.rtNext.IsZero() || now.Sub(d.rtNext) > 4*step {
			d.rtNext = now // do not replay a long pause all at once
		}
		var events []Event
		for !now.Before(d.rtNext) {
			d.ct++
			d.rtNext = d.rtNext.Add(step)
			events = append(events, d.runCombat(now)...)
			if len(d.engagedPlayers()) > 0 {
				d.waitSince = now
				break
			}
		}
		return events
	}
	d.rtNext = time.Time{}
	limit := time.Duration(d.content.Combat.TurnSeconds * float64(time.Second))
	if len(eng) < 2 || now.Sub(d.waitSince) < limit {
		return nil
	}
	// Someone is too slow: they pass, so the other players are not held up.
	for _, p := range eng {
		if p.turnAt <= d.ct {
			p.turnAt = d.ct + 1
		}
	}
	d.waitSince = now
	return d.runCombat(now)
}

// ---------- what creatures do ----------

// act is one creature's turn.
func (d *Domain) act(c *Creature) []Event {
	def := d.cdef(c)
	if c.Stunned {
		c.Stunned = false
		c.readyAt = d.ct + 1
		return []Event{d.creatureEvent(c)}
	}
	if len(c.Warn) > 0 {
		return d.strikeWarned(c)
	}
	if c.Owner != "" {
		return d.petAct(c)
	}
	if !d.hostile(c) {
		if c.Submissive {
			c.readyAt = d.ct + 2 // it lies low
			return nil
		}
		return d.wander(c)
	}

	// Who or what is it after?
	r := d.content.Combat.EngageRadius
	var prey *Player
	preyRange := r
	if c.Wave {
		preyRange = 3 // a pest goes for the crops unless someone stands right there
	}
	best := math.MaxInt
	for _, p := range d.sortedPlayers() {
		if dist := cheb(p.Pos, c.Pos); d.awake(p) && dist <= preyRange && dist < best {
			prey, best = p, dist
		}
	}
	preyAt, found := Point{}, prey != nil
	if found {
		preyAt = prey.Pos
	} else if pet := d.petNear(c.Pos, 1); pet != nil {
		// A tamed creature standing right there gets bitten too.
		preyAt, found = pet.Pos, true
	}
	if found {
		if tiles, dir := d.attackFrom(c, preyAt); tiles != nil {
			c.Facing = dir
			c.Warn = tiles
			c.warnDmg = def.Attack.Damage
			c.Burrowed = false // it comes up to strike
			c.readyAt = d.ct + def.Attack.Windup
			return []Event{d.creatureEvent(c)}
		}
		return d.stepToward(c, adjacentGoals(preyAt))
	}
	if c.Wave {
		if plot := d.nearestCrop(c); plot != nil {
			if cheb(plot.At, c.Pos) <= 1 && (plot.At == c.Pos || plot.At.X == c.Pos.X || plot.At.Y == c.Pos.Y) {
				return d.eat(c, plot)
			}
			goals := adjacentGoals(plot.At)
			goals[plot.At] = true
			return d.stepToward(c, goals)
		}
		// Nothing left to eat: go after whoever is around.
		for _, p := range d.sortedPlayers() {
			if dist := cheb(p.Pos, c.Pos); d.awake(p) && dist <= r && dist < best {
				prey, best = p, dist
			}
		}
		if prey != nil {
			return d.stepToward(c, adjacentGoals(prey.Pos))
		}
	}
	return d.wander(c)
}

// attackFrom returns the tiles c would hit if it attacked toward target
// now, or nil when target is out of reach.
func (d *Domain) attackFrom(c *Creature, target Point) ([]Point, string) {
	pattern := d.cdef(c).Attack.Pattern
	for _, dir := range []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right} {
		tiles := patternTiles(pattern, c.Pos, dir)
		for _, t := range tiles {
			if t == target {
				return tiles, dir
			}
		}
	}
	return nil, ""
}

// patternTiles lists the tiles an attack covers from at, facing dir.
func patternTiles(pattern string, at Point, dir string) []Point {
	front := at.Step(dir)
	switch pattern {
	case PatternArc:
		l, r := sideDirs(dir)
		return []Point{front.Step(l), front, front.Step(r)}
	case PatternCross:
		return []Point{at.Step(protocol.Up), at.Step(protocol.Right), at.Step(protocol.Down), at.Step(protocol.Left)}
	case PatternLine2:
		return []Point{front, front.Step(dir)}
	}
	return []Point{front}
}

func sideDirs(dir string) (string, string) {
	if dir == protocol.Up || dir == protocol.Down {
		return protocol.Left, protocol.Right
	}
	return protocol.Up, protocol.Down
}

func adjacentGoals(p Point) map[Point]bool {
	return map[Point]bool{
		p.Step(protocol.Up): true, p.Step(protocol.Down): true,
		p.Step(protocol.Left): true, p.Step(protocol.Right): true,
	}
}

// strikeWarned lands the blow c announced on its last turn.
func (d *Domain) strikeWarned(c *Creature) []Event {
	events := []Event{}
	for _, t := range c.Warn {
		for _, p := range d.sortedPlayers() {
			if d.awake(p) && p.Pos == t {
				events = append(events, d.hurt(p, c.warnDmg)...)
			}
		}
		if b := d.bAt[t]; b != nil && b.HP > 0 {
			events = append(events, d.damageBuilding(b, c.warnDmg)...)
		}
		if pet := d.creatureAt(t); pet != nil && pet.Owner != "" {
			events = append(events, d.hurtPet(pet, c.warnDmg)...)
		}
	}
	c.Warn = nil
	c.Burrowed = d.cdef(c).Trait == TraitBurrow
	c.readyAt = d.ct + 1
	return append(events, d.creatureEvent(c))
}

// hurt takes health from a player; at zero they fall. Esquive may dodge it.
func (d *Domain) hurt(p *Player, dmg int) []Event {
	if d.chance(p.mod(ModDodge)) {
		return []Event{{Msg: protocol.Hit{T: protocol.TypeHit, Target: protocol.HitPlayer, ID: p.ID, X: p.Pos.X, Y: p.Pos.Y, Dodged: true}}}
	}
	p.HP -= dmg
	events := []Event{{Msg: protocol.Hit{T: protocol.TypeHit, Target: protocol.HitPlayer, ID: p.ID, X: p.Pos.X, Y: p.Pos.Y, Amount: dmg}}}
	if p.HP <= 0 {
		return append(events, d.knockOut(p)...)
	}
	return append(events, d.statsEvent(p), d.playerEvent(p))
}

// knockOut is falling in combat: the player wakes in bed and the bag, with
// everything in it, is gone. Tools stay in hand and the house chest is safe.
func (d *Domain) knockOut(p *Player) []Event {
	p.Bag = map[string]int{}
	p.HP = max(1, d.maxHP(p)/2)
	d.wakeAtHome(p)
	p.Sleeping = true
	d.pendingSleep = true // if everyone is in bed now, the night passes
	return []Event{
		d.playerEvent(p), d.statsEvent(p), d.bagEvent(p),
		{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeKO}},
	}
}

func (d *Domain) damageBuilding(b *Building, dmg int) []Event {
	b.HP -= dmg
	events := []Event{{Msg: protocol.Hit{T: protocol.TypeHit, Target: protocol.HitBuilding, ID: strconv.Itoa(b.ID), X: b.At.X, Y: b.At.Y, Amount: dmg}}}
	if b.HP <= 0 {
		return append(events, d.removeBuilding(b))
	}
	return append(events, d.buildingEvent(b))
}

// eat is a pest destroying a crop: the soil stays tilled.
func (d *Domain) eat(c *Creature, plot *Plot) []Event {
	plot.State, plot.Crop, plot.Growth, plot.Neglect = protocol.PlotTilled, "", 0, 0
	c.readyAt = d.ct + 2
	if plot.At.X != c.Pos.X || plot.At.Y != c.Pos.Y {
		c.Facing = dirTo(c.Pos, plot.At)
	}
	return []Event{d.plotEvent(plot), d.creatureEvent(c)}
}

func dirTo(from, to Point) string {
	dx, dy := to.X-from.X, to.Y-from.Y
	if abs(dx) >= abs(dy) {
		if dx < 0 {
			return protocol.Left
		}
		return protocol.Right
	}
	if dy < 0 {
		return protocol.Up
	}
	return protocol.Down
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// nearestCrop is the closest growing crop c may go for. Crows keep away
// from scarecrows.
func (d *Domain) nearestCrop(c *Creature) *Plot {
	var best *Plot
	bestDist := math.MaxInt
	for _, p := range d.sortedPlots() {
		if p.State != protocol.PlotCrop || (d.cdef(c).Trait == TraitFly && d.scared(p.At)) {
			continue
		}
		if dist := abs(p.At.X-c.Pos.X) + abs(p.At.Y-c.Pos.Y); dist < bestDist {
			best, bestDist = p, dist
		}
	}
	return best
}

// scared reports whether a scarecrow keeps crows away from p.
func (d *Domain) scared(p Point) bool {
	for _, b := range d.buildings {
		if b.Kind == BuildingScarecrow && cheb(b.At, p) <= d.content.Buildings[b.Kind].Radius {
			return true
		}
	}
	return false
}

// passable reports whether c may step onto t. ignoreFences finds the way a
// creature would take if fences were not there.
func (d *Domain) passable(c *Creature, t Point, ignoreFences bool) bool {
	if !d.Map.InBounds(t) || d.creatureAt(t) != nil || d.occupied(t) {
		return false
	}
	trait := d.cdef(c).Trait
	if trait == TraitFly {
		// Over trees, water and fences, but not through the walls of a house.
		// Scarecrows keep wild ones away, not tamed ones.
		return (c.Owner != "" || !d.scared(t)) && (!d.Map.Blocked(t) || d.Map.Water(t))
	}
	if d.Map.Blocked(t) {
		return false
	}
	if r := d.resAt[t]; r != nil && r.blocks() {
		return false
	}
	if b := d.bAt[t]; b != nil {
		fence := b.Kind == BuildingFence
		return fence && (ignoreFences || (trait == TraitBurrow && c.Burrowed))
	}
	return true
}

// stepToward moves c one tile along the shortest path to any goal tile. A
// walker walled in by fences attacks the fence in its way.
func (d *Domain) stepToward(c *Creature, goals map[Point]bool) []Event {
	def := d.cdef(c)
	if goals[c.Pos] {
		c.readyAt = d.ct + 1
		return nil
	}
	if step, ok := d.firstStep(c, goals, false); ok {
		return d.moveCreature(c, step)
	}
	if step, ok := d.firstStep(c, goals, true); ok {
		if b := d.bAt[step]; b != nil && b.Kind == BuildingFence {
			c.Facing = dirTo(c.Pos, step)
			c.Warn = []Point{step}
			c.warnDmg = def.Attack.Damage
			c.Burrowed = false
			c.readyAt = d.ct + def.Attack.Windup
			return []Event{d.creatureEvent(c)}
		}
		return d.moveCreature(c, step) // walk up to the fence first
	}
	return d.wander(c)
}

func (d *Domain) moveCreature(c *Creature, to Point) []Event {
	c.Facing = dirTo(c.Pos, to)
	c.Pos = to
	c.readyAt = d.ct + d.cdef(c).MoveTicks
	return []Event{d.creatureEvent(c)}
}

// firstStep is a breadth-first search from c to the nearest goal.
func (d *Domain) firstStep(c *Creature, goals map[Point]bool, ignoreFences bool) (Point, bool) {
	type node struct{ at, first Point }
	seen := map[Point]bool{c.Pos: true}
	queue := []node{}
	for _, dir := range []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right} {
		t := c.Pos.Step(dir)
		if d.passable(c, t, ignoreFences) {
			seen[t] = true
			queue = append(queue, node{t, t})
		}
	}
	for len(queue) > 0 && len(seen) < 4000 {
		n := queue[0]
		queue = queue[1:]
		if goals[n.at] {
			return n.first, true
		}
		for _, dir := range []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right} {
			t := n.at.Step(dir)
			if !seen[t] && d.passable(c, t, ignoreFences) {
				seen[t] = true
				queue = append(queue, node{t, n.first})
			}
		}
	}
	return Point{}, false
}

// wander is what a creature does with nothing to chase: now and then a step,
// staying in the forest edge when it lives there.
func (d *Domain) wander(c *Creature) []Event {
	c.readyAt = d.ct + d.cdef(c).MoveTicks + 1
	if d.rng.Float64() > 0.35 {
		return nil
	}
	dirs := []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right}
	dir := dirs[d.rng.Intn(4)]
	t := c.Pos.Step(dir)
	if !d.passable(c, t, false) || (!c.Wave && !d.Map.Edge(t) && d.Map.Edge(c.Pos)) {
		return nil
	}
	return d.moveCreature(c, t)
}

// ---------- what players do to creatures ----------

// toolTicks is how long a tool takes when used au pas.
func (d *Domain) toolTicks(tool string) int {
	if a := d.content.Tools[tool].Combat; a != nil {
		return a.Ticks
	}
	return 2
}

// attack swings a weapon tool at the creatures in its pattern. ok is false
// when nothing there can be hit, so the tool does its farm work instead.
func (d *Domain) attack(p *Player, tool string) (events []Event, ok bool) {
	a := d.content.Tools[tool].Combat
	if a == nil {
		return nil, false
	}
	for _, t := range patternTiles(a.Pattern, p.Pos, p.Facing) {
		c := d.creatureAt(t)
		if c == nil || c.Owner != "" || (c.Burrowed && a.Effect != EffectUnearth) {
			continue // tamed creatures are never hit
		}
		if !d.hostile(c) && t != p.Pos.Step(p.Facing) {
			continue // a wide swing does not hurt a calm creature by accident
		}
		ok = true
		events = append(events, d.strikeCreature(p, c, tool, a)...)
	}
	return events, ok
}

func (d *Domain) strikeCreature(p *Player, c *Creature, tool string, a *ToolAttack) []Event {
	def := d.cdef(c)
	dmg, armor := a.Damage+p.mod(modToolDmgStart+toolSuffix(tool)), c.Armor
	if d.night() {
		dmg += p.mod(ModNight)
	}
	if p.mod(ModPackLeader) > 0 {
		dmg = 0 // the pack does the biting
	}
	if def.Weakness == tool {
		dmg *= 2
		armor = 0
		if def.Trait == TraitBark {
			c.Armor = 0 // the bark is broken for good
		}
	}
	dmg = max(0, dmg-armor)
	c.HP -= dmg
	events := []Event{{Msg: protocol.Hit{T: protocol.TypeHit, Target: protocol.HitCreature, ID: strconv.Itoa(c.ID), X: c.Pos.X, Y: c.Pos.Y, Amount: dmg}}}
	if c.HP <= 0 {
		events = append(events, d.loot(p, def)...)
		events = append(events, d.gainXP(p, XPCreature)...)
		return append(events, d.removeCreature(c, protocol.GoneKilled))
	}
	events = append(events, d.afterHit(p, c, dmg)...)
	switch a.Effect {
	case EffectUnearth:
		c.Burrowed = false
	case EffectStun:
		c.Stunned = true
		c.Warn = nil // the blow it was preparing is lost
		c.readyAt = max(c.readyAt, d.ct+1)
	case EffectKnock:
		if back := c.Pos.Step(p.Facing); d.passable(c, back, false) {
			c.Pos = back
			c.Warn = nil
		}
	}
	return append(events, d.creatureEvent(c))
}

// loot puts what a creature leaves into the bag of whoever finished it.
func (d *Domain) loot(p *Player, def CreatureDef) []Event {
	items := make([]string, 0, len(def.Loot))
	for item := range def.Loot {
		items = append(items, item)
	}
	sort.Strings(items)
	for _, item := range items {
		n := def.Loot[item]
		whole := int(n)
		if d.rng.Float64() < n-float64(whole) {
			whole++
		}
		p.Bag[item] += whole
	}
	return []Event{d.bagEvent(p)}
}

// eatFood restores health from food in the bag.
func (d *Domain) eatFood(p *Player, item string) ([]Event, string) {
	heal := d.content.Food[item]
	if heal <= 0 || p.Bag[item] <= 0 {
		return nil, ReasonNoItem
	}
	if p.HP >= d.maxHP(p) {
		return nil, ReasonFullHealth
	}
	p.Bag[item]--
	p.HP = min(d.maxHP(p), p.HP+heal)
	return []Event{d.bagEvent(p), d.statsEvent(p), d.playerEvent(p)}, ""
}

// ReasonFullHealth refuses eating at full health.
const ReasonFullHealth = "full_health"

// ---------- populations ----------

// edgeTiles lists the free forest-edge tiles outside the domain, away from
// every player, from which the house can be walked to: a creature spawned
// in a pocket closed off by trees would never come out.
func (d *Domain) edgeTiles() []Point {
	var out []Point
	r := d.content.Combat.EngageRadius
	reach := d.reachable(d.Map.WakeSpot())
	for y := 0; y < d.Map.Height; y++ {
	next:
		for x := 0; x < d.Map.Width; x++ {
			t := Point{x, y}
			if !d.Map.Edge(t) || d.Claimed(t) || !reach[t] || d.plots[t] != nil || d.creatureAt(t) != nil {
				continue
			}
			for _, p := range d.players {
				if p.Online && cheb(p.Pos, t) <= r+1 {
					continue next
				}
			}
			out = append(out, t)
		}
	}
	return out
}

// reachable floods the ground a walker can reach from start, ignoring
// players, creatures and fences.
func (d *Domain) reachable(start Point) map[Point]bool {
	open := func(t Point) bool {
		if d.Map.Blocked(t) {
			return false
		}
		if res := d.resAt[t]; res != nil && res.blocks() {
			return false
		}
		b := d.bAt[t]
		return b == nil || b.Kind == BuildingFence
	}
	seen := map[Point]bool{start: true}
	queue := []Point{start}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, dir := range []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right} {
			if t := p.Step(dir); !seen[t] && open(t) {
				seen[t] = true
				queue = append(queue, t)
			}
		}
	}
	return seen
}

// topUpEdge brings the forest-edge population back to its size.
func (d *Domain) topUpEdge() []Event {
	have := 0
	for _, c := range d.creatures {
		if !c.Wave && !c.Fauna && c.Owner == "" {
			have++
		}
	}
	species := d.content.Edge.Species
	if have >= d.content.Edge.Population || len(species) == 0 {
		return nil
	}
	tiles := d.edgeTiles()
	var events []Event
	for ; have < d.content.Edge.Population && len(tiles) > 0; have++ {
		i := d.rng.Intn(len(tiles))
		at := tiles[i]
		tiles = append(tiles[:i], tiles[i+1:]...)
		_, e := d.spawn(species[d.rng.Intn(len(species))], at, false)
		events = append(events, e)
	}
	return events
}

// WaveSize is how many pests come tonight: more crops and more days, more pests.
func (d *Domain) WaveSize() int {
	w := d.content.Wave
	crops := 0
	for _, p := range d.plots {
		if p.State == protocol.PlotCrop {
			crops++
		}
	}
	n := w.Base
	if w.PerCrops > 0 {
		n += crops / w.PerCrops
	}
	if w.PerDays > 0 {
		n += (d.Day() - 1) / w.PerDays
	}
	if w.Max > 0 {
		n = min(n, w.Max)
	}
	return n
}

// maybeWave sends the night's pests after the fields, once a night, only
// when someone is up to defend them: an empty valley is never attacked.
func (d *Domain) maybeWave() []Event {
	w := d.content.Wave
	if d.lastWaveDay == d.Day() || d.MinuteOfDay() < w.Hour*60 || !d.anyoneAwake() {
		return nil
	}
	hasCrops := false
	for _, p := range d.plots {
		if p.State == protocol.PlotCrop {
			hasCrops = true
			break
		}
	}
	d.lastWaveDay = d.Day()
	if !hasCrops {
		return nil // nothing to eat, nobody comes
	}
	species := w.Species
	if d.Day() < w.EarlyDays && len(w.EarlySpecies) > 0 {
		species = w.EarlySpecies
	}
	tiles := d.edgeTiles()
	var events []Event
	for i := 0; i < d.WaveSize() && len(tiles) > 0; i++ {
		j := d.rng.Intn(len(tiles))
		at := tiles[j]
		tiles = append(tiles[:j], tiles[j+1:]...)
		_, e := d.spawn(species[d.rng.Intn(len(species))], at, true)
		events = append(events, e)
	}
	if len(events) > 0 {
		events = append(events, Event{Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeWave}})
	}
	return events
}

// dawn sends the night's pests, and wild night creatures, back to the forest.
func (d *Domain) dawn() []Event {
	var events []Event
	for _, c := range d.sortedCreatures() {
		if c.Owner == "" && d.cdef(c).Nocturnal {
			events = append(events, d.removeCreature(c, protocol.GoneFled))
		}
	}
	waves := 0
	for _, c := range d.sortedCreatures() {
		if c.Wave {
			events = append(events, d.removeCreature(c, protocol.GoneFled))
			waves++
		}
	}
	if waves == 0 {
		return events
	}
	if len(events) > 0 {
		events = append(events, Event{Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeWaveOver}})
	}
	return events
}

// threatened reports whether a hostile creature stands within the flee radius of p.
func (d *Domain) threatened(p *Player) bool {
	for _, c := range d.creatures {
		if d.hostile(c) && cheb(c.Pos, p.Pos) <= d.content.Combat.FleeRadius {
			return true
		}
	}
	return false
}
