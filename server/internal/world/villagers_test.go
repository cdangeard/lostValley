package world

import (
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// villageDomain is a valley with a workshop, where Bram settles the next
// morning. Ana stands facing him.
func villageDomain(t *testing.T) (*Domain, *Villager, time.Time) {
	t.Helper()
	m := testMap(t,
		"..........",
		".@........",
		"..........",
		"....B....C",
		"..........",
	)
	c := testContent(t)
	bram := c.Villagers["bram"]
	bram.Place = [2]int{1, 2} // just south of the spawn
	c.Villagers["bram"] = bram
	d := NewDomain(m, c, 1)
	now := t0
	d.Join("ana", "Ana", now)
	d.addBuilding(&Building{ID: 1, Kind: BuildingWorkshop, At: Point{6, 0}, W: 2, H: 2})
	if len(d.villagers) != 0 {
		t.Fatal("nobody comes before the morning")
	}
	events := d.sleepThrough(now)
	if count[protocol.Villager](events) == 0 {
		t.Fatal("no villager arrived")
	}
	v := d.villagers["bram"]
	if v == nil || len(d.villagers) != 1 {
		t.Fatalf("villagers: %+v", d.villagers)
	}
	ana := d.Player("ana")
	ana.Pos, ana.Facing = Point{1, 1}, protocol.Down
	v.Pos, v.Spot = Point{1, 2}, Point{1, 2}
	return d, v, now
}

func lastDialogue(t *testing.T, r Result) protocol.Dialogue {
	t.Helper()
	if !r.OK {
		t.Fatalf("refused: %+v", r)
	}
	for _, e := range r.Events {
		if dl, ok := e.Msg.(protocol.Dialogue); ok {
			return dl
		}
	}
	t.Fatalf("no dialogue in %+v", r.Events)
	return protocol.Dialogue{}
}

// nextDay runs a morning, so chats and gifts count again.
func nextDay(d *Domain, now time.Time) time.Time {
	d.minutes += 60
	d.sleepThrough(now)
	return now.Add(time.Second)
}

func TestVillagersArriveOneByOneAsTheDomainGrows(t *testing.T) {
	d := NewDomain(testMap(t, "@........."), testContent(t), 1)
	d.Join("ana", "Ana", t0)
	d.sleepThrough(t0)
	if len(d.villagers) != 0 {
		t.Fatal("nobody should come to an empty valley")
	}
	d.addBuilding(&Building{ID: 1, Kind: BuildingWorkshop, At: Point{5, 0}, W: 2, H: 1})
	d.creatures[99] = &Creature{ID: 99, Kind: "chevre-lichen", Pos: Point{9, 0}, Owner: "ana"}
	d.minutes += 60
	d.sleepThrough(t0)
	if len(d.villagers) != 1 || d.villagers["bram"] == nil {
		t.Fatalf("first morning: %v", d.villagers)
	}
	d.minutes += 60
	d.sleepThrough(t0)
	if len(d.villagers) != 2 || d.villagers["pimprenelle"] == nil {
		t.Fatalf("second morning: %v", d.villagers)
	}
	if d.villagers["ysolde"] != nil {
		t.Fatal("Ysolde waits for the forest to recede")
	}
}

func TestTalkingAndGifts(t *testing.T) {
	d, v, now := villageDomain(t)
	ana := d.Player("ana")
	r := d.content.Romance

	dl := lastDialogue(t, d.Use("ana", "", now))
	if dl.Mood != protocol.MoodTalk || dl.Delta != r.Talk {
		t.Fatalf("first chat: %+v", dl)
	}
	dl = lastDialogue(t, d.Use("ana", "", now.Add(time.Second)))
	if dl.Delta != 0 {
		t.Fatalf("a second chat the same day should not count: %+v", dl)
	}

	ana.Bag["eclat-de-carapace"] = 2
	ana.Bag["fibre"] = 2
	dl = lastDialogue(t, d.Use("ana", "eclat-de-carapace", now.Add(2*time.Second)))
	if dl.Mood != protocol.MoodLove || dl.Delta != r.Gift["adore"] || ana.Bag["eclat-de-carapace"] != 1 {
		t.Fatalf("loved gift: %+v", dl)
	}
	dl = lastDialogue(t, d.Use("ana", "fibre", now.Add(3*time.Second)))
	if dl.Mood != protocol.MoodAlready || ana.Bag["fibre"] != 2 {
		t.Fatalf("second gift the same day: %+v", dl)
	}
	now = nextDay(d, now)
	dl = lastDialogue(t, d.Use("ana", "fibre", now))
	if dl.Mood != protocol.MoodHate || dl.Delta != r.Gift["deteste"] {
		t.Fatalf("hated gift: %+v", dl)
	}
	// A tool in hand is a chat, not a gift.
	dl = lastDialogue(t, d.Use("ana", "houe", now.Add(time.Second)))
	if dl.Mood != protocol.MoodTalk && dl.Mood != protocol.MoodScene {
		t.Fatalf("tool in hand: %+v", dl)
	}
	if b := v.Bonds["ana"]; b.Points != r.Talk*2+r.Gift["adore"]+r.Gift["deteste"] {
		t.Fatalf("points: %d", b.Points)
	}
}

func TestHeartScenesPlayOnce(t *testing.T) {
	d, v, now := villageDomain(t)
	b := d.bondWith(v, "ana")
	b.Points = 2*d.content.Romance.PointsPerHeart - d.content.Romance.Talk
	dl := lastDialogue(t, d.Use("ana", "", now))
	if dl.Mood != protocol.MoodScene || len(dl.Lines) < 2 {
		t.Fatalf("2-heart scene: %+v", dl)
	}
	dl = lastDialogue(t, d.Use("ana", "", now.Add(time.Second)))
	if dl.Mood != protocol.MoodTalk {
		t.Fatalf("the scene played twice: %+v", dl)
	}
}

func TestRomanceFromBouquetToWedding(t *testing.T) {
	d, v, now := villageDomain(t)
	r := d.content.Romance
	ana := d.Player("ana")
	b := d.bondWith(v, "ana")
	b.Seen = []int{2, 4, 6, 8}

	// Friendship stops at 8 hearts until the bouquet.
	b.Points = r.RomanceHearts*r.PointsPerHeart - 10
	ana.Bag["eclat-de-carapace"] = 1
	d.Use("ana", "eclat-de-carapace", now)
	if d.hearts(b) != r.RomanceHearts {
		t.Fatalf("hearts %d, points %d", d.hearts(b), b.Points)
	}

	// The bouquet is made at the workshop.
	ana.Bag["fibre"], ana.Bag["feve"] = 10, 5
	if res := d.Craft("ana", r.Bouquet, now); !res.OK || ana.Bag[r.Bouquet] != 1 {
		t.Fatalf("craft: %+v", res)
	}
	dl := lastDialogue(t, d.Use("ana", r.Bouquet, now.Add(time.Second)))
	if dl.Mood != protocol.MoodBouquet || b.Stage != protocol.StageCouple || ana.Bag[r.Bouquet] != 0 {
		t.Fatalf("bouquet: %+v, stage %q", dl, b.Stage)
	}

	// The pendant waits for 10 hearts.
	ana.Bag[r.Pendant] = 1
	dl = lastDialogue(t, d.Use("ana", r.Pendant, now.Add(2*time.Second)))
	if dl.Mood != protocol.MoodRefuse || ana.Bag[r.Pendant] != 1 {
		t.Fatalf("pendant too soon: %+v", dl)
	}
	b.Points = r.MaxHearts * r.PointsPerHeart
	dl = lastDialogue(t, d.Use("ana", r.Pendant, now.Add(3*time.Second)))
	if dl.Mood != protocol.MoodPendant || b.Stage != protocol.StageEngaged {
		t.Fatalf("pendant: %+v", dl)
	}

	// The wedding comes a few mornings later; the spouse moves in and helps.
	d.plots[Point{5, 4}] = &Plot{At: Point{5, 4}, State: protocol.PlotCrop, Crop: "navet"}
	wedding := b.Wedding
	for d.Day() < wedding {
		if b.Stage == protocol.StageMarried {
			t.Fatal("married too early")
		}
		now = nextDay(d, now)
	}
	if b.Stage != protocol.StageMarried || v.Spouse != "ana" {
		t.Fatalf("not married on day %d: %+v", d.Day(), b)
	}
	if cheb(v.Spot, d.Map.WakeSpot()) > 2 {
		t.Fatalf("the spouse lives at %v, the bed is at %v", v.Spot, d.Map.WakeSpot())
	}
	d.buildings[1].HP = 0
	fence := &Building{ID: 2, Kind: BuildingFence, At: Point{8, 4}, W: 1, H: 1, HP: 1}
	d.addBuilding(fence)
	now = nextDay(d, now)
	if fence.HP != d.content.Buildings[BuildingFence].HP {
		t.Fatalf("Bram should mend the fence: hp %d", fence.HP)
	}
	if line := d.dailyLine(v, b, d.Day()); line == "" {
		t.Fatal("no married line")
	}
}

func TestOneRomanceAtATime(t *testing.T) {
	d, v, now := villageDomain(t)
	r := d.content.Romance
	d.Join("bob", "Bob", now)
	bob := d.Player("bob")
	bob.Pos, bob.Facing = Point{0, 2}, protocol.Right
	d.bondWith(v, "ana").Stage = protocol.StageCouple
	bb := d.bondWith(v, "bob")
	bb.Points, bb.Seen = r.RomanceHearts*r.PointsPerHeart, []int{2, 4, 6, 8}
	bob.Bag[r.Bouquet] = 1
	dl := lastDialogue(t, d.Use("bob", r.Bouquet, now.Add(time.Second)))
	if dl.Mood != protocol.MoodRefuse || bb.Stage != protocol.StageNone || bob.Bag[r.Bouquet] != 1 {
		t.Fatalf("a taken villager accepted a bouquet: %+v", dl)
	}
}

func TestCraftNeedsTheWorkshop(t *testing.T) {
	d := NewDomain(testMap(t, "@...."), testContent(t), 1)
	d.Join("ana", "Ana", t0)
	d.Player("ana").Bag["fibre"], d.Player("ana").Bag["feve"] = 10, 5
	if r := d.Craft("ana", "bouquet-de-lierre", t0); r.Reason != ReasonNeedsPlan {
		t.Fatalf("crafted without a workshop: %+v", r)
	}
}

func TestVillagersAreSaved(t *testing.T) {
	d, v, now := villageDomain(t)
	b := d.bondWith(v, "ana")
	b.Points, b.Stage, b.Seen = 850, protocol.StageCouple, []int{2, 4}
	snap, _ := d.Snapshot(now)
	e := NewDomain(d.Map, d.content, 2)
	e.Restore(snap)
	got := e.villagers["bram"]
	if got == nil || got.Pos != v.Pos || got.Bonds["ana"].Points != 850 || got.Bonds["ana"].Stage != protocol.StageCouple ||
		len(got.Bonds["ana"].Seen) != 2 {
		t.Fatalf("restored %+v", got)
	}
	if w := e.Welcome("ana", "test"); len(w.Villagers) != 1 || len(w.Bonds) != 1 || w.Bonds[0].Hearts != 8 {
		t.Fatalf("welcome: %+v %+v", w.Villagers, w.Bonds)
	}
}
