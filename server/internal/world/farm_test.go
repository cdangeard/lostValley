package world

import (
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// clockAt is the real time at which an online domain created at t0 reaches
// the given number of game minutes after the first morning.
func onlineAfter(d *Domain, gameMinutes float64) time.Time {
	secs := gameMinutes / minutesPerDay * d.content.Time.OnlineDaySeconds
	return t0.Add(time.Duration(secs * float64(time.Second)))
}

// farmer stands at the spawn of a map with a field to the right, facing it.
func farmer(t *testing.T, rows ...string) (*Domain, *Player) {
	t.Helper()
	d := joined(t, testMap(t, rows...), "ana")
	p := d.Player("ana")
	d.Move("ana", protocol.Right, t0) // step right onto the field's edge
	p.Pos = d.Map.Spawn
	p.Facing = protocol.Right
	p.readyAt = time.Time{}
	return d, p
}

func mustUse(t *testing.T, d *Domain, item string, now time.Time) {
	t.Helper()
	d.Player("ana").readyAt = time.Time{}
	if r := d.Use("ana", item, now); !r.OK {
		t.Fatalf("use %s: %+v", item, r)
	}
}

func TestTillSowWaterGrowHarvest(t *testing.T) {
	d, p := farmer(t, "@..")
	field := Point{1, 0}

	mustUse(t, d, "houe", t0)
	if pl := d.plots[field]; pl == nil || pl.State != protocol.PlotTilled {
		t.Fatalf("plot after hoe: %+v", pl)
	}
	if p.Energy != d.content.Energy.Max-d.content.Tools["houe"].Energy {
		t.Fatalf("energy = %d", p.Energy)
	}
	seeds := p.Bag["graine-navet"]
	mustUse(t, d, "graine-navet", t0)
	if p.Bag["graine-navet"] != seeds-1 || d.plots[field].Crop != "navet" {
		t.Fatalf("after sowing: bag %v plot %+v", p.Bag, d.plots[field])
	}

	days := d.content.Crops["navet"].Days
	for i := 0; i < days; i++ {
		mustUse(t, d, "arrosoir", t0)
		if r := d.Use("ana", "arrosoir", t0.Add(time.Second)); r.OK {
			t.Fatal("watering twice in a day should do nothing")
		}
		d.growPlots()
	}
	if !d.ripe(d.plots[field]) {
		t.Fatalf("navet not ripe after %d watered days: %+v", days, d.plots[field])
	}
	// Harvest works whatever is in hand.
	mustUse(t, d, "", t0)
	if p.Bag["navet"] != d.content.Crops["navet"].Amount || d.plots[field].State != protocol.PlotTilled {
		t.Fatalf("after harvest: bag %v plot %+v", p.Bag, d.plots[field])
	}
}

func TestToolRules(t *testing.T) {
	d, p := farmer(t, "@T.", "~..")
	if r := d.Use("ana", "pioche", t0); r.Reason != ReasonWrongTool {
		t.Fatalf("pickaxe on a tree: %+v", r)
	}
	if r := d.Use("ana", "houe", t0); r.Reason != ReasonWrongTool {
		t.Fatalf("hoe on a tree: %+v", r)
	}
	if r := d.Use("ana", "epee", t0); r.Reason != ReasonNoItem {
		t.Fatalf("unknown item: %+v", r)
	}
	p.Energy = 1
	if r := d.Use("ana", "hache", t0); r.Reason != ReasonExhausted {
		t.Fatalf("axe without energy: %+v", r)
	}

	// Refilling the can at the water is free.
	p.Water = 0
	p.Facing = protocol.Down
	mustUse(t, d, "arrosoir", t0)
	if p.Water != d.content.WateringCanSize || p.Energy != 1 {
		t.Fatalf("after refill: water %d energy %d", p.Water, p.Energy)
	}
	// Walls and water cannot be tilled.
	p.Energy = 100
	if r := d.Use("ana", "houe", t0.Add(time.Hour)); r.OK {
		t.Fatal("tilled the river")
	}
}

func TestEmptyCan(t *testing.T) {
	d, p := farmer(t, "@..")
	mustUse(t, d, "houe", t0)
	p.Water = 0
	if r := d.Use("ana", "arrosoir", t0.Add(time.Second)); r.Reason != ReasonEmptyCan {
		t.Fatalf("watering with an empty can: %+v", r)
	}
}

func TestDryCropWithersThenGoesWildOrMutates(t *testing.T) {
	d, _ := farmer(t, "@..")
	field := Point{1, 0}
	mustUse(t, d, "houe", t0)
	mustUse(t, d, "graine-ble", t0)
	d.plots[field].Watered = false

	rules := d.content.Plots
	for i := 0; i < rules.DryDaysToWither; i++ {
		d.growPlots()
	}
	if d.plots[field].State != protocol.PlotWithered {
		t.Fatalf("after %d dry days: %+v", rules.DryDaysToWither, d.plots[field])
	}
	for i := 0; i < rules.WitheredDaysToSeed; i++ {
		d.growPlots()
	}
	pl := d.plots[field]
	if pl.State != protocol.PlotWild && !(pl.State == protocol.PlotCrop && pl.Crop == "ble-ambre") {
		t.Fatalf("a withered crop should go wild or mutate: %+v", pl)
	}
}

func TestMutationIsAWildCrop(t *testing.T) {
	d, _ := farmer(t, "@..")
	d.content.Crops["navet"] = func(c CropDef) CropDef { c.MutationChance = 1; return c }(d.content.Crops["navet"])
	field := Point{1, 0}
	mustUse(t, d, "houe", t0)
	mustUse(t, d, "graine-navet", t0)
	d.plots[field].Growth = d.content.Crops["navet"].Days // ripe, and nobody comes
	for i := 0; i < d.content.Plots.RipeDaysToSeed; i++ {
		d.growPlots()
	}
	pl := d.plots[field]
	if pl.State != protocol.PlotCrop || pl.Crop != "navet-lune" || pl.Growth != 0 {
		t.Fatalf("ripe crop left alone should reseed as its wild variant: %+v", pl)
	}
	// Wild crops grow without water.
	for i := 0; i < d.content.Crops["navet-lune"].Days; i++ {
		d.growPlots()
	}
	if !d.ripe(pl) {
		t.Fatalf("wild crop should ripen unwatered: %+v", pl)
	}
}

func TestIdleSoilGoesWildAndTheScytheClearsIt(t *testing.T) {
	d, p := farmer(t, "@..")
	field := Point{1, 0}
	mustUse(t, d, "houe", t0)
	for i := 0; i < d.content.Plots.IdleDaysToWild; i++ {
		d.growPlots()
	}
	if d.plots[field].State != protocol.PlotWild {
		t.Fatalf("idle soil: %+v", d.plots[field])
	}
	if r := d.Use("ana", "graine-navet", t0.Add(time.Second)); r.OK {
		t.Fatal("sowed into weeds")
	}
	fibre := p.Bag["fibre"]
	mustUse(t, d, "faux", t0)
	if d.plots[field] != nil || p.Bag["fibre"] != fibre+1 {
		t.Fatalf("after scything: plot %+v bag %v", d.plots[field], p.Bag)
	}
}

func TestRainWatersEverything(t *testing.T) {
	d, _ := farmer(t, "@..")
	mustUse(t, d, "houe", t0)
	d.content.Time.RainChance = 1
	d.sleepThrough(t0)
	if d.weather != protocol.WeatherRain || !d.plots[Point{1, 0}].Watered {
		t.Fatalf("rain: weather %q plot %+v", d.weather, d.plots[Point{1, 0}])
	}
}

func TestSleepingTogether(t *testing.T) {
	m := testMap(t,
		"#B#",
		"...",
		"@..",
	)
	d := joined(t, m, "ana", "bob")
	ana, bob := d.Player("ana"), d.Player("bob")
	ana.Pos, ana.Facing = Point{1, 1}, protocol.Up
	ana.Energy, bob.Energy = 3, 3
	day := d.Day()

	r := d.Use("ana", "", t0)
	if !r.OK || !ana.Sleeping || count[protocol.Notice](r.Events) != 1 || d.Day() != day {
		t.Fatalf("first in bed waits for the others: %+v", r)
	}
	// Bob leaves the valley: everyone left is asleep, so the night passes.
	ev := d.Leave("bob", t0.Add(time.Second))
	if d.Day() != day+1 || ana.Sleeping || ana.Energy != d.content.Energy.Max {
		t.Fatalf("after the night: day %d ana %+v events %+v", d.Day(), ana, ev)
	}
	if d.MinuteOfDay() != d.content.Time.WakeHour*60 {
		t.Fatalf("woke at minute %d", d.MinuteOfDay())
	}
	if bob.Energy != d.content.Energy.Max {
		t.Fatal("an offline player rests too")
	}
}

func TestStayingUpLateFaints(t *testing.T) {
	m := testMap(t,
		"#B#",
		"...",
		"..@",
	)
	d := joined(t, m, "ana")
	ana := d.Player("ana")
	day := d.Day()
	// From 6:00 to 2:00 is 20 game hours.
	ev := d.Tick(onlineAfter(d, 20*60+1))
	if d.Day() != day+1 || ana.Energy != d.content.Energy.AfterFaint || ana.Pos != m.WakeSpot() {
		t.Fatalf("fainting: day %d ana %+v", d.Day(), ana)
	}
	if count[protocol.Notice](ev) != 1 || count[protocol.Plots](ev) != 1 {
		t.Fatalf("events: %+v", ev)
	}
	// The real time left over after 2:00 keeps running from 6:00.
	if got := d.MinuteOfDay(); got < d.content.Time.WakeHour*60 || got > d.content.Time.WakeHour*60+2 {
		t.Fatalf("minute after fainting = %d", got)
	}
}

func TestTimePassesOfflineAtItsOwnPace(t *testing.T) {
	d, _ := farmer(t, "@..")
	mustUse(t, d, "houe", t0)
	mustUse(t, d, "graine-navet", t0)
	mustUse(t, d, "arrosoir", t0)
	d.Leave("ana", t0)
	day := d.Day()
	offlineDay := time.Duration(d.content.Time.OfflineDaySeconds * float64(time.Second))

	d.Tick(t0.Add(offlineDay * 2))
	if d.Day() != day+2 {
		t.Fatalf("two offline days: day %d → %d", day, d.Day())
	}
	if d.plots[Point{1, 0}].Growth != 1 {
		// Watered on the first day only.
		t.Fatalf("growth = %d", d.plots[Point{1, 0}].Growth)
	}
	// Nobody plays at night: nobody faints, the clock just runs.
	if d.Player("ana").Pos != d.Map.Spawn {
		t.Fatal("an offline player was moved")
	}
}

func TestLongAbsenceIsCapped(t *testing.T) {
	d := joined(t, testMap(t, "@.."))
	offlineDay := time.Duration(d.content.Time.OfflineDaySeconds * float64(time.Second))
	d.Tick(t0)
	d.Tick(t0.Add(offlineDay * 1000))
	if d.Mornings() != d.content.Time.MaxCatchUpDays {
		t.Fatalf("simulated %d mornings", d.Mornings())
	}
}

func TestClockView(t *testing.T) {
	d := joined(t, testMap(t, "@.."))
	v := d.ClockView()
	if v.Day != 1 || v.Minute != 6*60 || v.Season != d.content.Time.Seasons[0] || v.SeasonDay != 1 || v.Year != 1 {
		t.Fatalf("first morning: %+v", v)
	}
	d.minutes += float64(d.content.Time.DaysPerSeason * minutesPerDay)
	if v := d.ClockView(); v.Season != d.content.Time.Seasons[1] || v.SeasonDay != 1 {
		t.Fatalf("a season later: %+v", v)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	d, p := farmer(t, "@.T", "...")
	mustUse(t, d, "houe", t0)
	mustUse(t, d, "graine-feve", t0)
	mustUse(t, d, "arrosoir", t0)
	p.Facing = protocol.Down
	mustUse(t, d, "houe", t0)
	p.Pos = Point{1, 0}
	p.Facing = protocol.Right
	mustUse(t, d, "hache", t0) // a tree with one hit taken

	snap, _ := d.Snapshot(t0.Add(time.Minute))
	d2 := NewDomain(d.Map, d.content, 2)
	d2.Restore(snap)

	if len(d2.plots) != 2 || d2.plots[Point{1, 0}].Crop != "feve" || !d2.plots[Point{1, 0}].Watered {
		t.Fatalf("plots: %+v", d2.plots)
	}
	if d2.minutes != d.minutes || d2.weather != d.weather {
		t.Fatal("clock not restored")
	}
	tree := d2.resAt[Point{2, 0}]
	if tree.Hits != ResourceKinds["arbre"].Hits-1 {
		t.Fatalf("tree: %+v", tree)
	}
	q := d2.Player("ana")
	if q == nil || q.Online || q.Energy != p.Energy || q.Bag["graine-feve"] != p.Bag["graine-feve"] || len(q.Tools) != len(p.Tools) {
		t.Fatalf("player: %+v", q)
	}
	// The restored valley catches up at the offline pace.
	offlineDay := time.Duration(d.content.Time.OfflineDaySeconds * float64(time.Second))
	d2.Tick(snap.SavedAt.Add(offlineDay))
	if d2.Day() != d.Day()+1 {
		t.Fatalf("catch-up: day %d, was %d", d2.Day(), d.Day())
	}
}
