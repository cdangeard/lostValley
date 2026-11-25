package world

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// levelUp gives p enough experience for the given level.
func levelUp(d *Domain, p *Player, level int) {
	p.XP = d.content.XPForLevel(level)
	p.Level = level
}

func TestConstellationFile(t *testing.T) {
	c := testContent(t)
	tree := c.Constellation
	if tree == nil {
		t.Fatal("contenu.json should name the constellation")
	}
	if n := len(tree.Nodes); n < 55 || n > 70 {
		t.Fatalf("%d nodes, the v1 aims at about 60", n)
	}
	// Distance from the heart, in points.
	dist := map[string]int{tree.Heart: 0}
	queue := []string{tree.Heart}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, m := range tree.adj[n] {
			if _, seen := dist[m]; !seen {
				dist[m] = dist[n] + 1
				queue = append(queue, m)
			}
		}
	}
	keystones, regions := 0, map[string]int{}
	for _, n := range tree.Nodes {
		regions[n.Region]++
		if n.Type == NodeKeystone {
			keystones++
			if dist[n.ID] > 6 {
				t.Errorf("keystone %s is %d points away; a tester should reach it in a session", n.ID, dist[n.ID])
			}
		}
		if n.Type != NodeHeart && len(n.Mods) == 0 {
			t.Errorf("node %s does nothing", n.ID)
		}
		if (n.Type == NodeNotable || n.Type == NodeKeystone) && n.Description == "" {
			t.Errorf("node %s needs a description", n.ID)
		}
	}
	if keystones != 2 {
		t.Fatalf("%d keystones, want 2", keystones)
	}
	for _, r := range []string{"terre", "sylve", "forge", "lame"} {
		if regions[r] < 12 {
			t.Errorf("region %s has %d nodes", r, regions[r])
		}
	}
}

