package world

import (
	"math"
	"sort"
	"strconv"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// Taming. Creatures are not caught: they are won over.
//
// Some species of the forest (the "faune" in contenu.json) can be tamed.
// Feeding one something it likes raises its trust; a meal at most every
// appetitMinutes game minutes. Hostile species must first be worn down
// without being killed: below their soumission share of health they give in
// and stop fighting, and only then accept food. Hitting a calm creature
// makes it forget the trust it had.
//
// When trust is full, the creature binds to the player who fed it, as long
// as the domain has a free habitat for it (an enclos, a tanière, a nichoir).
// Then it has a role:
//   - production: each morning after a day it was fed, it leaves something
//     in its habitat (E on the habitat to collect);
//   - garde: it stays by its den and fights what comes within rayonGarde;
//   - compagnon: it follows its player and fights what comes within
//     rayonCompagnon of them.
//
// E (whatever is in hand, except its food) on your own creature tells it to
// follow you or to go home; only so many may follow at once (compagnons). A
// tamed creature never dies: worn out, it goes home to rest until morning.

// Reasons specific to taming.
const (
	ReasonNotHungry  = "not_hungry"
	ReasonGreenThumb = "green_thumb" // Main verte: the watering can no longer waters
)

func toolSuffix(tool string) string {
	if tool == "" {
		return ""
	}
	return string(tool[0]-'a'+'A') + tool[1:]
}

// night reports whether it is night: nocturnal creatures are out.
func (d *Domain) night() bool {
	m := d.MinuteOfDay()
	return m >= d.content.Taming.NightHour*60 || m < d.content.Time.WakeHour*60
}

// creatureMaxHP is a creature's health; tamed ones get their player's talents.
func (d *Domain) creatureMaxHP(c *Creature) int {
	hp := d.cdef(c).HP
	if o := d.players[c.Owner]; o != nil {
		hp += o.mod(ModPetHP)
	}
	return hp
}

func (d *Domain) isHabitat(kind string) bool {
	for _, cr := range d.content.Creatures {
		if cr.Taming != nil && cr.Taming.Habitat == kind {
			return true
		}
	}
	return false
}

// resident is the tamed creature living in b, or nil.
func (d *Domain) resident(b *Building) *Creature {
	for _, c := range d.creatures {
		if c.Owner != "" && c.Home == b.ID {
			return c
		}
	}
	return nil
}

// freeHabitat is a building of the given kind with nobody living in it.
func (d *Domain) freeHabitat(kind string) *Building {
	for _, b := range d.sortedBuildings() {
		if b.Kind == kind && d.resident(b) == nil {
			return b
		}
	}
	return nil
}

// petNear is the nearest tamed creature within r of at, or nil.
func (d *Domain) petNear(at Point, r int) *Creature {
	var best *Creature
	for _, c := range d.sortedCreatures() {
		if c.Owner != "" && !c.Resting && cheb(c.Pos, at) <= r && (best == nil || cheb(c.Pos, at) < cheb(best.Pos, at)) {
			best = c
		}
	}
	return best
}

func (d *Domain) followers(p *Player) []*Creature {
	var out []*Creature
	for _, c := range d.sortedCreatures() {
		if c.Owner == p.ID && c.Mode == protocol.ModeFollow {
			out = append(out, c)
		}
	}
	return out
}

func (d *Domain) followerMax(p *Player) int {
	return d.content.Taming.BaseFollowers + p.mod(ModFollowers)
}

// followerLimit sends home the followers beyond what p may lead now.
func (d *Domain) followerLimit(p *Player) []Event {
	var events []Event
	f := d.followers(p)
	for i := d.followerMax(p); i < len(f); i++ {
		f[i].Mode = protocol.ModeHome
		events = append(events, d.creatureEvent(f[i]))
	}
	// Talents may have changed their health too.
	for _, c := range d.sortedCreatures() {
		if c.Owner == p.ID && c.HP > d.creatureMaxHP(c) {
			c.HP = d.creatureMaxHP(c)
			events = append(events, d.creatureEvent(c))
		}
	}
	return events
}

func notice(to, code, arg string) Event {
	return Event{To: to, Msg: protocol.Notice{T: protocol.TypeNotice, Code: code, Arg: arg}}
}

// ---------- what players do to creatures ----------

// useOnCreature is E facing a creature. ok is false when nothing happens,
// so the rest of Use goes on.
func (d *Domain) useOnCreature(p *Player, c *Creature, item string) (Result, bool) {
	tm := d.cdef(c).Taming
	if c.Owner == p.ID {
		if tm != nil && tm.Diet[item] > 0 && p.Bag[item] > 0 {
			return d.feedPet(p, c, item), true
		}
		return d.toggleFollow(p, c), true
	}
	if c.Owner != "" || tm == nil || tm.Diet[item] <= 0 || p.Bag[item] <= 0 || d.hostile(c) {
		return Result{}, false
	}
	return d.feedWild(p, c, item), true
}

func (d *Domain) hungry(c *Creature) bool {
	return c.ateAt == 0 || d.minutes-c.ateAt >= float64(d.content.Taming.AppetiteMins)
}

// feedWild gives a wild creature a meal: it trusts the player a little more,
// and once trust is full and a habitat is free, it is tamed.
func (d *Domain) feedWild(p *Player, c *Creature, item string) Result {
	tm := d.cdef(c).Taming
	maxTrust := d.content.Taming.MaxTrust
	if c.Trust >= maxTrust {
		return d.bond(p, c)
	}
	if !d.hungry(c) {
		return Result{OK: true, Events: []Event{notice(p.ID, protocol.NoticeFull, c.Kind)}}
	}
	p.Bag[item]--
	c.ateAt = d.minutes
	gain := tm.Diet[item] * (100 + p.mod(ModTrust)) / 100
	c.Trust = min(maxTrust, c.Trust+gain)
	events := []Event{d.bagEvent(p), d.creatureEvent(c)}
	events = append(events, d.gainXP(p, XPFeed)...)
	if c.Trust >= maxTrust {
		res := d.bond(p, c)
		return Result{OK: true, Events: append(events, res.Events...)}
	}
	return Result{OK: true, Events: append(events, notice(p.ID, protocol.NoticeFed, c.Kind))}
}

// bond ties a creature whose trust is full to p, if a habitat is free.
func (d *Domain) bond(p *Player, c *Creature) Result {
	tm := d.cdef(c).Taming
	home := d.freeHabitat(tm.Habitat)
	if home == nil {
		return Result{OK: true, Events: []Event{notice(p.ID, protocol.NoticeHabitat, tm.Habitat)}}
	}
	c.Owner, c.Home, c.Fauna, c.Submissive, c.Trust = p.ID, home.ID, false, false, 0
	c.Warn, c.Burrowed, c.Stunned = nil, false, false
	c.HP = d.creatureMaxHP(c)
	c.Mode = protocol.ModeHome
	if tm.Role == RoleCompanion && len(d.followers(p)) < d.followerMax(p) {
		c.Mode = protocol.ModeFollow
	}
	c.readyAt = d.ct + 1
	if c.Mode == protocol.ModeHome {
		d.teleport(c, d.homeSpot(c))
	}
	events := []Event{d.creatureEvent(c), notice(p.ID, protocol.NoticeTamed, c.Kind)}
	return Result{OK: true, Events: append(events, d.gainXP(p, XPTame)...)}
}

// feedPet keeps a tamed creature happy: a production creature only gives
// something the morning after a day it was fed. A meal also heals.
func (d *Domain) feedPet(p *Player, c *Creature, item string) Result {
	if !d.hungry(c) {
		return Result{OK: true, Events: []Event{notice(p.ID, protocol.NoticeFull, c.Kind)}}
	}
	p.Bag[item]--
	c.ateAt = d.minutes
	c.Fed = true
	c.HP = min(d.creatureMaxHP(c), c.HP+2)
	events := []Event{d.bagEvent(p), d.creatureEvent(c), notice(p.ID, protocol.NoticeFed, c.Kind)}
	return Result{OK: true, Events: append(events, d.gainXP(p, XPFeed)...)}
}

func (d *Domain) toggleFollow(p *Player, c *Creature) Result {
	if c.Mode == protocol.ModeFollow {
		c.Mode = protocol.ModeHome
		return Result{OK: true, Events: []Event{d.creatureEvent(c), notice(p.ID, protocol.NoticeStay, c.Kind)}}
	}
	if d.cdef(c).Taming.Role == RoleProduction {
		return Result{OK: true, Events: []Event{notice(p.ID, protocol.NoticeStay, c.Kind)}} // it stays by its pen
	}
	if len(d.followers(p)) >= d.followerMax(p) {
		return Result{OK: true, Events: []Event{notice(p.ID, protocol.NoticeTooMany, strconv.Itoa(d.followerMax(p)))}}
	}
	c.Mode = protocol.ModeFollow
	c.Resting = false
	return Result{OK: true, Events: []Event{d.creatureEvent(c), notice(p.ID, protocol.NoticeFollow, c.Kind)}}
}

// afterHit is what a blow that did not kill does to a creature's trust.
func (d *Domain) afterHit(p *Player, c *Creature, dmg int) []Event {
	tm := d.cdef(c).Taming
	if tm == nil || c.Owner != "" {
		return nil
	}
	if !d.hostile(c) && !c.Submissive {
		if c.Trust > 0 {
			c.Trust = 0
			return []Event{notice(p.ID, protocol.NoticeScared, c.Kind)}
		}
		return nil
	}
	if tm.Submission > 0 && !c.Submissive && c.HP <= int(math.Ceil(float64(d.creatureMaxHP(c))*tm.Submission)) {
		c.Submissive = true
		c.Warn = nil
		return []Event{notice(p.ID, protocol.NoticeSubdued, c.Kind)}
	}
	return nil
}

// swapWithPet lets a player walk through their own creature: they trade places.
func (d *Domain) swapWithPet(p *Player, pet *Creature) bool {
	if pet.Owner != p.ID {
		return false
	}
	target := pet.Pos
	if r := d.resAt[target]; d.Map.Blocked(target) || d.bAt[target] != nil || (r != nil && r.blocks()) {
		return false
	}
	pet.Pos, p.Pos = p.Pos, target
	pet.Facing = dirTo(target, pet.Pos)
	return true
}

// ---------- tamed creatures ----------

// homeSpot is where a tamed creature stays: by its habitat, or by the bed.
func (d *Domain) homeSpot(c *Creature) Point {
	b := d.buildings[c.Home]
	if b == nil {
		return d.Map.WakeSpot()
	}
	// The first free tile around the building, from below, then the sides.
	var ring []Point
	for x := b.At.X - 1; x <= b.At.X+b.W; x++ {
		ring = append(ring, Point{x, b.At.Y + b.H})
	}
	for y := b.At.Y + b.H - 1; y >= b.At.Y; y-- {
		ring = append(ring, Point{b.At.X - 1, y}, Point{b.At.X + b.W, y})
	}
	for x := b.At.X - 1; x <= b.At.X+b.W; x++ {
		ring = append(ring, Point{x, b.At.Y - 1})
	}
	for _, t := range ring {
		if d.standable(c, t) {
			return t
		}
	}
	return ring[0]
}

// standable reports whether c could stand on t, ignoring other creatures.
func (d *Domain) standable(c *Creature, t Point) bool {
	if !d.Map.InBounds(t) || d.bAt[t] != nil {
		return false
	}
	fly := d.cdef(c).Trait == TraitFly
	if d.Map.Blocked(t) {
		return fly && d.Map.Water(t)
	}
	if r := d.resAt[t]; r != nil && r.blocks() {
		return fly
	}
	return true
}

// teleport puts c on the free tile nearest to near (it walked there off screen).
func (d *Domain) teleport(c *Creature, near Point) {
	for r := 0; r <= 4; r++ {
		for y := near.Y - r; y <= near.Y+r; y++ {
			for x := near.X - r; x <= near.X+r; x++ {
				t := Point{x, y}
				if cheb(t, near) != r || t == c.Pos {
					continue
				}
				if d.standable(c, t) && !d.occupied(t) && d.creatureAt(t) == nil {
					c.Pos = t
					return
				}
			}
		}
	}
}

// petAct is a tamed creature's turn.
func (d *Domain) petAct(c *Creature) []Event {
	def := d.cdef(c)
	o := d.players[c.Owner]
	c.readyAt = d.ct + def.MoveTicks
	home := d.homeSpot(c)
	if c.Resting {
		if c.Pos != home && cheb(c.Pos, home) > 1 {
			d.teleport(c, home)
			return []Event{d.creatureEvent(c)}
		}
		c.readyAt = d.ct + 4
		return nil
	}
	following := c.Mode == protocol.ModeFollow && o != nil && o.Online
	anchor, radius := home, 0
	switch {
	case following:
		anchor, radius = o.Pos, d.content.Taming.FollowerRadius
	case def.Taming.Role == RoleGuard:
		radius = d.content.Taming.GuardRadius
	}
	if radius > 0 {
		if foe := d.foeNear(anchor, radius); foe != nil {
			return d.petFight(c, foe)
		}
	}
	if following {
		dist := cheb(c.Pos, o.Pos)
		switch {
		case dist > 6:
			d.teleport(c, o.Pos)
			return []Event{d.creatureEvent(c)}
		case dist > 1:
			return d.petStep(c, adjacentGoals(o.Pos), o.Pos)
		}
		c.readyAt = d.ct + 1
		return nil
	}
	if cheb(c.Pos, home) > 2 {
		goals := adjacentGoals(home)
		goals[home] = true
		return d.petStep(c, goals, home)
	}
	// Idle by its home: a step now and then.
	c.readyAt = d.ct + def.MoveTicks + 2
	if d.rng.Float64() > 0.3 {
		return nil
	}
	dirs := []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right}
	t := c.Pos.Step(dirs[d.rng.Intn(4)])
	if cheb(t, home) > 2 || !d.passable(c, t, false) {
		return nil
	}
	return d.moveCreature(c, t)
}

