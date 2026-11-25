package world

import (
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// Phase 2 exit criterion: "a defended night is tense but winnable". On the
// real valley, a field of crops is attacked by the night wave. A player who
// fights with the right tools keeps most of the field and survives; a player
// busy elsewhere loses it.

var weaponFor = map[string]string{
	"rongeur-racine": "houe", "corbeau-voleur": "faux", "ronceau": "hache", "scarabee-galet": "pioche",
}

type nightResult struct {
	planted, left, minHP, kills int
	fell                        bool
}

func playNight(t *testing.T, day int, seed int64, defend bool) nightResult {
	t.Helper()
	m, err := LoadMap("../../../assets/maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	c := testContent(t)
	d := NewDomain(m, c, seed)
	now := t0
	d.Join("ana", "ana", now)
	ana := d.Player("ana")

	// A 4 × 3 field south of the house, inside the domain.
	planted := 0
	for y := 18; y <= 19; y++ {
		for x := 3; x <= 8; x++ {
			at := Point{x, y}
			if d.canTill(at) {
				d.plots[at] = &Plot{At: at, State: protocol.PlotCrop, Crop: "navet", Growth: 2, Watered: true}
				planted++
			}
		}
	}
	// Evening of the given day, the player by the field.
	d.minutes = float64((day-1)*minutesPerDay + 19*60 + 59)
	d.clockStep = d.halfHour()
	ana.Pos, ana.Facing = Point{6, 17}, protocol.Down
	if !defend {
		// Busy far away, across the river: the pests have the field to themselves.
		for x := m.Width - 2; x > 0 && ana.Pos == (Point{6, 17}); x-- {
			if at := (Point{x, 20}); d.Walkable(at) {
				ana.Pos = at
			}
		}
	}
	res := nightResult{planted: planted, minHP: ana.HP}
	now = now.Add(2 * time.Second)
	d.Tick(now)
	if d.lastWaveDay != d.Day() {
		t.Fatal("no wave came")
	}

	steps := []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right}
	for i := 0; i < 3000; i++ {
		now = now.Add(300 * time.Millisecond)
		for _, e := range d.Tick(now) {
			if g, ok := e.Msg.(protocol.CreatureGone); ok && g.How == protocol.GoneKilled {
				res.kills++
			}
		}
		wave := 0
		for _, cr := range d.creatures {
			if cr.Wave {
				wave++
			}
		}
		if wave == 0 || ana.Sleeping {
			break
		}
		if !defend {
			continue
		}
		if d.engaged(ana) && ana.turnAt > d.ct {
			continue
		}
		warned := map[Point]bool{}
		for _, cr := range d.creatures {
			for _, w := range cr.Warn {
				warned[w] = true
			}
		}
		act := func() Result {
			if ana.HP <= 4 && ana.Bag["ration"] > 0 {
				return d.Use("ana", "ration", now)
			}
			for _, dir := range steps {
				cr := d.creatureAt(ana.Pos.Step(dir))
				if cr == nil || (cr.Burrowed && cr.Kind != "rongeur-racine") {
					continue
				}
				if ana.Facing != dir {
					d.Move("ana", dir, now) // blocked by the creature: turns for free
				}
				return d.Use("ana", weaponFor[cr.Kind], now)
			}
			if warned[ana.Pos] {
				for _, dir := range steps {
					if t := ana.Pos.Step(dir); d.Walkable(t) && !warned[t] {
						return d.Move("ana", dir, now)
					}
				}
			}
			// Go for the wave creature closest to the field.
			var target *Creature
			for _, cr := range d.sortedCreatures() {
				if cr.Wave && (target == nil || cheb(cr.Pos, ana.Pos) < cheb(target.Pos, ana.Pos)) {
					target = cr
				}
			}
			if step, ok := d.playerStep(ana.Pos, adjacentGoals(target.Pos)); ok && !warned[step] {
				return d.Move("ana", dirTo(ana.Pos, step), now)
			}
			return d.Wait("ana", now)
		}
		r := act()
		for _, e := range r.Events {
			if g, ok := e.Msg.(protocol.CreatureGone); ok && g.How == protocol.GoneKilled {
				res.kills++
			}
		}
		res.minHP = min(res.minHP, ana.HP)
		if ana.Sleeping {
			res.fell = true
			break
		}
	}
	for _, p := range d.plots {
		if p.State == protocol.PlotCrop {
			res.left++
		}
	}
	return res
}

// playerStep is the first step of a shortest walk to any goal.
func (d *Domain) playerStep(from Point, goals map[Point]bool) (Point, bool) {
	type node struct{ at, first Point }
	seen := map[Point]bool{from: true}
	var queue []node
	for _, dir := range []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right} {
		if t := from.Step(dir); d.Walkable(t) {
			seen[t] = true
			queue = append(queue, node{t, t})
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if goals[n.at] {
			return n.first, true
		}
		for _, dir := range []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right} {
			if t := n.at.Step(dir); !seen[t] && d.Walkable(t) {
				seen[t] = true
				queue = append(queue, node{t, n.first})
			}
		}
	}
	return Point{}, false
}

func TestNightIsTenseButWinnable(t *testing.T) {
	if testing.Short() {
		t.Skip("plays whole nights")
	}
	// Day 1 brings only small pests; by day 8 brambles and beetles come too.
	for _, tc := range []struct{ day, minWon int }{{1, 6}, {8, 4}} {
		won, idleLost := 0, 0
		const runs = 8
		for seed := int64(1); seed <= runs; seed++ {
			r := playNight(t, tc.day, seed, true)
			if !r.fell && r.left*4 >= r.planted*3 {
				won++
			}
			if idle := playNight(t, tc.day, seed, false); idle.left*2 < idle.planted {
				idleLost++
			}
		}
		t.Logf("day %d: defended and won %d/%d nights; left alone, the field was mostly eaten %d/%d times", tc.day, won, runs, idleLost, runs)
		if won < tc.minWon {
			t.Errorf("day %d: a careful defender won only %d/%d nights: too hard", tc.day, won, runs)
		}
		if idleLost < runs*3/4 {
			t.Errorf("day %d: an undefended field survived %d/%d nights: no tension", tc.day, runs-idleLost, runs)
		}
	}
}
