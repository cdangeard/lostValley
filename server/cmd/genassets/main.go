// Command genassets writes the placeholder art and the first valley map.
//
// Everything is drawn procedurally so the repository holds only original art.
// The map it writes is a regular Tiled JSON file: once it exists, edit it in
// Tiled rather than regenerating it (re-running overwrites the file).
//
//	go run ./cmd/genassets -out ../assets
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
)

const tile = 16

// Terrain tile indexes (0-based, gid = index + 1).
const (
	tGrass = iota
	tGrassFlowers
	tForestFloor
	tPath
	tWater
	tSand
	tTilled
	tBridge
	terrainCount
)

// Entity frames in entities.png.
const (
	fTree = iota
	fStump
	fRock
	fRubble
	fGrass
	fGrassCut
	fPlayerDown
	fPlayerLeft
	fPlayerRight
	fPlayerUp
	entityCount
)

func main() {
	out := flag.String("out", "../assets", "assets directory")
	seed := flag.Int64("seed", 7, "random seed")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))
	must(writePNG(filepath.Join(*out, "tiles", "terrain.png"), drawTerrain(rng)))
	must(writePNG(filepath.Join(*out, "sprites", "entities.png"), drawEntities(rng)))
	must(writeJSON(filepath.Join(*out, "maps", "valley.json"), buildMap(rng)))
	fmt.Println("assets written to", *out)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// ---------- drawing helpers ----------

func hex(s string) color.RGBA {
	var r, g, b uint8
	fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}
}

type canvas struct{ img *image.RGBA }

func (c canvas) px(ox, x, y int, col color.RGBA) {
	if x < 0 || y < 0 || x >= tile || y >= tile {
		return
	}
	c.img.SetRGBA(ox*tile+x, y, col)
}

func (c canvas) fill(ox int, col color.RGBA) {
	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			c.px(ox, x, y, col)
		}
	}
}

func (c canvas) speckle(ox int, rng *rand.Rand, n int, cols ...color.RGBA) {
	for i := 0; i < n; i++ {
		c.px(ox, rng.Intn(tile), rng.Intn(tile), cols[rng.Intn(len(cols))])
	}
}

func (c canvas) disc(ox int, cx, cy, r float64, col color.RGBA) {
	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				c.px(ox, x, y, col)
			}
		}
	}
}

func (c canvas) rect(ox, x0, y0, w, h int, col color.RGBA) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			c.px(ox, x, y, col)
		}
	}
}

// ---------- terrain ----------

func drawTerrain(rng *rand.Rand) image.Image {
	c := canvas{image.NewRGBA(image.Rect(0, 0, tile*terrainCount, tile))}
	grass, grassD, grassL := hex("#5f9e4a"), hex("#4f8a3e"), hex("#78b55a")

	c.fill(tGrass, grass)
	c.speckle(tGrass, rng, 28, grassD, grassL)

	c.fill(tGrassFlowers, grass)
	c.speckle(tGrassFlowers, rng, 20, grassD, grassL)
	for _, p := range [][2]int{{3, 4}, {11, 3}, {7, 10}, {13, 12}} {
		c.px(tGrassFlowers, p[0], p[1], hex("#f3e7a1"))
		c.px(tGrassFlowers, p[0]+1, p[1], hex("#e9a3c9"))
	}

	c.fill(tForestFloor, hex("#3f6b35"))
	c.speckle(tForestFloor, rng, 40, hex("#355c2d"), hex("#4b7a3c"), hex("#6b5a3a"))

	c.fill(tPath, hex("#b99764"))
	c.speckle(tPath, rng, 30, hex("#a5844f"), hex("#c9a877"))

	c.fill(tWater, hex("#3d7fb8"))
	for i := 0; i < 5; i++ {
		y, x := rng.Intn(tile), rng.Intn(tile-4)
		c.rect(tWater, x, y, 4, 1, hex("#6aa6d6"))
	}

	c.fill(tSand, hex("#d9c48e"))
	c.speckle(tSand, rng, 24, hex("#cbb37a"), hex("#e6d6a6"))

	c.fill(tTilled, hex("#7a5434"))
	for y := 1; y < tile; y += 4 {
		c.rect(tTilled, 0, y, tile, 1, hex("#5e3f25"))
	}

	c.fill(tBridge, hex("#8a6239"))
	for x := 0; x < tile; x += 4 {
		c.rect(tBridge, x, 0, 1, tile, hex("#6b4a2a"))
	}
	c.rect(tBridge, 0, 0, tile, 1, hex("#5a3d22"))
	c.rect(tBridge, 0, tile-1, tile, 1, hex("#5a3d22"))
	return c.img
}