// petStep walks c toward a goal; with no way through (fences…), it finds its
// way around off screen.
func (d *Domain) petStep(c *Creature, goals map[Point]bool, near Point) []Event {
	if step, ok := d.firstStep(c, goals, false); ok {
		return d.moveCreature(c, step)
	}
	d.teleport(c, near)
	return []Event{d.creatureEvent(c)}
}

// foeNear is the nearest hostile creature within r of at that a tamed
// creature could reach (not underground).
func (d *Domain) foeNear(at Point, r int) *Creature {
	var best *Creature
	for _, c := range d.sortedCreatures() {
		if !d.hostile(c) || c.Burrowed || cheb(c.Pos, at) > r {
			continue
		}
		if best == nil || cheb(c.Pos, at) < cheb(best.Pos, at) {
			best = c
		}
	}
	return best
}

// petFight attacks foe if it is in reach, or goes for it.
func (d *Domain) petFight(c, foe *Creature) []Event {
	def := d.cdef(c)
	tiles, dir := d.attackFrom(c, foe.Pos)
	if tiles == nil {
		return d.petStep(c, adjacentGoals(foe.Pos), c.Pos)
	}
	c.Facing = dir
	c.readyAt = d.ct + def.Attack.Windup + 1
	events := []Event{d.creatureEvent(c)}
	for _, t := range tiles {
		if target := d.creatureAt(t); target != nil && d.hostile(target) && !target.Burrowed {
			events = append(events, d.petStrike(c, target)...)
		}
	}
	return events
}

