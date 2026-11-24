package world

import (
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// steps hands out times one second apart, past every cooldown.
type steps struct{ now time.Time }

func (s *steps) next() time.Time {
	s.now = s.now.Add(time.Second)
	return s.now
}

// still keeps a creature from acting during a test.
func still(c *Creature) { c.readyAt = 1 << 30 }

func TestCreatureWarnsThenStrikes(t *testing.T) {
	m := testMap(t,
		".......",
		".@.....",
		".......",
	)
	d := joined(t, m, "ana")
	c, _ := d.spawn("rongeur-racine", Point{4, 1}, false)
	ana := d.Player("ana")
	if !d.engaged(ana) {
		t.Fatal("a creature 3 tiles away should put ana au pas")
	}
	s := &steps{t0}
	for i := 0; i < 10 && len(c.Warn) == 0; i++ {
		if r := d.Wait("ana", s.next()); !r.OK {
			t.Fatalf("wait: %+v", r)
		}
	}
	if len(c.Warn) == 0 || c.Warn[0] != ana.Pos {
		t.Fatalf("the rodent should mark ana's tile, warn = %v at %v", c.Warn, c.Pos)
	}
	// Step aside: the blow lands on an empty tile.
	away := protocol.Down
	if c.Pos == ana.Pos.Step(protocol.Down) {
		away = protocol.Up
	}
	if r := d.Move("ana", away, s.next()); !r.OK {
		t.Fatalf("step aside: %+v", r)
	}
	if ana.HP != d.content.Health.Max {
		t.Fatalf("hp = %d after dodging", ana.HP)
	}
	// Stand still: it comes and bites.
	for i := 0; i < 10 && ana.HP == d.content.Health.Max; i++ {
		d.Wait("ana", s.next())
	}
	if ana.HP != d.content.Health.Max-1 {
		t.Fatalf("hp = %d, want one bite", ana.HP)
	}
}

func TestOutOfReachIsRealTime(t *testing.T) {
	m := testMap(t, ".@..........")
	d := joined(t, m, "ana")
	c, _ := d.spawn("ronceau", Point{11, 0}, false)
	if d.engaged(d.Player("ana")) {
		t.Fatal("10 tiles away is not au pas")
	}
	// Moving is never refused for want of a turn, and the clock ticks on its own.
	ct := d.ct
	d.Tick(t0.Add(3 * time.Second))
	if d.ct <= ct {
		t.Fatal("the combat clock should follow real time while nobody is au pas")
	}
	_ = c
}

func TestTurnsWithTwoPlayers(t *testing.T) {
	m := testMap(t,
		".....",
		".@...",
		".....",
	)
	d := joined(t, m, "ana", "bob")
	c, _ := d.spawn("ronceau", Point{4, 2}, false)
	still(c)
	s := &steps{t0}
	if r := d.Wait("ana", s.next()); !r.OK {
		t.Fatalf("ana waits: %+v", r)
	}
	if r := d.Wait("ana", s.next()); r.Reason != ReasonNotYourTurn {
		t.Fatalf("ana again before bob: %+v", r)
	}
	if r := d.Move("bob", protocol.Right, s.next()); !r.OK {
		t.Fatalf("bob: %+v", r)
	}
	if r := d.Wait("ana", s.next()); !r.OK {
		t.Fatalf("ana after bob: %+v", r)
	}
	// Bob dawdles: after the turn delay he passes and ana plays on.
	d.Tick(s.now.Add(11 * time.Second))
	if r := d.Wait("ana", s.now.Add(12*time.Second)); !r.OK {
		t.Fatalf("ana after bob timed out: %+v", r)
	}
}

func TestToolsAgainstArmour(t *testing.T) {
	m := testMap(t, ".@...")
	d := joined(t, m, "ana")
	d.Player("ana").Facing = protocol.Right
	s := &steps{t0}

	scarab, _ := d.spawn("scarabee-galet", Point{2, 0}, false)
	still(scarab)
	d.Use("ana", "houe", s.next())
	if scarab.HP != 6 {
		t.Fatalf("the hoe should not get through the shell: hp %d", scarab.HP)
	}
	d.Use("ana", "pioche", s.next())
	if scarab.HP != 2 || !scarab.Stunned {
		t.Fatalf("the pickaxe pierces and stuns: hp %d stunned %v", scarab.HP, scarab.Stunned)
	}
	d.removeCreature(scarab, protocol.GoneFled)

	bramble, _ := d.spawn("ronceau", Point{2, 0}, false)
	still(bramble)
	d.Use("ana", "houe", s.next())
	if bramble.HP != 8 {
		t.Fatalf("bark stops the hoe: hp %d", bramble.HP)
	}
	d.Use("ana", "hache", s.next())
	if bramble.HP != 2 || bramble.Armor != 0 {
		t.Fatalf("the axe breaks the bark: hp %d armour %d", bramble.HP, bramble.Armor)
	}
	if bramble.Pos != (Point{3, 0}) {
		t.Fatalf("the axe knocks back: at %v", bramble.Pos)
	}
}

func TestBurrowedRodentAndLoot(t *testing.T) {
	m := testMap(t, ".@...")
	d := joined(t, m, "ana")
	d.Player("ana").Facing = protocol.Right
	s := &steps{t0}
	rat, _ := d.spawn("rongeur-racine", Point{2, 0}, false)
	still(rat)
	if r := d.Use("ana", "hache", s.next()); r.OK {
		t.Fatalf("the axe cannot reach underground: %+v", r)
	}
	d.Use("ana", "houe", s.next())
	if rat.Burrowed || rat.HP != 1 {
		t.Fatalf("the hoe digs it up: burrowed %v hp %d", rat.Burrowed, rat.HP)
	}
	d.Use("ana", "faux", s.next())
	if d.creatures[rat.ID] != nil {
		t.Fatal("the rodent should be dead")
	}
	if d.Player("ana").Bag["fibre"] < 1 {
		t.Fatalf("loot: %v", d.Player("ana").Bag)
	}
}

func TestFallingLosesTheBagOnly(t *testing.T) {
	m := testMap(t,
		"B.C...",
		"......",
		"..@...",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	ana.Bag["bois"] = 9
	d.chest["pierre"] = 4
	ana.HP = 1
	c, _ := d.spawn("ronceau", Point{3, 2}, false)
	c.Warn = []Point{ana.Pos}
	c.warnDmg = 2
	c.readyAt = d.ct

	events := d.Wait("ana", t0.Add(time.Second)).Events
	if count[protocol.Notice](events) == 0 {
		t.Fatal("expected a notice")
	}
	if len(ana.Bag) != 0 {
		t.Fatalf("bag after falling: %v", ana.Bag)
	}
	if len(ana.Tools) != len(d.content.StarterTools) || d.chest["pierre"] != 4 {
		t.Fatal("tools and the chest are kept")
	}
	// Alone in the valley, falling sends straight to the next morning.
	if ana.Sleeping || ana.HP != d.content.Health.Max || ana.Pos != m.WakeSpot() || d.Day() != 2 {
		t.Fatalf("woke: sleeping %v hp %d at %v day %d", ana.Sleeping, ana.HP, ana.Pos, d.Day())
	}
}

func TestLeavingNextToACreatureCountsAsFalling(t *testing.T) {
	m := testMap(t, ".@....")
	d := joined(t, m, "ana")
	d.Player("ana").Bag["bois"] = 3
	d.spawn("ronceau", Point{2, 0}, false)
	d.Leave("ana", t0.Add(time.Second))
	if len(d.Player("ana").Bag) != 0 {
		t.Fatal("closing the game at a creature's throat should lose the bag")
	}

	d2 := joined(t, testMap(t, ".@......."), "bob")
	d2.Player("bob").Bag["bois"] = 3
	d2.spawn("ronceau", Point{6, 0}, false)
	d2.Leave("bob", t0.Add(time.Second))
	if d2.Player("bob").Bag["bois"] != 3 {
		t.Fatal("leaving with a creature at a distance is fine")
	}
}

func TestNoBedWithACreatureAround(t *testing.T) {
	m := testMap(t, "B.....", "@.....")
	d := joined(t, m, "ana")
	d.Move("ana", protocol.Up, t0)
	d.spawn("ronceau", Point{4, 1}, false)
	if r := d.Use("ana", "", t0.Add(time.Second)); r.Reason != ReasonNotNow {
		t.Fatalf("bed au pas: %+v", r)
	}
}

func TestNightWave(t *testing.T) {
	// The forest edge is on the left, the house and its domain on the right.
	m := testMap(t,
		"ff............................",
		"ff...........................B",
		"ff...........................@",
	)
	d := joined(t, m, "ana")
	night := t0.Add(onlineUntil(d, 20*60+1))

	// No crops, no wave.
	d.Tick(night)
	if len(d.creatures) != 0 {
		t.Fatal("pests only come for crops")
	}

	d2 := joined(t, m, "ana")
	d2.plots[Point{6, 1}] = &Plot{At: Point{6, 1}, State: protocol.PlotCrop, Crop: "navet", Watered: true}
	events := d2.Tick(night)
	if n := len(d2.creatures); n != d2.WaveSize() || n < 2 {
		t.Fatalf("wave of %d, want %d", n, d2.WaveSize())
	}
	if count[protocol.Notice](events) == 0 {
		t.Fatal("players are warned of the wave")
	}
	// Nobody defends: in real time the pests reach the crop and eat it.
	now := night
	for i := 0; i < 120 && d2.plots[Point{6, 1}].State == protocol.PlotCrop; i++ {
		now = now.Add(500 * time.Millisecond)
		d2.Tick(now)
	}
	if d2.plots[Point{6, 1}].State != protocol.PlotTilled {
		t.Fatal("an undefended crop should be eaten")
	}
	// At dawn the pests run off.
	d2.sleepThrough(now)
	if len(d2.creatures) != 0 {
		t.Fatal("pests leave at dawn")
	}
}

func TestEmptyValleyIsNeverAttacked(t *testing.T) {
	m := testMap(t, "ff........@")
	d := joined(t, m, "ana")
	d.plots[Point{5, 0}] = &Plot{At: Point{5, 0}, State: protocol.PlotCrop, Crop: "navet"}
	d.Leave("ana", t0)
	d.Tick(t0.Add(30 * time.Minute)) // the evening passes offline
	for _, c := range d.creatures {
		if c.Wave {
			t.Fatal("no wave in an empty valley")
		}
	}
}

func TestFencesAndScarecrows(t *testing.T) {
	m := testMap(t, ".@........")
	d := joined(t, m, "ana")
	d.addBuilding(&Building{Kind: BuildingFence, At: Point{5, 0}, W: 1, H: 1, HP: 4})
	d.addBuilding(&Building{Kind: BuildingScarecrow, At: Point{9, 0}, W: 1, H: 1})
	walker, _ := d.spawn("ronceau", Point{4, 0}, false)
	rat, _ := d.spawn("rongeur-racine", Point{6, 0}, false)
	crow, _ := d.spawn("corbeau-voleur", Point{3, 0}, false)
	fence := Point{5, 0}
	if d.passable(walker, fence, false) {
		t.Fatal("a fence stops a walker")
	}
	if !d.passable(rat, fence, false) {
		t.Fatal("a burrowed rodent goes under a fence")
	}
	if !d.passable(crow, fence, false) {
		t.Fatal("a crow flies over a fence")
	}
	if d.passable(crow, Point{7, 0}, false) {
		t.Fatal("a crow keeps 3 tiles from a scarecrow")
	}

	walls := testMap(t, ".@#~.")
	d2 := joined(t, walls, "ana")
	crow2, _ := d2.spawn("corbeau-voleur", Point{4, 0}, false)
	if d2.passable(crow2, Point{2, 0}, false) || !d2.passable(crow2, Point{3, 0}, false) {
		t.Fatal("a crow flies over water but not through walls")
	}
}

func TestWalledInWalkerBreaksTheFence(t *testing.T) {
	m := testMap(t, "......#", ".@.....", "......#")
	d := joined(t, m, "ana")
	for _, at := range []Point{{4, 0}, {4, 1}, {4, 2}} {
		d.addBuilding(&Building{Kind: BuildingFence, At: at, W: 1, H: 1, HP: 2})
	}
	c, _ := d.spawn("ronceau", Point{6, 1}, false)
	s := &steps{t0}
	for i := 0; i < 20 && d.bAt[Point{4, 1}] != nil; i++ {
		d.Wait("ana", s.next())
	}
	if d.bAt[Point{4, 1}] != nil {
		t.Fatalf("the bramble should break through the fence (at %v)", c.Pos)
	}
}

// onlineUntil is the real time from the first morning to minute m of day 1
// with someone in the valley.
func onlineUntil(d *Domain, m int) time.Duration {
	game := float64(m - d.content.Time.WakeHour*60)
	return time.Duration(game / minutesPerDay * d.content.Time.OnlineDaySeconds * float64(time.Second))
}