func TestConstellationChecks(t *testing.T) {
	bad := []string{
		`{"noeuds":[{"id":"a","type":"petit"}],"liens":[]}`,                                                     // no heart
		`{"noeuds":[{"id":"c","type":"coeur"},{"id":"a","type":"petit"}],"liens":[]}`,                           // cut off
		`{"noeuds":[{"id":"c","type":"coeur"},{"id":"a","type":"petit","mods":{"vol":1}}],"liens":[["c","a"]]}`, // unknown mod
		`{"noeuds":[{"id":"c","type":"coeur"}],"liens":[["c","x"]]}`,                                            // unknown node
	}
	for _, raw := range bad {
		if _, err := ParseConstellation([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestLevels(t *testing.T) {
	c := testContent(t)
	if c.XPForLevel(1) != 0 || c.XPForLevel(2) != c.Progress.FirstLevelXP ||
		c.XPForLevel(3) != 2*c.Progress.FirstLevelXP+c.Progress.LevelStepXP {
		t.Fatalf("level curve: %d %d %d", c.XPForLevel(1), c.XPForLevel(2), c.XPForLevel(3))
	}
	d := joined(t, testMap(t, ".@.."), "ana")
	ana := d.Player("ana")
	var events []Event
	for ana.Level == 1 {
		events = append(events, d.gainXP(ana, XPBuild)...)
	}
	if ana.Points() != 1 {
		t.Fatalf("points = %d at level 2", ana.Points())
	}
	ups := 0
	for _, e := range events {
		if n, ok := e.Msg.(protocol.Notice); ok && n.Code == protocol.NoticeLevelUp && n.Arg == "2" {
			ups++
		}
	}
	if ups != 1 {
		t.Fatalf("want one level-up notice, got %d", ups)
	}
}

func TestTakeAndGiveBackTalents(t *testing.T) {
	d := joined(t, testMap(t, ".@.."), "ana")
	ana := d.Player("ana")
	now := t0.Add(time.Second)
	if r := d.Talent("ana", "lame-t1", protocol.TalentTake, now); r.Reason != ReasonNoPoint {
		t.Fatalf("level 1 has no point: %+v", r)
	}
	levelUp(d, ana, 3)
	if r := d.Talent("ana", "lame-t2", protocol.TalentTake, now); r.Reason != ReasonNotLinked {
		t.Fatalf("lame-t2 is not next to the heart: %+v", r)
	}
	for _, n := range []string{"lame-t1", "lame-t2"} {
		if r := d.Talent("ana", n, protocol.TalentTake, now); !r.OK {
			t.Fatalf("take %s: %+v", n, r)
		}
	}
	if ana.Points() != 0 {
		t.Fatalf("points = %d", ana.Points())
	}
	if d.maxHP(ana) != d.content.Health.Max+2 {
		t.Fatalf("Peau dure should add 2 health, max = %d", d.maxHP(ana))
	}
	if r := d.Talent("ana", "lame-t1", protocol.TalentRefund, now); r.Reason != ReasonNotLeaf {
		t.Fatalf("lame-t1 holds lame-t2 to the heart: %+v", r)
	}
	seeds := ana.Bag[OblivionSeed]
	if seeds < 1 {
		t.Fatal("the starter bag should hold a seed of oblivion")
	}
	if r := d.Talent("ana", "lame-t2", protocol.TalentRefund, now); !r.OK {
		t.Fatalf("give back lame-t2: %+v", r)
	}
	if ana.Bag[OblivionSeed] != seeds-1 || ana.Points() != 1 || ana.Talents["lame-t2"] {
		t.Fatalf("after giving back: seeds %d, points %d", ana.Bag[OblivionSeed], ana.Points())
	}
	ana.Bag[OblivionSeed] = 0
	if r := d.Talent("ana", "lame-t1", protocol.TalentRefund, now); r.Reason != ReasonNoOblivion {
		t.Fatalf("no seed, no refund: %+v", r)
	}
	if ana.HP > d.maxHP(ana) {
		t.Fatal("health above the max")
	}
}

// take gives p the path of nodes, levelling up as needed.
func take(t *testing.T, d *Domain, p *Player, nodes ...string) {
	t.Helper()
	levelUp(d, p, p.Level+len(nodes))
	for _, n := range nodes {
		if r := d.Talent(p.ID, n, protocol.TalentTake, t0); !r.OK {
			t.Fatalf("take %s: %+v", n, r)
		}
	}
}

var greenThumbPath = []string{"terre-t1", "terre-t2", "terre-t3", "terre-m1", "terre-m2", "terre-m3"}
var packLeaderPath = []string{"sylve-t1", "sylve-t2", "sylve-t3", "sylve-m1", "sylve-m2", "sylve-m3"}

func TestGreenThumb(t *testing.T) {
	m := testMap(t,
		".@..",
		"....",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	take(t, d, ana, greenThumbPath...)
	if ana.mod(ModGreenThumb) == 0 {
		t.Fatal("keystone not taken")
	}
	now := t0.Add(time.Second)
	at := Point{1, 1}
	ana.Facing = protocol.Down
	d.Use("ana", "houe", now)
	now = now.Add(time.Second)
	if r := d.Use("ana", "graine-navet", now); !r.OK {
		t.Fatalf("sow: %+v", r)
	}
	now = now.Add(time.Second)
	if r := d.Use("ana", "arrosoir", now); r.Reason != ReasonGreenThumb {
		t.Fatalf("the can should not water: %+v", r)
	}
	plot := d.plots[at]
	days := d.crop(plot).Days
	for i := 0; i < 2*days; i++ {
		d.morning(now, false)
	}
	if !d.ripe(plot) {
		t.Fatalf("growth %d/%d after %d dry days: one in two should count", plot.Growth, days, 2*days)
	}
	before := ana.Bag["navet"]
	d.harvest(ana, plot)
	if got := ana.Bag["navet"] - before; got < 2*d.content.Crops["navet"].Amount {
		t.Fatalf("harvest %d, want double", got)
	}
}

func TestPackLeader(t *testing.T) {
	m := testMap(t, ".@...")
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	take(t, d, ana, packLeaderPath...)
	c, _ := d.spawn("ronceau", Point{2, 0}, false)
	still(c)
	ana.Facing = protocol.Right
	hp := c.HP
	if r := d.Use("ana", "hache", t0.Add(time.Second)); !r.OK {
		t.Fatalf("swing: %+v", r)
	}
	if c.HP != hp {
		t.Fatalf("a Meneur de meute does no damage himself: %d → %d", hp, c.HP)
	}
	owl := tameAt(t, d, "hibou-lanterne", Point{4, 0}, ana)
	before, want := c.HP, max(1, 3*d.content.Creatures["hibou-lanterne"].Attack.Damage-c.Armor)
	d.petStrike(owl, c)
	if dealt := before - c.HP; dealt != want {
		t.Fatalf("pack damage %d, want %d (tripled)", dealt, want)
	}
}

func TestTalentsSaved(t *testing.T) {
	m := testMap(t,
		"ff.....",
		"ff.@...",
		"ff.....",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	take(t, d, ana, "lame-t1", "lame-t2")
	ana.XP += 7
	snap, _ := d.Snapshot(t0.Add(time.Second))
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var back Snapshot
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	d2 := NewDomain(m, testContent(t), 2)
	d2.Restore(&back)
	a2 := d2.Player("ana")
	if a2.Level != ana.Level || a2.XP != ana.XP || len(a2.Talents) != 2 || d2.maxHP(a2) != d.maxHP(ana) {
		t.Fatalf("restored level %d xp %d talents %v", a2.Level, a2.XP, a2.Talents)
	}
}