// ---------- entities ----------

func drawEntities(rng *rand.Rand) image.Image {
	c := canvas{image.NewRGBA(image.Rect(0, 0, tile*entityCount, tile))}
	trunk, trunkD := hex("#6e4a2b"), hex("#553820")

	// Tree: round canopy on a short trunk.
	c.rect(fTree, 7, 10, 3, 6, trunk)
	c.rect(fTree, 7, 10, 1, 6, trunkD)
	c.disc(fTree, 8.5, 7, 6.5, hex("#2f6e3a"))
	c.disc(fTree, 7.5, 6, 4.5, hex("#3d8a46"))
	c.disc(fTree, 6.5, 4.5, 2, hex("#58a85c"))

	// Stump.
	c.rect(fStump, 5, 9, 7, 5, trunk)
	c.rect(fStump, 5, 9, 7, 2, hex("#c79a62"))
	c.px(fStump, 8, 10, trunkD)

	// Rock.
	c.disc(fRock, 8, 10, 6, hex("#7d8087"))
	c.disc(fRock, 7, 9, 4.5, hex("#979aa1"))
	c.disc(fRock, 6, 7.5, 1.5, hex("#b8bbc1"))

	// Rubble.
	c.disc(fRubble, 5, 12, 2, hex("#7d8087"))
	c.disc(fRubble, 10, 13, 2, hex("#979aa1"))
	c.disc(fRubble, 8, 10, 1.5, hex("#7d8087"))

	// Tall grass (fibre).
	for i := 0; i < 9; i++ {
		x := 2 + i*12/9 + rng.Intn(2)
		h := 6 + rng.Intn(6)
		col := []color.RGBA{hex("#8cc063"), hex("#6fa64b"), hex("#a3cf70")}[i%3]
		c.rect(fGrass, x, tile-h, 1, h, col)
	}
	// Cut grass.
	for i := 0; i < 7; i++ {
		c.rect(fGrassCut, 3+i*2, 13, 1, 3, hex("#6fa64b"))
	}

	drawPlayer(c, fPlayerDown, "down")
	drawPlayer(c, fPlayerLeft, "left")
	drawPlayer(c, fPlayerRight, "right")
	drawPlayer(c, fPlayerUp, "up")
	return c.img
}

func drawPlayer(c canvas, f int, dir string) {
	skin, hair := hex("#f1c9a0"), hex("#7a4b2a")
	tunic, tunicD := hex("#c96b3c"), hex("#a3532b")
	boots := hex("#4a3424")
	// Body.
	c.rect(f, 5, 8, 6, 5, tunic)
	c.rect(f, 5, 12, 6, 1, tunicD)
	c.rect(f, 5, 13, 2, 2, boots)
	c.rect(f, 9, 13, 2, 2, boots)
	// Head.
	c.rect(f, 5, 2, 6, 6, skin)
	c.rect(f, 5, 1, 6, 2, hair)
	switch dir {
	case "down":
		c.px(f, 6, 5, hex("#2b2b2b"))
		c.px(f, 9, 5, hex("#2b2b2b"))
	case "up":
		c.rect(f, 5, 2, 6, 5, hair)
	case "left":
		c.rect(f, 8, 2, 3, 5, hair)
		c.px(f, 6, 5, hex("#2b2b2b"))
	case "right":
		c.rect(f, 5, 2, 3, 5, hair)
		c.px(f, 9, 5, hex("#2b2b2b"))
	}
}

