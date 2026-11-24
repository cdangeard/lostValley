package world

import (
	"encoding/json"
	"fmt"
	"os"
)

// Layer and object names used in the Tiled maps. The client reads the same
// names, so keep both sides in sync (see client/src/scenes/ValleyScene.ts).
const (
	LayerGround    = "sol"
	LayerResources = "ressources"
	LayerMarkers   = "reperes"
	MarkerSpawn    = "apparition"
	MarkerBed      = "lit"
	MarkerChest    = "coffre"  // the house chest: what is in it is never lost
	MarkerDomain   = "domaine" // a rectangle: the domain at the start, before any clearing
	PropBlocking   = "bloquant"
	PropCultivable = "cultivable" // the hoe may till this ground
	PropWater      = "eau"        // the watering can refills here
	PropEdge       = "lisiere"    // forest edge: creatures live and the night wave comes from here
)

// Map is the static part of a domain, loaded from a Tiled JSON export.
type Map struct {
	Width, Height int
	TileSize      int
	blocked       []bool
	cultivable    []bool
	water         []bool
	edge          []bool
	Resources     []ResourceSpawn
	Spawn         Point
	// Bed is where players sleep; HasBed is false on maps without one.
	Bed    Point
	HasBed bool
	// Chest is the house chest; HasChest is false on maps without one.
	Chest    Point
	HasChest bool
	// Domain is the starting domain; DomainW is 0 when the map has none.
	Domain           Point
	DomainW, DomainH int
}

// ResourceSpawn is where a resource node starts on the map.
type ResourceSpawn struct {
	Kind string
	At   Point
}

// InBounds reports whether p is on the map.
func (m *Map) InBounds(p Point) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < m.Width && p.Y < m.Height
}

// Blocked reports whether the ground itself stops movement (water, cliffs,
// walls, the bed…).
func (m *Map) Blocked(p Point) bool {
	return !m.InBounds(p) || m.blocked[p.Y*m.Width+p.X] || (m.HasBed && p == m.Bed) || (m.HasChest && p == m.Chest)
}

// Cultivable reports whether the hoe may till the ground at p.
func (m *Map) Cultivable(p Point) bool {
	return m.InBounds(p) && m.cultivable[p.Y*m.Width+p.X] && !m.Blocked(p)
}

// Water reports whether p is water the watering can can draw from.
func (m *Map) Water(p Point) bool {
	return m.InBounds(p) && m.water[p.Y*m.Width+p.X]
}

// Edge reports whether p is forest edge, where creatures come from.
func (m *Map) Edge(p Point) bool {
	return m.InBounds(p) && m.edge[p.Y*m.Width+p.X]
}

// WakeSpot is where a player wakes up: in front of the bed, or at the spawn.
func (m *Map) WakeSpot() Point {
	if m.HasBed {
		return Point{m.Bed.X, m.Bed.Y + 1}
	}
	return m.Spawn
}

