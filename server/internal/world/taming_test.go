package world

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// wild puts a tamable wild creature at at; it does not move on its own.
func wild(d *Domain, kind string, at Point) *Creature {
	c, _ := d.spawn(kind, at, false)
	c.Fauna = true
	still(c)
	return c
}

// tameAt puts a creature already tamed by p at at, with no habitat.
func tameAt(t *testing.T, d *Domain, kind string, at Point, p *Player) *Creature {
	t.Helper()
	c, _ := d.spawn(kind, at, false)
	c.Owner, c.Mode, c.Burrowed = p.ID, protocol.ModeHome, false
	if d.content.Creatures[kind].Taming.Role == RoleCompanion {
		c.Mode = protocol.ModeFollow
	}
	c.HP = d.creatureMaxHP(c)
	return c
}

// habitat builds kind with its top-left corner at at.
func habitat(d *Domain, kind string, at Point) *Building {
	def := d.content.Buildings[kind]
	b := &Building{Kind: kind, At: at, W: def.W, H: def.H}
	d.addBuilding(b)
	return b
}

// hasNotice reports whether events tell to a notice with that code.
func hasNotice(events []Event, code string) bool {
	for _, e := range events {
		if n, ok := e.Msg.(protocol.Notice); ok && n.Code == code {
			return true
		}
	}
	return false
}