// ---------- map ----------

type tiledProp struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type tiledObject struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Rotation float64 `json:"rotation"`
	Visible  bool    `json:"visible"`
	Point    bool    `json:"point,omitempty"`
}

type tiledLayer struct {
	ID        int           `json:"id"`
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	Width     int           `json:"width,omitempty"`
	Height    int           `json:"height,omitempty"`
	Data      []int         `json:"data,omitempty"`
	Objects   []tiledObject `json:"objects,omitempty"`
	DrawOrder string        `json:"draworder,omitempty"`
	Opacity   float64       `json:"opacity"`
	Visible   bool          `json:"visible"`
	X         int           `json:"x"`
	Y         int           `json:"y"`
}

type tiledTile struct {
	ID         int         `json:"id"`
	Properties []tiledProp `json:"properties"`
}

type tiledTileset struct {
	FirstGID    int         `json:"firstgid"`
	Name        string      `json:"name"`
	Image       string      `json:"image"`
	ImageWidth  int         `json:"imagewidth"`
	ImageHeight int         `json:"imageheight"`
	TileWidth   int         `json:"tilewidth"`
	TileHeight  int         `json:"tileheight"`
	TileCount   int         `json:"tilecount"`
	Columns     int         `json:"columns"`
	Margin      int         `json:"margin"`
	Spacing     int         `json:"spacing"`
	Tiles       []tiledTile `json:"tiles"`
}

type tiledMap struct {
	Type         string         `json:"type"`
	Version      string         `json:"version"`
	TiledVersion string         `json:"tiledversion"`
	Orientation  string         `json:"orientation"`
	RenderOrder  string         `json:"renderorder"`
	Width        int            `json:"width"`
	Height       int            `json:"height"`
	TileWidth    int            `json:"tilewidth"`
	TileHeight   int            `json:"tileheight"`
	Infinite     bool           `json:"infinite"`
	NextLayerID  int            `json:"nextlayerid"`
	NextObjectID int            `json:"nextobjectid"`
	Layers       []tiledLayer   `json:"layers"`
	Tilesets     []tiledTileset `json:"tilesets"`
}

const (
	mapW = 48
	mapH = 32
)

