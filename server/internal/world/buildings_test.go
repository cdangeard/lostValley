package world

import (
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

func TestBuildWorkshopThenTheRest(t *testing.T) {
	m := testMap(t,
		"..........",
		".@........",
		"..........",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	s := &steps{t0}
	d.Move("ana", protocol.Right, s.next()) // ana at (2,1), facing (3,1)

	if r := d.Build("ana", "puits", Point{3, 1}, s.next()); r.Reason != ReasonNeedsPlan {
		t.Fatalf("well before workshop: %+v", r)
	}
	if r := d.Build("ana", "atelier", Point{3, 1}, s.next()); r.Reason != ReasonCost {
		t.Fatalf("workshop without materials: %+v", r)
	}
	ana.Bag["bois"], ana.Bag["pierre"] = 40, 30
	if r := d.Build("ana", "atelier", Point{5, 1}, s.next()); r.Reason != ReasonCannotBuild {
		t.Fatalf("a footprint away from the faced tile: %+v", r)
	}
	if r := d.Build("ana", "atelier", Point{2, 0}, s.next()); r.Reason != ReasonCannotBuild {
		t.Fatalf("a footprint over the player: %+v", r)
	}
	if r := d.Build("ana", "atelier", Point{3, 0}, s.next()); !r.OK {
		t.Fatalf("workshop: %+v", r)
	}
	if ana.Bag["bois"] != 20 || ana.Bag["pierre"] != 20 {
		t.Fatalf("cost not paid: %v", ana.Bag)
	}
	if d.Walkable(Point{4, 1}) {
		t.Fatal("buildings block")
	}
	if r := d.Use("ana", "houe", s.next()); !r.OK || count[protocol.Notice](r.Events) != 1 {
		t.Fatalf("facing the workshop opens the build menu: %+v", r)
	}

	// The well refills the can.
	d.Move("ana", protocol.Down, s.next()) // (2,2), facing (2,3): off the map
	d.Move("ana", protocol.Left, s.next()) // (1,2) facing (0,2)
	ana.Water = 0
	if r := d.Build("ana", "puits", Point{0, 2}, s.next()); !r.OK {
		t.Fatalf("well: %+v", r)
	}
	if r := d.Use("ana", "arrosoir", s.next()); !r.OK || ana.Water != d.content.WateringCanSize {
		t.Fatalf("well refill: %+v water %d", r, ana.Water)
	}

	// Taking things down gives the materials back, but the workshop stays
	// while others depend on it.
	before := ana.Bag["pierre"]
	if r := d.Dismantle("ana", s.next()); !r.OK || ana.Bag["pierre"] != before+12 {
		t.Fatalf("dismantle well: %+v, pierre %d", r, ana.Bag["pierre"])
	}
}

func TestBuildingOnlyInsideTheDomain(t *testing.T) {
	m := testMap(t, ".@........................")
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	ana.Bag["bois"], ana.Bag["pierre"] = 40, 30
	ana.Pos = Point{20, 0}
	ana.Facing = protocol.Right
	if d.Claimed(Point{21, 0}) {
		t.Fatal("the starting domain is 12 tiles wide")
	}
	if r := d.Build("ana", "atelier", Point{21, 0}, t0.Add(time.Second)); r.Reason != ReasonCannotBuild {
		t.Fatalf("outside the domain: %+v", r)
	}
}

func TestUprootingGrowsTheDomain(t *testing.T) {
	m := testMap(t, ".@..................T.....")
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	tree := Point{20, 0}
	// Claim up to the tree's neighbourhood, as if the player had cleared there.
	for x := 0; x <= 17; x++ {
		d.setClaim(Point{x, 0})
	}
	ana.Pos, ana.Facing = Point{19, 0}, protocol.Right
	s := &steps{t0}
	for i := 0; i < ResourceKinds["arbre"].Hits; i++ {
		d.Use("ana", "hache", s.next())
	}
	r := d.Use("ana", "pioche", s.next())
	if !r.OK || count[protocol.Claim](r.Events) != 1 {
		t.Fatalf("uproot: %+v", r)
	}
	if !d.Claimed(tree) || !d.Claimed(Point{22, 0}) || d.Claimed(Point{23, 0}) {
		t.Fatal("the domain should grow 2 tiles around the stump")
	}
}

func TestDryingRack(t *testing.T) {
	m := testMap(t, ".@....")
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	d.addBuilding(&Building{Kind: BuildingRack, At: Point{2, 0}, W: 2, H: 1})
	d.Move("ana", protocol.Right, t0)
	ana.Bag["navet"], ana.Bag["navet-lune"] = 9, 2
	s := &steps{t0}
	if r := d.Use("ana", "houe", s.next()); !r.OK {
		t.Fatalf("hang crops: %+v", r)
	}
	if ana.Bag["navet"] != 3 || ana.Bag["navet-lune"] != 2 {
		t.Fatalf("the rack takes 6 plain crops and leaves rare ones: %v", ana.Bag)
	}
	d.sleepThrough(s.next())
	d.Use("ana", "", s.next())
	if ana.Bag["ration"] != 6 {
		t.Fatalf("rations: %v", ana.Bag)
	}
	ana.HP = 2
	if r := d.Use("ana", "ration", s.next()); !r.OK || ana.HP != 7 {
		t.Fatalf("eat: %+v hp %d", r, ana.HP)
	}
}

func TestChest(t *testing.T) {
	m := testMap(t, "C@...")
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	ana.Bag["bois"] = 5
	s := &steps{t0}
	if r := d.ChestMove("ana", protocol.ChestPut, "bois", 0, s.next()); r.OK {
		t.Fatal("the chest must be faced")
	}
	d.Move("ana", protocol.Left, s.next())
	if r := d.Use("ana", "", s.next()); !r.OK || count[protocol.Chest](r.Events) != 1 {
		t.Fatalf("open chest: %+v", r)
	}
	if r := d.ChestMove("ana", protocol.ChestPut, "bois", 3, s.next()); !r.OK || d.chest["bois"] != 3 || ana.Bag["bois"] != 2 {
		t.Fatalf("put: %+v", r)
	}
	if r := d.ChestMove("ana", protocol.ChestTake, "bois", 0, s.next()); !r.OK || ana.Bag["bois"] != 5 || d.chest["bois"] != 0 {
		t.Fatalf("take all: %+v", r)
	}
}

func TestPhase2SnapshotRoundTrip(t *testing.T) {
	m := testMap(t, "C@...T", "......")
	d := joined(t, m, "ana")
	d.addBuilding(&Building{Kind: BuildingFence, At: Point{3, 0}, W: 1, H: 1, HP: 2, Owner: "ana"})
	d.addBuilding(&Building{Kind: BuildingRack, At: Point{1, 1}, W: 2, H: 1, Load: 3})
	d.chest["pierre"] = 7
	d.resources[0].Depleted, d.resources[0].Removed = true, true
	delete(d.resAt, d.resources[0].At)
	d.Player("ana").HP = 4

	snap, _ := d.Snapshot(t0)
	d2 := NewDomain(m, testContent(t), 2)
	d2.Restore(snap)
	if len(d2.buildings) != 2 || d2.bAt[Point{3, 0}].HP != 2 || d2.bAt[Point{2, 1}].Load != 3 {
		t.Fatalf("buildings: %+v", d2.buildings)
	}
	if d2.chest["pierre"] != 7 || d2.Player("ana").HP != 4 || d2.claimString() != d.claimString() {
		t.Fatal("chest, health or domain lost")
	}
	if d2.resAt[Point{5, 0}] != nil {
		t.Fatal("an uprooted tree stays gone")
	}
	// New buildings keep fresh IDs.
	d2.addBuilding(&Building{Kind: BuildingFence, At: Point{4, 0}, W: 1, H: 1})
	if len(d2.buildings) != 3 {
		t.Fatal("building IDs collided after restore")
	}
}