func TestFeedingATameGoat(t *testing.T) {
	m := testMap(t,
		"........",
		".@......",
		"........",
		"........",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	goat := wild(d, "chevre-lichen", Point{2, 1})
	ana.Facing = protocol.Right
	ana.Bag["ble"] = 10
	if d.engaged(ana) {
		t.Fatal("a peaceful goat does not put anyone au pas")
	}
	s := &steps{t0}
	r := d.Use("ana", "ble", s.next())
	if !r.OK || goat.Trust == 0 || ana.Bag["ble"] != 9 {
		t.Fatalf("feed: %+v, trust %d", r, goat.Trust)
	}
	// Not hungry again right away.
	r = d.Use("ana", "ble", s.next())
	if !hasNotice(r.Events, protocol.NoticeFull) || ana.Bag["ble"] != 9 {
		t.Fatalf("it just ate: %+v", r)
	}
	for goat.Trust < d.content.Taming.MaxTrust {
		d.minutes += float64(d.content.Taming.AppetiteMins)
		r = d.Use("ana", "ble", s.next())
	}
	if goat.Owner != "" || !hasNotice(r.Events, protocol.NoticeHabitat) {
		t.Fatalf("no pen, no bond: owner %q", goat.Owner)
	}
	pen := habitat(d, "enclos", Point{5, 1})
	r = d.Use("ana", "ble", s.next())
	if goat.Owner != "ana" || goat.Home != pen.ID || !hasNotice(r.Events, protocol.NoticeTamed) {
		t.Fatalf("tamed: owner %q home %d events %+v", goat.Owner, goat.Home, r.Events)
	}
	if cheb(goat.Pos, Point{5, 1}) > 2 {
		t.Fatalf("the goat should go to its pen, at %v", goat.Pos)
	}

	// Fed today, it leaves milk in the pen the next morning.
	ana.Pos, ana.Facing = goat.Pos.Step(protocol.Left), protocol.Right
	if d.creatureAt(ana.Pos) != nil || !d.Walkable(ana.Pos) {
		ana.Pos, ana.Facing = goat.Pos.Step(protocol.Right), protocol.Left
	}
	d.minutes += float64(d.content.Taming.AppetiteMins)
	if r := d.Use("ana", "ble", s.next()); !r.OK || !goat.Fed {
		t.Fatalf("feed the tamed goat: %+v", r)
	}
	d.morning(s.next(), false)
	if pen.Ready != 1 {
		t.Fatalf("milk in the pen = %d", pen.Ready)
	}
	d.morning(s.next(), false)
	if pen.Ready != 1 {
		t.Fatal("not fed yesterday: no milk")
	}
	if r := d.Dismantle("ana", s.next()); r.OK && d.buildings[pen.ID] == nil {
		t.Fatal("a pen with a goat in it cannot be taken down")
	}
	// Collect it.
	ana.Pos, ana.Facing = Point{5, 0}, protocol.Down
	d.players["ana"].readyAt = time.Time{}
	if r := d.Use("ana", "houe", s.next()); !r.OK || ana.Bag["lait-de-chevre"] != 1 || pen.Ready != 0 {
		t.Fatalf("collect: %+v, milk %d", r, ana.Bag["lait-de-chevre"])
	}
}

func TestHittingACalmCreatureLosesItsTrust(t *testing.T) {
	d := joined(t, testMap(t, ".@...."), "ana")
	ana := d.Player("ana")
	goat := wild(d, "chevre-lichen", Point{2, 0})
	goat.Trust = 60
	ana.Facing = protocol.Right
	r := d.Use("ana", "faux", t0.Add(time.Second))
	if !r.OK || goat.Trust != 0 || !hasNotice(r.Events, protocol.NoticeScared) {
		t.Fatalf("hit: %+v, trust %d", r, goat.Trust)
	}
}

func TestWolfMustGiveInFirst(t *testing.T) {
	m := testMap(t,
		"........",
		".@......",
		"........",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	wolf := wild(d, "loup-ronce", Point{2, 1})
	ana.Facing = protocol.Right
	ana.Bag["ration"] = 5
	habitat(d, "taniere", Point{6, 0})
	if !d.engaged(ana) {
		t.Fatal("a wild wolf is hostile")
	}
	s := &steps{t0}
	ana.HP = 3
	r := d.Use("ana", "ration", s.next())
	if wolf.Trust != 0 {
		t.Fatalf("a fighting wolf takes no food: %+v", r)
	}
	ana.HP = d.maxHP(ana)
	ana.turnAt = d.ct
	r = d.Use("ana", "hache", s.next())
	if !r.OK || !wolf.Submissive || wolf.HP <= 0 || !hasNotice(r.Events, protocol.NoticeSubdued) {
		t.Fatalf("one axe blow should subdue it: hp %d, %+v", wolf.HP, r)
	}
	if d.engaged(ana) {
		t.Fatal("a subdued wolf no longer fights")
	}
	// The axe knocked it back a tile: step up to it.
	ana.Pos = wolf.Pos.Step(protocol.Left)
	for i := 0; i < 10 && wolf.Owner == "" && ana.Bag["ration"] > 0; i++ {
		d.minutes += float64(d.content.Taming.AppetiteMins)
		ana.readyAt = time.Time{}
		d.Use("ana", "ration", s.next())
	}
	if wolf.Owner != "ana" || wolf.Mode != protocol.ModeHome || wolf.HP != d.creatureMaxHP(wolf) {
		t.Fatalf("tamed wolf: owner %q mode %q hp %d", wolf.Owner, wolf.Mode, wolf.HP)
	}
}

func TestOwlFollowsAndFights(t *testing.T) {
	m := testMap(t,
		"..............",
		".@............",
		"..............",
		"..............",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	owl := tameAt(t, d, "hibou-lanterne", Point{1, 2}, ana)
	s := &steps{t0}
	// Walk away: the owl keeps up.
	for i := 0; i < 5; i++ {
		d.Move("ana", protocol.Right, s.next())
		d.Tick(s.now.Add(600 * time.Millisecond))
		d.Tick(s.now.Add(1200 * time.Millisecond))
	}
	if cheb(owl.Pos, ana.Pos) > 2 {
		t.Fatalf("owl at %v, ana at %v", owl.Pos, ana.Pos)
	}
	// Walking into it swaps places.
	ana.Facing = dirTo(ana.Pos, owl.Pos)
	if cheb(owl.Pos, ana.Pos) == 1 && (owl.Pos.X == ana.Pos.X || owl.Pos.Y == ana.Pos.Y) {
		from := ana.Pos
		if r := d.Move("ana", ana.Facing, s.next()); !r.OK || owl.Pos != from {
			t.Fatalf("swap: %+v, owl %v", r, owl.Pos)
		}
	}
	// A crow comes: the owl goes for it and the player gets the loot.
	crow, _ := d.spawn("corbeau-voleur", Point{ana.Pos.X + 3, ana.Pos.Y}, false)
	still(crow)
	ana.Bag = map[string]int{}
	for i := 0; i < 30 && d.creatures[crow.ID] != nil; i++ {
		d.Wait("ana", s.next())
	}
	if d.creatures[crow.ID] != nil {
		t.Fatalf("the owl should kill the crow, crow hp %d", crow.HP)
	}
	if ana.Bag["plume-noire"] == 0 {
		t.Fatal("the loot goes to the owl's player")
	}
	// The player's own swings never touch it.
	ana.Facing = dirTo(ana.Pos, owl.Pos)
	hp := owl.HP
	d.Use("ana", "faux", s.next())
	if owl.HP != hp {
		t.Fatal("ana hit her own owl")
	}
}

func TestFollowerLimit(t *testing.T) {
	d := joined(t, testMap(t, ".@....", "......"), "ana")
	ana := d.Player("ana")
	tameAt(t, d, "hibou-lanterne", Point{2, 1}, ana)
	second := tameAt(t, d, "hibou-lanterne", Point{1, 0}.Step(protocol.Left), ana)
	second.Mode = protocol.ModeHome
	ana.Facing = protocol.Left
	r := d.Use("ana", "houe", t0.Add(time.Second))
	if second.Mode != protocol.ModeHome || !hasNotice(r.Events, protocol.NoticeTooMany) {
		t.Fatalf("one follower at first: %+v", r)
	}
	take(t, d, ana, "sylve-t1", "sylve-t2", "sylve-t3", "sylve-m1", "sylve-m2") // Lien: +1
	ana.readyAt = time.Time{}
	r = d.Use("ana", "houe", t0.Add(2*time.Second))
	if second.Mode != protocol.ModeFollow {
		t.Fatalf("Lien allows a second follower: %+v", r)
	}
	// Giving the point back sends one home.
	ana.Bag[OblivionSeed] = 1
	d.Talent("ana", "sylve-m2", protocol.TalentRefund, t0.Add(3*time.Second))
	if len(d.followers(ana)) != 1 {
		t.Fatalf("followers = %d", len(d.followers(ana)))
	}
}

func TestGuardWolfDefendsAtNight(t *testing.T) {
	m := testMap(t,
		"ff..........",
		"ff..........",
		"ff..........",
		"ff.......@B.",
		"ff..........",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	den := habitat(d, "taniere", Point{5, 1})
	wolf := tameAt(t, d, "loup-ronce", d.homeSpot(&Creature{Home: den.ID, Kind: "loup-ronce"}), ana)
	wolf.Home = den.ID
	pest, _ := d.spawn("corbeau-voleur", Point{3, 1}, true)
	s := &steps{t0}
	for i := 0; i < 80 && d.creatures[pest.ID] != nil; i++ {
		d.Tick(s.next())
	}
	if d.creatures[pest.ID] != nil {
		t.Fatalf("the wolf should guard its den: pest at %v hp %d, wolf at %v", pest.Pos, pest.HP, wolf.Pos)
	}
}

func TestWornOutPetRestsUntilMorning(t *testing.T) {
	m := testMap(t, "..........", ".@.......B", "..........")
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	owl := tameAt(t, d, "hibou-lanterne", Point{5, 0}, ana)
	events := d.hurtPet(owl, 100)
	if !owl.Resting || owl.HP <= 0 || !hasNotice(events, protocol.NoticeTired) {
		t.Fatalf("worn out: resting %v hp %d", owl.Resting, owl.HP)
	}
	d.morning(t0.Add(time.Second), false)
	if owl.Resting || owl.HP != d.creatureMaxHP(owl) {
		t.Fatal("rested by morning")
	}
}

func TestFaunaComesAndGoes(t *testing.T) {
	m := testMap(t,
		"ffff..........",
		"ffff..........",
		"ffff.......@B.",
		"ffff..........",
	)
	c := testContent(t)
	c.Fauna = []FaunaSpawn{{"chevre-lichen", 1}, {"loup-ronce", 1}, {"hibou-lanterne", 1}}
	d := NewDomain(m, c, 1)
	d.Join("ana", "ana", t0)
	kinds := func() map[string]int {
		out := map[string]int{}
		for _, cr := range d.creatures {
			out[cr.Kind]++
		}
		return out
	}
	if k := kinds(); k["chevre-lichen"] != 1 || k["loup-ronce"] != 1 || k["hibou-lanterne"] != 0 {
		t.Fatalf("by day: %v", k)
	}
	d.minutes = float64(d.Day()-1)*minutesPerDay + 18*60 + 5
	d.Tick(t0)
	if k := kinds(); k["hibou-lanterne"] != 1 {
		t.Fatalf("at night the owl comes out: %v", k)
	}
	d.sleepThrough(t0)
	if k := kinds(); k["hibou-lanterne"] != 0 {
		t.Fatalf("at dawn the wild owl leaves: %v", k)
	}
}

func TestPetsSaved(t *testing.T) {
	m := testMap(t,
		"..........",
		".@........",
		"..........",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	pen := habitat(d, "enclos", Point{6, 0})
	goat := tameAt(t, d, "chevre-lichen", Point{6, 2}, ana)
	goat.Home, goat.Fed = pen.ID, true
	tameAt(t, d, "hibou-lanterne", Point{2, 1}, ana)
	snap, _ := d.Snapshot(t0.Add(time.Second))
	raw, _ := json.Marshal(snap)
	var back Snapshot
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	d2 := NewDomain(m, testContent(t), 3)
	d2.Restore(&back)
	pets := map[string]*Creature{}
	for _, c := range d2.creatures {
		pets[c.Kind] = c
	}
	g, o := pets["chevre-lichen"], pets["hibou-lanterne"]
	if g == nil || o == nil || g.Owner != "ana" || g.Home != pen.ID || !g.Fed || o.Mode != protocol.ModeFollow {
		t.Fatalf("restored pets: %+v %+v", g, o)
	}
}