// petStrike is a tamed creature's blow: its damage plus its player's
// talents, tripled for a Meneur de meute, and never fully stopped by armour.
func (d *Domain) petStrike(c, target *Creature) []Event {
	o := d.players[c.Owner]
	dmg := d.cdef(c).Attack.Damage
	if o != nil {
		dmg += o.mod(ModPetDamage)
		if o.mod(ModPackLeader) > 0 {
			dmg *= 3
		}
	}
	dmg = max(1, dmg-target.Armor)
	target.HP -= dmg
	events := []Event{{Msg: protocol.Hit{T: protocol.TypeHit, Target: protocol.HitCreature, ID: strconv.Itoa(target.ID), X: target.Pos.X, Y: target.Pos.Y, Amount: dmg}}}
	if target.HP > 0 {
		if o != nil {
			events = append(events, d.afterHit(o, target, dmg)...)
		}
		return append(events, d.creatureEvent(target))
	}
	if o != nil && o.Online {
		events = append(events, d.loot(o, d.cdef(target))...)
		events = append(events, d.gainXP(o, XPCreature)...)
	}
	return append(events, d.removeCreature(target, protocol.GoneKilled))
}

// hurtPet is a blow on a tamed creature. Worn out, it goes home to rest.
func (d *Domain) hurtPet(c *Creature, dmg int) []Event {
	c.HP -= dmg
	events := []Event{{Msg: protocol.Hit{T: protocol.TypeHit, Target: protocol.HitCreature, ID: strconv.Itoa(c.ID), X: c.Pos.X, Y: c.Pos.Y, Amount: dmg}}}
	if c.HP <= 0 {
		c.HP = 1
		c.Resting = true
		c.Warn = nil
		d.teleport(c, d.homeSpot(c))
		events = append(events, notice(c.Owner, protocol.NoticeTired, c.Kind))
	}
	return append(events, d.creatureEvent(c))
}

