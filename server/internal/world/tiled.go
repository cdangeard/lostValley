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
	PropBlocking   = "bloquant"
)

// Map is the static part of a domain, loaded from a Tiled JSON export.
type Map struct {
	Width, Height int
	TileSize      int
	blocked       []bool
	Resources     []ResourceSpawn
	Spawn         Point
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

// Blocked reports whether the ground itself stops movement (water, cliffs…).
func (m *Map) Blocked(p Point) bool {
	return !m.InBounds(p) || m.blocked[p.Y*m.Width+p.X]
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
			Name  string  `json:"name"`
			Type  string  `json:"type"`
			Class string  `json:"class"` // Tiled >= 1.9 writes "class" instead of "type"
			X     float64 `json:"x"`
			Y     float64 `json:"y"`
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

	blockingGID := map[int]bool{}
	for _, ts := range f.Tilesets {
		if ts.Source != "" {
			return nil, fmt.Errorf("parse tiled map: external tileset %q not supported, embed it in the map", ts.Source)
		}
		for _, t := range ts.Tiles {
			for _, p := range t.Properties {
				if p.Name == PropBlocking && p.Value == true {
					blockingGID[ts.FirstGID+t.ID] = true
				}
			}
		}
	}

	m := &Map{Width: f.Width, Height: f.Height, TileSize: f.TileWidth, blocked: make([]bool, f.Width*f.Height)}
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
				m.blocked[i] = blockingGID[gid&0x0fffffff]
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
				if o.Name == MarkerSpawn {
					m.Spawn = Point{int(o.X / ts), int(o.Y / ts)}
					foundSpawn = true
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
	return m, nil
}