func buildMap(rng *rand.Rand) tiledMap {
	ground := make([]int, mapW*mapH)
	set := func(x, y, t int) {
		if x >= 0 && y >= 0 && x < mapW && y < mapH {
			ground[y*mapW+x] = t + 1
		}
	}
	get := func(x, y int) int { return ground[y*mapW+x] - 1 }

	// Meadow everywhere, with a few flowers.
	for y := 0; y < mapH; y++ {
		for x := 0; x < mapW; x++ {
			t := tGrass
			if rng.Float64() < 0.08 {
				t = tGrassFlowers
			}
			set(x, y, t)
		}
	}
	// Forest edge to the north (Sylve d'Orme-Creux), ragged border.
	for x := 0; x < mapW; x++ {
		depth := 5 + rng.Intn(3)
		for y := 0; y < depth; y++ {
			set(x, y, tForestFloor)
		}
	}
	// River running north to south, meandering, with sandy banks.
	riverX := 31
	for y := 0; y < mapH; y++ {
		if y%5 == 0 {
			riverX += rng.Intn(3) - 1
		}
		set(riverX-2, y, tSand)
		set(riverX+3, y, tSand)
		for dx := -1; dx <= 2; dx++ {
			set(riverX+dx, y, tWater)
		}
	}
	// A path from the clearing east to a bridge across the river.
	pathY := 17
	for x := 8; x < mapW-2; x++ {
		if get(x, pathY) == tWater {
			set(x, pathY, tBridge)
			set(x, pathY+1, tBridge)
			continue
		}
		set(x, pathY, tPath)
		set(x, pathY+1, tPath)
	}
	// A path north to the forest edge.
	for y := 7; y < pathY; y++ {
		set(13, y, tPath)
	}
	// Tilled field next to the clearing (phase 1 will plant here).
	for y := 20; y < 25; y++ {
		for x := 10; x < 17; x++ {
			set(x, y, tTilled)
		}
	}

	var objects []tiledObject
	nextID := 1
	occupied := map[[2]int]bool{}
	add := func(kind string, x, y int) {
		if occupied[[2]int{x, y}] {
			return
		}
		occupied[[2]int{x, y}] = true
		objects = append(objects, tiledObject{
			ID: nextID, Type: kind, X: float64(x * tile), Y: float64(y * tile),
			Width: tile, Height: tile, Visible: true,
		})
		nextID++
	}
	free := func(x, y int) bool {
		if x < 1 || y < 1 || x >= mapW-1 || y >= mapH-1 || occupied[[2]int{x, y}] {
			return false
		}
		t := get(x, y)
		return t == tGrass || t == tGrassFlowers || t == tForestFloor
	}

	// Border of trees all around so nobody walks off the map.
	for x := 0; x < mapW; x++ {
		if get(x, 0) != tWater {
			add("arbre", x, 0)
		}
		if get(x, mapH-1) != tWater {
			add("arbre", x, mapH-1)
		}
	}
	for y := 1; y < mapH-1; y++ {
		add("arbre", 0, y)
		add("arbre", mapW-1, y)
	}
	// Dense trees in the forest, sparse ones in the meadow.
	for y := 1; y < mapH-1; y++ {
		for x := 1; x < mapW-1; x++ {
			if !free(x, y) {
				continue
			}
			p := 0.025
			if get(x, y) == tForestFloor {
				p = 0.45
			}
			if rng.Float64() < p {
				add("arbre", x, y)
			}
		}
	}
	// Rocks cluster east of the river.
	for i := 0; i < 28; i++ {
		x, y := 36+rng.Intn(10), 8+rng.Intn(22)
		if free(x, y) {
			add("rocher", x, y)
		}
	}
	// Tall grass patches around the meadow.
	for i := 0; i < 14; i++ {
		cx, cy := 3+rng.Intn(26), 9+rng.Intn(20)
		for j := 0; j < 4; j++ {
			x, y := cx+rng.Intn(3)-1, cy+rng.Intn(3)-1
			if free(x, y) && (x < 9 || x > 17 || y < 15 || y > 25) {
				add("herbes", x, y)
			}
		}
	}

	spawn := tiledObject{ID: nextID, Name: "apparition", Type: "apparition",
		X: 13 * tile, Y: 15 * tile, Width: tile, Height: tile, Visible: true}
	nextID++

	return tiledMap{
		Type: "map", Version: "1.10", TiledVersion: "1.10.2",
		Orientation: "orthogonal", RenderOrder: "right-down",
		Width: mapW, Height: mapH, TileWidth: tile, TileHeight: tile,
		NextLayerID: 4, NextObjectID: nextID,
		Layers: []tiledLayer{
			{ID: 1, Name: "sol", Type: "tilelayer", Width: mapW, Height: mapH, Data: ground, Opacity: 1, Visible: true},
			{ID: 2, Name: "ressources", Type: "objectgroup", DrawOrder: "topdown", Objects: objects, Opacity: 1, Visible: true},
			{ID: 3, Name: "reperes", Type: "objectgroup", DrawOrder: "topdown", Objects: []tiledObject{spawn}, Opacity: 1, Visible: true},
		},
		Tilesets: []tiledTileset{{
			FirstGID: 1, Name: "terrain", Image: "../tiles/terrain.png",
			ImageWidth: tile * terrainCount, ImageHeight: tile,
			TileWidth: tile, TileHeight: tile, TileCount: terrainCount, Columns: terrainCount,
			Tiles: []tiledTile{{ID: tWater, Properties: []tiledProp{{Name: "bloquant", Type: "bool", Value: true}}}},
		}},
	}
}

// ---------- output ----------

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
