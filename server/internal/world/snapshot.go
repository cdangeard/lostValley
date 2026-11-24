package world

import (
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// SnapshotVersion is bumped when the saved shape changes incompatibly.
const SnapshotVersion = 1

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
}

// ResourceState is a saved resource node; untouched nodes are not saved.
type ResourceState struct {
	ID       int       `json:"id"`
	Kind     string    `json:"kind"`
	X        int       `json:"x"`
	Y        int       `json:"y"`
	Hits     int       `json:"hits"`
	Depleted bool      `json:"depleted,omitempty"`
	RegrowAt time.Time `json:"regrowAt,omitempty"`
}

// PlayerState is a saved character. Players will later travel between
// domains, so they are stored apart from the valley.
type PlayerState struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	X      int            `json:"x"`
	Y      int            `json:"y"`
	Facing string         `json:"facing"`
	Tools  []string       `json:"tools"`
	Bag    map[string]int `json:"bag"`
	Energy int            `json:"energy"`
	Water  int            `json:"water"`
}

// Snapshot captures the domain at now. The clock is advanced to now first so
// the saved time matches the saved minute.
func (d *Domain) Snapshot(now time.Time) (*Snapshot, []Event) {
	events := d.advanceClock(now)
	s := &Snapshot{SavedAt: d.lastTick, Domain: DomainState{
		Version: SnapshotVersion, Minutes: d.minutes, Weather: d.weather,
		Plots: []PlotState{}, Resources: []ResourceState{},
	}}
	for _, p := range d.sortedPlots() {
		s.Domain.Plots = append(s.Domain.Plots, PlotState{X: p.At.X, Y: p.At.Y, State: p.State, Crop: p.Crop,
			Growth: p.Growth, Watered: p.Watered, Neglect: p.Neglect})
	}
	for _, r := range d.resources {
		if r.Depleted || r.Hits != r.def().Hits {
			s.Domain.Resources = append(s.Domain.Resources, ResourceState{ID: r.ID, Kind: r.Kind, X: r.At.X, Y: r.At.Y,
				Hits: r.Hits, Depleted: r.Depleted, RegrowAt: r.RegrowAt})
		}
	}
	for _, p := range d.sortedPlayers() {
		s.Players = append(s.Players, PlayerState{ID: p.ID, Name: p.Name, X: p.Pos.X, Y: p.Pos.Y, Facing: p.Facing,
			Tools: append([]string{}, p.Tools...), Bag: p.bagCopy(), Energy: p.Energy, Water: p.Water})
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
		d.plots[at] = &Plot{At: at, State: ps.State, Crop: ps.Crop, Growth: ps.Growth, Watered: ps.Watered, Neglect: ps.Neglect}
	}
	for _, rs := range ds.Resources {
		r := d.resAt[Point{rs.X, rs.Y}]
		if r == nil || r.ID != rs.ID || r.Kind != rs.Kind {
			continue // the map changed
		}
		r.Hits, r.Depleted, r.RegrowAt = rs.Hits, rs.Depleted, rs.RegrowAt
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
		d.players[p.ID] = p
	}
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