type tiledProperty struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type tiledFile struct {
	Width      int `json:"width"`
	Height     int `json:"height"`
	TileWidth  int `json:"tilewidth"`
	TileHeight int `json:"tileheight"`
	Layers     []struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Data    []int  `json:"data"`
		Objects []struct {
			Name   string  `json:"name"`
			Type   string  `json:"type"`
			Class  string  `json:"class"` // Tiled >= 1.9 writes "class" instead of "type"
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"objects"`
	} `json:"layers"`
	Tilesets []struct {
		FirstGID int    `json:"firstgid"`
		Source   string `json:"source"`
		Tiles    []struct {
			ID         int             `json:"id"`
			Properties []tiledProperty `json:"properties"`
		} `json:"tiles"`
	} `json:"tilesets"`
}

// LoadMap reads a Tiled JSON map from disk.
func LoadMap(path string) (*Map, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseMap(raw)
}

// ParseMap parses a Tiled JSON map (orthogonal, finite, embedded tilesets).
func ParseMap(raw []byte) (*Map, error) {
	var f tiledFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse tiled map: %w", err)
	}
	if f.Width <= 0 || f.Height <= 0 {
		return nil, fmt.Errorf("parse tiled map: invalid size %dx%d", f.Width, f.Height)
	}
	if f.TileWidth != f.TileHeight {
		return nil, fmt.Errorf("parse tiled map: tiles must be square, got %dx%d", f.TileWidth, f.TileHeight)
	}

	blockingGID, cultivableGID, waterGID, edgeGID := map[int]bool{}, map[int]bool{}, map[int]bool{}, map[int]bool{}
	for _, ts := range f.Tilesets {
		if ts.Source != "" {
			return nil, fmt.Errorf("parse tiled map: external tileset %q not supported, embed it in the map", ts.Source)
		}
		for _, t := range ts.Tiles {
			for _, p := range t.Properties {
				if p.Value != true {
					continue
				}
				switch p.Name {
				case PropBlocking:
					blockingGID[ts.FirstGID+t.ID] = true
				case PropCultivable:
					cultivableGID[ts.FirstGID+t.ID] = true
				case PropWater:
					waterGID[ts.FirstGID+t.ID] = true
				case PropEdge:
					edgeGID[ts.FirstGID+t.ID] = true
				}
			}
		}
	}

	n := f.Width * f.Height
	m := &Map{Width: f.Width, Height: f.Height, TileSize: f.TileWidth,
		blocked: make([]bool, n), cultivable: make([]bool, n), water: make([]bool, n), edge: make([]bool, n)}
	ts := float64(f.TileWidth)
	foundGround, foundSpawn := false, false
	for _, l := range f.Layers {
		switch {
		case l.Name == LayerGround && l.Type == "tilelayer":
			if len(l.Data) != f.Width*f.Height {
				return nil, fmt.Errorf("parse tiled map: layer %q has %d tiles, want %d", l.Name, len(l.Data), f.Width*f.Height)
			}
			for i, gid := range l.Data {
				// The top 4 bits of a gid are flip flags.
				gid &= 0x0fffffff
				m.blocked[i] = blockingGID[gid]
				m.cultivable[i] = cultivableGID[gid]
				m.water[i] = waterGID[gid]
				m.edge[i] = edgeGID[gid]
			}
			foundGround = true
		case l.Name == LayerResources && l.Type == "objectgroup":
			for _, o := range l.Objects {
				kind := o.Type
				if kind == "" {
					kind = o.Class
				}
				p := Point{int(o.X / ts), int(o.Y / ts)}
				if _, ok := ResourceKinds[kind]; !ok {
					return nil, fmt.Errorf("parse tiled map: unknown resource kind %q at %v", kind, p)
				}
				if !m.InBounds(p) {
					return nil, fmt.Errorf("parse tiled map: resource %q out of bounds at %v", kind, p)
				}
				m.Resources = append(m.Resources, ResourceSpawn{Kind: kind, At: p})
			}
		case l.Name == LayerMarkers && l.Type == "objectgroup":
			for _, o := range l.Objects {
				switch o.Name {
				case MarkerSpawn:
					m.Spawn = Point{int(o.X / ts), int(o.Y / ts)}
					foundSpawn = true
				case MarkerBed:
					m.Bed = Point{int(o.X / ts), int(o.Y / ts)}
					m.HasBed = true
				case MarkerChest:
					m.Chest = Point{int(o.X / ts), int(o.Y / ts)}
					m.HasChest = true
				case MarkerDomain:
					m.Domain = Point{int(o.X / ts), int(o.Y / ts)}
					m.DomainW, m.DomainH = int(o.Width/ts), int(o.Height/ts)
				}
			}
		}
	}
	if !foundGround {
		return nil, fmt.Errorf("parse tiled map: missing tile layer %q", LayerGround)
	}
	if !foundSpawn {
		return nil, fmt.Errorf("parse tiled map: missing %q object in layer %q", MarkerSpawn, LayerMarkers)
	}
	if m.Blocked(m.Spawn) {
		return nil, fmt.Errorf("parse tiled map: spawn %v is on a blocking tile", m.Spawn)
	}
	if m.HasBed && (!m.InBounds(m.Bed) || m.Blocked(m.WakeSpot())) {
		return nil, fmt.Errorf("parse tiled map: the tile below the bed %v must be free", m.Bed)
	}
	if m.HasChest && !m.InBounds(m.Chest) {
		return nil, fmt.Errorf("parse tiled map: chest %v out of bounds", m.Chest)
	}
	return m, nil
}