// petsMorning heals and wakes tamed creatures, and production creatures fed
// yesterday leave their product in their habitat.
func (d *Domain) petsMorning() []Event {
	var events []Event
	for _, c := range d.sortedCreatures() {
		if c.Owner == "" {
			continue
		}
		tm := d.cdef(c).Taming
		if b := d.buildings[c.Home]; b != nil && tm.Role == RoleProduction && c.Fed {
			n := 1
			if o := d.players[c.Owner]; o != nil && d.chance(o.mod(ModProduction)) {
				n++
			}
			b.Ready = min(tm.StockMax, b.Ready+n)
			events = append(events, d.buildingEvent(b))
		}
		c.Fed = false
		c.Resting = false
		c.HP = d.creatureMaxHP(c)
		events = append(events, d.creatureEvent(c))
	}
	return events
}

// collectProduce takes what a production creature left in its habitat.
func (d *Domain) collectProduce(p *Player, b *Building) Result {
	product := ""
	for _, cr := range d.content.Creatures {
		if cr.Taming != nil && cr.Taming.Habitat == b.Kind && cr.Taming.Product != "" {
			product = cr.Taming.Product
		}
	}
	if product == "" {
		return Result{Reason: ReasonNothing}
	}
	p.Bag[product] += b.Ready
	b.Ready = 0
	events := []Event{d.buildingEvent(b), d.bagEvent(p), notice(p.ID, protocol.NoticeProduce, product)}
	return Result{OK: true, Events: append(events, d.gainXP(p, XPProduce)...)}
}

