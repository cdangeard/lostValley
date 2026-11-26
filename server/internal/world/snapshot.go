package world

import (
	"sort"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// SnapshotVersion is bumped when the saved shape changes incompatibly.
const SnapshotVersion = 1 // phase 3 only added fields

// Snapshot is everything worth saving about a domain. It is plain data,
// stored as JSON (JSONB in PostgreSQL).
type Snapshot struct {
	SavedAt time.Time     `json:"savedAt"`
	Domain  DomainState   `json:"domain"`
	Players []PlayerState `json:"players"`
}

// DomainState is the valley itself: its clock, plots and resource nodes.
type DomainState struct {
	Version   int             `json:"version"`
	Minutes   float64         `json:"minutes"`
	Weather   string          `json:"weather"`
	Plots     []PlotState     `json:"plots"`
	Resources []ResourceState `json:"resources"`
	Buildings []BuildingState `json:"buildings,omitempty"`
	Claim     string          `json:"claim,omitempty"` // '1' per tile of the domain, row by row
	Chest     map[string]int  `json:"chest,omitempty"`
	WaveDay   int             `json:"waveDay,omitempty"` // last day the night wave came
	Pets      []PetState      `json:"pets,omitempty"`    // tamed creatures; wild ones are not saved
	Villagers []VillagerState `json:"villagers,omitempty"`
}

// PetState is a saved tamed creature.
type PetState struct {
	Kind    string `json:"kind"`
	Owner   string `json:"owner"`
	Mode    string `json:"mode"`
	Home    int    `json:"home,omitempty"`
	HP      int    `json:"hp"`
	Fed     bool   `json:"fed,omitempty"`
	Resting bool   `json:"resting,omitempty"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

// BuildingState is a saved building.
type BuildingState struct {
	ID    int    `json:"id"`
	Kind  string `json:"kind"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	HP    int    `json:"hp,omitempty"`
	Owner string `json:"owner,omitempty"`
	Load  int    `json:"load,omitempty"`
	Ready int    `json:"ready,omitempty"`
}

// PlotState is a saved plot.
type PlotState struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	State   string `json:"state"`
	Crop    string `json:"crop,omitempty"`
	Growth  int    `json:"growth,omitempty"`
	Watered bool   `json:"watered,omitempty"`
	Neglect int    `json:"neglect,omitempty"`
	Sower   string `json:"sower,omitempty"`
	DryDays int    `json:"dryDays,omitempty"`
}

// ResourceState is a saved resource node; untouched nodes are not saved.
type ResourceState struct {
	ID       int       `json:"id"`
	Kind     string    `json:"kind"`
	X        int       `json:"x"`
	Y        int       `json:"y"`
	Hits     int       `json:"hits"`
	Depleted bool      `json:"depleted,omitempty"`
	Removed  bool      `json:"removed,omitempty"`
	RegrowAt time.Time `json:"regrowAt,omitempty"`
}

// PlayerState is a saved character. Players will later travel between
// domains, so they are stored apart from the valley.
type PlayerState struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	X       int            `json:"x"`
	Y       int            `json:"y"`
	Facing  string         `json:"facing"`
	Tools   []string       `json:"tools"`
	Bag     map[string]int `json:"bag"`
	Energy  int            `json:"energy"`
	HP      int            `json:"hp,omitempty"` // 0 in old saves: full health
	Water   int            `json:"water"`
	Level   int            `json:"level,omitempty"` // 0 in old saves: level 1
	XP      int            `json:"xp,omitempty"`
	Talents []string       `json:"talents,omitempty"`
}

// Snapshot captures the domain at now. The clock is advanced to now first so
// the saved time matches the saved minute.
func (d *Domain) Snapshot(now time.Time) (*Snapshot, []Event) {
	events := d.advanceClock(now)
	s := &Snapshot{SavedAt: d.lastTick, Domain: DomainState{
		Version: SnapshotVersion, Minutes: d.minutes, Weather: d.weather,
		Plots: []PlotState{}, Resources: []ResourceState{},
		Claim: d.claimString(), Chest: map[string]int{}, WaveDay: d.lastWaveDay, Pets: d.petStates(),
		Villagers: d.villagerStates(),
	}}
	for k, v := range d.chest {
		if v > 0 {
			s.Domain.Chest[k] = v
		}
	}
	for _, b := range d.sortedBuildings() {
		s.Domain.Buildings = append(s.Domain.Buildings, BuildingState{ID: b.ID, Kind: b.Kind, X: b.At.X, Y: b.At.Y,
			HP: b.HP, Owner: b.Owner, Load: b.Load, Ready: b.Ready})
	}
	for _, p := range d.sortedPlots() {
		s.Domain.Plots = append(s.Domain.Plots, PlotState{X: p.At.X, Y: p.At.Y, State: p.State, Crop: p.Crop,
			Growth: p.Growth, Watered: p.Watered, Neglect: p.Neglect, Sower: p.Sower, DryDays: p.DryDays})
	}
	for _, r := range d.resources {
		if r.Depleted || r.Removed || r.Hits != r.def().Hits {
			s.Domain.Resources = append(s.Domain.Resources, ResourceState{ID: r.ID, Kind: r.Kind, X: r.At.X, Y: r.At.Y,
				Hits: r.Hits, Depleted: r.Depleted, Removed: r.Removed, RegrowAt: r.RegrowAt})
		}
	}
	for _, p := range d.sortedPlayers() {
		ps := PlayerState{ID: p.ID, Name: p.Name, X: p.Pos.X, Y: p.Pos.Y, Facing: p.Facing,
			Tools: append([]string{}, p.Tools...), Bag: p.bagCopy(), Energy: p.Energy, HP: p.HP, Water: p.Water,
			Level: p.Level, XP: p.XP}
		for id := range p.Talents {
			ps.Talents = append(ps.Talents, id)
		}
		sort.Strings(ps.Talents)
		s.Players = append(s.Players, ps)
	}
	return s, events
}

// Restore loads a snapshot into a fresh domain (no one online). Saved things
// that no longer fit the map or the content are dropped. The time between
// the save and the next Tick is then caught up at the offline pace.
func (d *Domain) Restore(s *Snapshot) {
	ds := s.Domain
	d.minutes = ds.Minutes
	if ds.Weather != "" {
		d.weather = ds.Weather
	}
	d.lastTick = s.SavedAt
	d.clockStep = d.halfHour()

	d.plots = map[Point]*Plot{}
	for _, ps := range ds.Plots {
		at := Point{ps.X, ps.Y}
		if !d.Map.Cultivable(at) || d.resAt[at] != nil || !validPlot(ps, d.content) {
			continue
		}
		d.plots[at] = &Plot{At: at, State: ps.State, Crop: ps.Crop, Growth: ps.Growth, Watered: ps.Watered, Neglect: ps.Neglect,
			Sower: ps.Sower, DryDays: ps.DryDays}
	}
	for _, rs := range ds.Resources {
		r := d.resAt[Point{rs.X, rs.Y}]
		if r == nil || r.ID != rs.ID || r.Kind != rs.Kind {
			continue // the map changed
		}
		r.Hits, r.Depleted, r.RegrowAt = rs.Hits, rs.Depleted, rs.RegrowAt
		if rs.Removed {
			r.Removed = true
			delete(d.resAt, r.At)
		}
	}
	d.restoreClaim(ds.Claim)
	d.lastWaveDay = ds.WaveDay
	for k, v := range ds.Chest {
		if v > 0 {
			d.chest[k] = v
		}
	}
	for _, bs := range ds.Buildings {
		def, ok := d.content.Buildings[bs.Kind]
		if !ok {
			continue
		}
		b := &Building{ID: bs.ID, Kind: bs.Kind, At: Point{bs.X, bs.Y}, W: def.W, H: def.H, HP: bs.HP,
			Owner: bs.Owner, Load: bs.Load, Ready: bs.Ready}
		if def.HP > 0 && (b.HP <= 0 || b.HP > def.HP) {
			b.HP = def.HP
		}
		fits := bs.ID > 0
		for _, t := range b.tiles() {
			if !d.Map.InBounds(t) || d.Map.Blocked(t) || d.bAt[t] != nil || d.resAt[t] != nil || d.plots[t] != nil {
				fits = false
			}
		}
		if fits {
			d.addBuilding(b)
		}
	}
	// Creatures are not saved; drop any spawned where something now stands.
	for _, c := range d.sortedCreatures() {
		if d.bAt[c.Pos] != nil || d.Claimed(c.Pos) {
			delete(d.creatures, c.ID)
		}
	}
	for _, ps := range s.Players {
		p := d.newPlayer(ps.ID)
		p.Name, p.Pos, p.Facing = ps.Name, Point{ps.X, ps.Y}, ps.Facing
		if !protocol.ValidDir(p.Facing) {
			p.Facing = protocol.Down
		}
		p.Tools = append([]string{}, ps.Tools...)
		p.Bag = map[string]int{}
		for k, v := range ps.Bag {
			p.Bag[k] = v
		}
		p.Energy, p.Water = ps.Energy, ps.Water
		p.Level, p.XP = max(1, ps.Level), ps.XP
		if tree := d.content.Constellation; tree != nil {
			for _, id := range ps.Talents {
				if tree.Node(id) != nil && id != tree.Heart {
					p.Talents[id] = true
				}
			}
			// A changed constellation may leave nodes cut off: keep what still links to the heart.
			reach := tree.connected(p.Talents, "")
			for id := range p.Talents {
				if !reach[id] {
					delete(p.Talents, id)
				}
			}
		}
		for p.Points() < 0 {
			for id := range p.Talents { // more nodes than levels (the rules changed): drop some
				delete(p.Talents, id)
				break
			}
		}
		d.recomputeMods(p)
		if ps.HP > 0 {
			p.HP = ps.HP
		}
		d.clampStats(p)
		d.players[p.ID] = p
	}
	d.restorePets(ds.Pets)
	d.restoreVillagers(ds.Villagers)
}

func validPlot(ps PlotState, c *Content) bool {
	switch ps.State {
	case protocol.PlotTilled, protocol.PlotWild:
		return ps.Crop == ""
	case protocol.PlotCrop, protocol.PlotWithered:
		_, ok := c.Crops[ps.Crop]
		return ok
	}
	return false
}