// petsRejoin brings a player's followers to their side when they come back.
func (d *Domain) petsRejoin(p *Player) []Event {
	var events []Event
	for _, c := range d.followers(p) {
		if !c.Resting && cheb(c.Pos, p.Pos) > 1 {
			d.teleport(c, p.Pos)
			events = append(events, d.creatureEvent(c))
		}
	}
	return events
}

// ---------- wild fauna ----------

// topUpFauna keeps the wild creatures of contenu.json's faune around, at the
// forest edge; nocturnal ones only at night.
func (d *Domain) topUpFauna() []Event {
	if !d.populated {
		return nil
	}
	var events []Event
	tiles := d.edgeTiles()
	for _, f := range d.content.Fauna {
		def := d.content.Creatures[f.Species]
		if def.Nocturnal && !d.night() {
			continue
		}
		have := 0
		for _, c := range d.creatures {
			if c.Kind == f.Species && c.Owner == "" && c.Fauna {
				have++
			}
		}
		for ; have < f.Count && len(tiles) > 0; have++ {
			i := d.rng.Intn(len(tiles))
			at := tiles[i]
			tiles = append(tiles[:i], tiles[i+1:]...)
			c, e := d.spawn(f.Species, at, false)
			c.Fauna = true
			events = append(events, e)
		}
	}
	return events
}

// faunaClock brings out the night creatures once night falls.
func (d *Domain) faunaClock() []Event {
	if !d.populated || !d.night() || d.faunaNight == d.Day() {
		return nil
	}
	d.faunaNight = d.Day()
	return d.topUpFauna()
}

// ---------- saving ----------

func (d *Domain) petStates() []PetState {
	var out []PetState
	for _, c := range d.sortedCreatures() {
		if c.Owner == "" {
			continue
		}
		out = append(out, PetState{Kind: c.Kind, Owner: c.Owner, Mode: c.Mode, Home: c.Home, HP: c.HP,
			Fed: c.Fed, Resting: c.Resting, X: c.Pos.X, Y: c.Pos.Y})
	}
	return out
}

func (d *Domain) restorePets(states []PetState) {
	sort.SliceStable(states, func(i, j int) bool { return states[i].Owner < states[j].Owner })
	for _, ps := range states {
		def, ok := d.content.Creatures[ps.Kind]
		if !ok || def.Taming == nil {
			continue
		}
		c, _ := d.spawn(ps.Kind, Point{ps.X, ps.Y}, false)
		c.Owner, c.Mode, c.Home, c.Fed, c.Resting = ps.Owner, ps.Mode, ps.Home, ps.Fed, ps.Resting
		c.Burrowed = false
		if c.Mode != protocol.ModeFollow {
			c.Mode = protocol.ModeHome
		}
		if b := d.buildings[c.Home]; b == nil || b.Kind != def.Taming.Habitat {
			c.Home = 0 // its habitat is gone: it stays by the bed until it gets another
		}
		c.HP = ps.HP
		if c.HP <= 0 || c.HP > d.creatureMaxHP(c) {
			c.HP = d.creatureMaxHP(c)
		}
		if !d.standable(c, c.Pos) || d.occupiedByOther(c) {
			d.teleport(c, d.homeSpot(c))
		}
	}
}

func (d *Domain) occupiedByOther(c *Creature) bool {
	for _, o := range d.creatures {
		if o != c && o.Pos == c.Pos {
			return true
		}
	}
	return false
}
