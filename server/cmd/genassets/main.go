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
	tFloor // inside the house
	tWall
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
	fBed
	fSoilDry // tilled plot overlays
	fSoilWet
	fSeed // generic growth stages
	fSprout
	fYoung
	fRipeNavet
	fRipeBle
	fRipeFeve
	fRipeNavetLune
	fRipeBleAmbre
	fRipeFeveRonce
	fWithered
	fWeeds
	// Phase 2: creatures of the forest edge.
	fRodent
	fRodentMound // burrowed
	fCrow
	fBramble
	fBeetle
	// Phase 2: buildings, one frame per tile, row by row.
	fWorkshop // 2 × 2: four frames
	fWorkshopTR
	fWorkshopBL
	fWorkshopBR
	fWell
	fRackL // 2 × 1
	fRackR
	fScarecrow
	fFence
	fChest
	// Phase 3: creatures that can be tamed, and their habitats.
	fGoat
	fWolf
	fWolfDown // subdued: lying low
	fOwl
	fPenTL // 2 × 2: four frames
	fPenTR
	fPenBL
	fPenBR
	fDen
	fNestBox
	// Phase 4: villagers.
	fBram
	fYsolde
	fPimprenelle
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

	// Phase 1 tiles use their own random source so older tiles stay identical.
	r2 := rand.New(rand.NewSource(101))
	c.fill(tFloor, hex("#a77b4f"))
	for y := 3; y < tile; y += 4 {
		c.rect(tFloor, 0, y, tile, 1, hex("#8d6440"))
	}
	c.speckle(tFloor, r2, 10, hex("#b88a5c"))

	c.fill(tWall, hex("#6b4a2f"))
	for y := 0; y < tile; y += 5 {
		c.rect(tWall, 0, y, tile, 1, hex("#4e3521"))
	}
	c.rect(tWall, 0, 0, tile, 2, hex("#8f3f2e")) // a strip of roof
	c.speckle(tWall, r2, 8, hex("#7d5838"))
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
	drawFarm(c)
	drawCreatures(c)
	drawBuildings(c)
	drawTamable(c)
	drawVillagers(c)
	return c.img
}

// drawCreatures draws the four creatures of the forest edge.
func drawCreatures(c canvas) {
	ink := hex("#1f1a14")
	// Root rodent: a round brown body with a rootlike tail.
	c.disc(fRodent, 8, 10, 4.5, hex("#8a6a4a"))
	c.disc(fRodent, 6.5, 9, 2.5, hex("#a5845e"))
	c.rect(fRodent, 12, 11, 3, 1, hex("#6b8a3a"))
	c.px(fRodent, 14, 10, hex("#6b8a3a"))
	c.px(fRodent, 5, 8, ink)
	c.px(fRodent, 4, 6, hex("#c9a877"))
	c.px(fRodent, 7, 6, hex("#c9a877"))
	// Its mound when it hides underground.
	c.disc(fRodentMound, 8, 12, 4.5, hex("#6b4529"))
	c.disc(fRodentMound, 7, 11, 2.5, hex("#8a5f3a"))
	c.px(fRodentMound, 10, 9, hex("#6b8a3a"))
	c.px(fRodentMound, 11, 8, hex("#6b8a3a"))
	// Thieving crow: black with a pale beak, wings spread.
	c.disc(fCrow, 8, 9, 3.5, hex("#2a2a35"))
	c.rect(fCrow, 1, 7, 5, 2, hex("#3a3a48"))
	c.rect(fCrow, 10, 7, 5, 2, hex("#3a3a48"))
	c.rect(fCrow, 8, 12, 1, 3, hex("#3a3a48"))
	c.px(fCrow, 7, 8, hex("#f3d27a"))
	c.rect(fCrow, 8, 10, 2, 1, hex("#d6a24a"))
	// Bramble: a tangle of thorny stems with two glowing eyes.
	r := rand.New(rand.NewSource(303))
	c.disc(fBramble, 8, 9, 6, hex("#3b5a2a"))
	for i := 0; i < 18; i++ {
		c.px(fBramble, 2+r.Intn(12), 3+r.Intn(12), []color.RGBA{hex("#5d3a2a"), hex("#6f8f3a"), hex("#c9c0a0")}[r.Intn(3)])
	}
	c.px(fBramble, 6, 8, hex("#f0d050"))
	c.px(fBramble, 10, 8, hex("#f0d050"))
	// Pebble beetle: a grey shell split down the middle.
	c.disc(fBeetle, 8, 9, 5.5, hex("#6f727a"))
	c.disc(fBeetle, 7, 8, 3.5, hex("#8e9199"))
	c.rect(fBeetle, 8, 4, 1, 10, hex("#4a4c52"))
	c.rect(fBeetle, 6, 1, 1, 3, ink)
	c.rect(fBeetle, 10, 1, 1, 3, ink)
	for _, y := range []int{7, 10, 13} {
		c.px(fBeetle, 2, y, ink)
		c.px(fBeetle, 14, y, ink)
	}
}

// drawBuildings draws the buildings, one 16 × 16 frame per tile.
func drawBuildings(c canvas) {
	wood, woodD, roof, roofD := hex("#9a6a3c"), hex("#6b4a2a"), hex("#a8473a"), hex("#7f3329")
	stone, stoneD := hex("#8e9199"), hex("#6a6d74")

	// Workshop, 2 × 2: a red roof over a plank front with a wide door.
	for _, f := range []int{fWorkshop, fWorkshopTR} {
		c.fill(f, roof)
		for y := 2; y < tile; y += 4 {
			c.rect(f, 0, y, tile, 1, roofD)
		}
	}
	c.rect(fWorkshop, 0, 0, 1, tile, roofD)
	c.rect(fWorkshopTR, 15, 0, 1, tile, roofD)
	c.rect(fWorkshop, 10, 3, 4, 4, hex("#d9c48e")) // a sign: a saw and a hammer
	c.rect(fWorkshop, 11, 5, 3, 1, woodD)
	for _, f := range []int{fWorkshopBL, fWorkshopBR} {
		c.fill(f, wood)
		for x := 0; x < tile; x += 4 {
			c.rect(f, x, 0, 1, tile, woodD)
		}
		c.rect(f, 0, 15, tile, 1, woodD)
	}
	c.rect(fWorkshopBL, 10, 5, 6, 11, hex("#4a3220"))
	c.rect(fWorkshopBR, 0, 5, 6, 11, hex("#4a3220"))
	c.rect(fWorkshopBL, 2, 4, 5, 4, hex("#b9d4f0"))
	c.rect(fWorkshopBR, 9, 4, 5, 4, hex("#b9d4f0"))

	// Well: a stone ring around dark water, under a little roof.
	c.disc(fWell, 8, 10, 6, stone)
	c.disc(fWell, 8, 10, 4, stoneD)
	c.disc(fWell, 8, 10, 3, hex("#2d5f8f"))
	c.rect(fWell, 2, 2, 1, 7, woodD)
	c.rect(fWell, 13, 2, 1, 7, woodD)
	c.rect(fWell, 1, 1, 14, 2, roof)

	// Drying rack, 2 × 1: two posts and lines with herbs hanging.
	c.rect(fRackL, 2, 3, 2, 13, woodD)
	c.rect(fRackR, 12, 3, 2, 13, woodD)
	for _, f := range []int{fRackL, fRackR} {
		c.rect(f, 0, 4, tile, 1, hex("#c9bd9a"))
		c.rect(f, 0, 9, tile, 1, hex("#c9bd9a"))
	}
	for _, x := range []int{6, 9, 12} {
		c.rect(fRackL, x, 5, 1, 3, hex("#7aa846"))
		c.rect(fRackR, x-4, 10, 1, 3, hex("#c9a646"))
	}

	// Scarecrow: a straw head, a hat and outstretched arms.
	c.rect(fScarecrow, 7, 5, 2, 11, woodD)
	c.rect(fScarecrow, 2, 7, 12, 2, woodD)
	c.rect(fScarecrow, 5, 7, 6, 5, hex("#4f7a9e"))
	c.disc(fScarecrow, 8, 4, 2.5, hex("#e3c76a"))
	c.rect(fScarecrow, 4, 1, 8, 1, hex("#5a3d22"))
	c.rect(fScarecrow, 6, 0, 4, 1, hex("#5a3d22"))
	c.rect(fScarecrow, 1, 8, 1, 2, hex("#e3c76a"))
	c.rect(fScarecrow, 14, 8, 1, 2, hex("#e3c76a"))

	// Fence: two posts and two rails.
	c.rect(fFence, 1, 3, 3, 12, wood)
	c.rect(fFence, 12, 3, 3, 12, wood)
	c.rect(fFence, 0, 6, tile, 2, wood)
	c.rect(fFence, 0, 11, tile, 2, wood)
	c.rect(fFence, 1, 3, 3, 1, hex("#c79a62"))
	c.rect(fFence, 12, 3, 3, 1, hex("#c79a62"))

	// The house chest: a wooden box with iron bands and a lock.
	c.rect(fChest, 2, 4, 12, 10, wood)
	c.rect(fChest, 2, 4, 12, 3, hex("#b07a46"))
	c.rect(fChest, 2, 7, 12, 1, woodD)
	c.rect(fChest, 4, 4, 1, 10, stoneD)
	c.rect(fChest, 11, 4, 1, 10, stoneD)
	c.rect(fChest, 7, 7, 2, 2, hex("#e0b25c"))
}

// drawTamable draws the creatures that can be tamed and their habitats.
func drawTamable(c canvas) {
	ink := hex("#1f1a14")
	// Lichen goat: a pale body patched with green lichen, curled horns.
	c.disc(fGoat, 8, 10, 5, hex("#d8d2bf"))
	c.disc(fGoat, 4, 7, 3, hex("#e6e0cc"))
	c.disc(fGoat, 10, 9, 2, hex("#8fae5e"))
	c.disc(fGoat, 7, 12, 1.5, hex("#7a9a4e"))
	c.rect(fGoat, 5, 14, 1, 2, hex("#6b5b48"))
	c.rect(fGoat, 11, 14, 1, 2, hex("#6b5b48"))
	c.px(fGoat, 3, 7, ink)
	c.rect(fGoat, 2, 3, 2, 1, hex("#8a7a62"))
	c.px(fGoat, 1, 4, hex("#8a7a62"))
	c.rect(fGoat, 5, 3, 2, 1, hex("#8a7a62"))
	c.px(fGoat, 7, 4, hex("#8a7a62"))
	// Bramble wolf: grey-green fur bristling with thorns.
	wolf, wolfD := hex("#5e6b5a"), hex("#434d40")
	c.disc(fWolf, 8, 9, 5, wolf)
	c.disc(fWolf, 4, 6, 3, wolf)
	c.rect(fWolf, 2, 2, 1, 3, wolfD)
	c.rect(fWolf, 5, 2, 1, 3, wolfD)
	c.rect(fWolf, 12, 8, 3, 2, wolfD)
	c.px(fWolf, 3, 6, hex("#f0d050"))
	c.px(fWolf, 1, 7, ink)
	c.rect(fWolf, 5, 13, 1, 3, wolfD)
	c.rect(fWolf, 11, 13, 1, 3, wolfD)
	for _, p := range [][2]int{{7, 4}, {10, 5}, {12, 6}, {9, 7}} {
		c.px(fWolf, p[0], p[1], hex("#9a5a3a"))
	}
	// Lying low, ears flat.
	c.disc(fWolfDown, 8, 12, 4, wolf)
	c.rect(fWolfDown, 2, 10, 12, 4, wolf)
	c.disc(fWolfDown, 3, 11, 2.5, wolfD)
	c.px(fWolfDown, 2, 11, ink)
	c.rect(fWolfDown, 12, 12, 3, 1, wolfD)
	// Lantern owl: round, tawny, with a glowing chest.
	c.disc(fOwl, 8, 9, 5.5, hex("#8a6a42"))
	c.disc(fOwl, 8, 11, 3, hex("#f3d27a"))
	c.disc(fOwl, 8, 11, 1.5, hex("#fff2b8"))
	c.disc(fOwl, 6, 6, 1.8, hex("#efe6c8"))
	c.disc(fOwl, 10, 6, 1.8, hex("#efe6c8"))
	c.px(fOwl, 6, 6, ink)
	c.px(fOwl, 10, 6, ink)
	c.px(fOwl, 8, 8, hex("#d6a24a"))
	c.px(fOwl, 4, 3, hex("#6b4f30"))
	c.px(fOwl, 12, 3, hex("#6b4f30"))

	wood, woodD := hex("#9a6a3c"), hex("#6b4a2a")
	// Pen, 2 × 2: a rail fence around straw.
	for _, f := range []int{fPenTL, fPenTR, fPenBL, fPenBR} {
		c.fill(f, hex("#c9b46a"))
		r := rand.New(rand.NewSource(int64(500 + f)))
		for i := 0; i < 14; i++ {
			c.rect(f, r.Intn(14)+1, r.Intn(14)+1, 2, 1, hex("#a8924e"))
		}
	}
	for _, f := range []int{fPenTL, fPenTR} {
		c.rect(f, 0, 0, tile, 2, wood)
		c.rect(f, 0, 4, tile, 1, woodD)
	}
	for _, f := range []int{fPenBL, fPenBR} {
		c.rect(f, 0, 14, tile, 2, wood)
	}
	for _, f := range []int{fPenTL, fPenBL} {
		c.rect(f, 0, 0, 2, tile, wood)
	}
	for _, f := range []int{fPenTR, fPenBR} {
		c.rect(f, 14, 0, 2, tile, wood)
	}
	c.rect(fPenBL, 6, 8, 5, 3, hex("#5a7d9e")) // a water trough
	c.rect(fPenBL, 7, 9, 3, 1, hex("#8fb8dc"))
	// Den: a mound of stones with a dark mouth.
	c.disc(fDen, 8, 10, 7, hex("#7d8087"))
	c.disc(fDen, 7, 8, 5, hex("#979aa1"))
	c.disc(fDen, 8, 12, 3.5, hex("#2a2622"))
	c.px(fDen, 4, 6, hex("#6f8f3a"))
	c.px(fDen, 12, 7, hex("#6f8f3a"))
	// Nest box: a little house on a post, with a round hole.
	c.rect(fNestBox, 7, 9, 2, 7, woodD)
	c.rect(fNestBox, 3, 3, 10, 8, wood)
	c.rect(fNestBox, 2, 2, 12, 2, hex("#a8473a"))
	c.disc(fNestBox, 8, 7, 2, hex("#2a2016"))
}

// drawFarm draws the bed and everything that grows in a plot.
func drawFarm(c canvas) {
	r := rand.New(rand.NewSource(202))

	// Bed, seen from above: wooden frame, blanket, pillow at the top.
	c.rect(fBed, 2, 1, 12, 14, hex("#6e4a2b"))
	c.rect(fBed, 3, 2, 10, 4, hex("#efe6cf"))
	c.rect(fBed, 3, 6, 10, 8, hex("#3f6e9e"))
	c.rect(fBed, 3, 6, 10, 1, hex("#5b8ac0"))

	// Tilled soil, dry and watered, drawn over the ground.
	for _, f := range []struct {
		frame       int
		base, ridge string
	}{{fSoilDry, "#8a5f3a", "#6b4529"}, {fSoilWet, "#5a3b24", "#432a18"}} {
		c.rect(f.frame, 1, 1, 14, 14, hex(f.base))
		for y := 2; y < 15; y += 3 {
			c.rect(f.frame, 2, y, 12, 1, hex(f.ridge))
		}
	}

	leaf, leafD := hex("#6fbf4a"), hex("#4f9a35")
	// Seed: a few grains in the soil.
	for _, p := range [][2]int{{6, 8}, {9, 7}, {8, 10}} {
		c.px(fSeed, p[0], p[1], hex("#e6d6a6"))
	}
	// Sprout: two small leaves.
	c.rect(fSprout, 8, 9, 1, 4, leafD)
	c.rect(fSprout, 6, 8, 2, 1, leaf)
	c.rect(fSprout, 9, 7, 2, 1, leaf)
	// Young plant: a small tuft.
	for i := 0; i < 5; i++ {
		x := 5 + i*2
		h := 4 + r.Intn(3)
		c.rect(fYoung, x, 13-h, 1, h, []color.RGBA{leaf, leafD}[i%2])
	}

	ripeRoot := func(f int, root, top string) {
		c.disc(f, 8, 11, 3.5, hex(root))
		c.disc(f, 7, 10, 1.2, hex("#ffffff"))
		for i := 0; i < 4; i++ {
			c.rect(f, 5+i*2, 2+i%2, 1, 6, hex(top))
		}
	}
	ripeGrain := func(f int, stalk, ear string) {
		for i := 0; i < 5; i++ {
			x := 3 + i*2 + r.Intn(2)
			c.rect(f, x, 6, 1, 9, hex(stalk))
			c.rect(f, x, 2+r.Intn(2), 1, 4, hex(ear))
		}
	}
	ripePods := func(f int, leafCol, pod string) {
		c.rect(f, 7, 3, 2, 12, hex(leafCol))
		c.disc(f, 5, 6, 2.5, hex(leafCol))
		c.disc(f, 11, 9, 2.5, hex(leafCol))
		c.rect(f, 4, 9, 2, 4, hex(pod))
		c.rect(f, 10, 4, 2, 4, hex(pod))
	}
	ripeRoot(fRipeNavet, "#e9e1f0", "#5aa83f")
	ripeGrain(fRipeBle, "#c9a646", "#e8c95a")
	ripePods(fRipeFeve, "#4f9a35", "#9bd06a")
	// Wild variants: same shapes, stranger colours.
	ripeRoot(fRipeNavetLune, "#b9c7f2", "#7c8fd6")
	c.px(fRipeNavetLune, 10, 12, hex("#f6f2ff"))
	ripeGrain(fRipeBleAmbre, "#b9772f", "#f0a040")
	ripePods(fRipeFeveRonce, "#3b6b3a", "#c0476a")
	c.px(fRipeFeveRonce, 6, 4, hex("#e6e0c6"))
	c.px(fRipeFeveRonce, 12, 12, hex("#e6e0c6"))

	// Withered crop: bent brown stalks.
	for i := 0; i < 4; i++ {
		x := 4 + i*3
		c.rect(fWithered, x, 9, 1, 5, hex("#8a6a3a"))
		c.px(fWithered, x+1, 8, hex("#a5844f"))
	}
	// Weeds: a dense, messy tangle.
	for i := 0; i < 14; i++ {
		x := 1 + r.Intn(14)
		h := 3 + r.Intn(8)
		col := []color.RGBA{hex("#5d8c3a"), hex("#7aa846"), hex("#4a7030"), hex("#8f6f3a")}[r.Intn(4)]
		c.rect(fWeeds, x, tile-1-h, 1, h, col)
	}
	c.px(fWeeds, 4, 6, hex("#d6c35a"))
	c.px(fWeeds, 11, 8, hex("#d6c35a"))
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

// drawVillagers draws the three villagers facing down: Bram the smith of
// Valcendre, Ysolde the elf herbalist and Pimprenelle the fairy musician.
func drawVillagers(c canvas) {
	eye := hex("#2b2b2b")
	body := func(f int, skin, hair, top, topD, legs color.RGBA) {
		c.rect(f, 5, 8, 6, 5, top)
		c.rect(f, 5, 12, 6, 1, topD)
		c.rect(f, 5, 13, 2, 2, legs)
		c.rect(f, 9, 13, 2, 2, legs)
		c.rect(f, 5, 2, 6, 6, skin)
		c.rect(f, 5, 1, 6, 2, hair)
		c.px(f, 6, 5, eye)
		c.px(f, 9, 5, eye)
	}
	// Bram: broad, dark beard, leather apron over a grey shirt.
	body(fBram, hex("#d9a77c"), hex("#3b2a20"), hex("#7c8088"), hex("#5d6168"), hex("#3a2a1e"))
	c.rect(fBram, 4, 8, 1, 4, hex("#7c8088"))
	c.rect(fBram, 11, 8, 1, 4, hex("#7c8088"))
	c.rect(fBram, 6, 6, 4, 2, hex("#3b2a20"))
	c.rect(fBram, 6, 9, 4, 4, hex("#8a5a32"))
	// Ysolde: pale, long silver hair, pointed ears, a green robe to the ground.
	body(fYsolde, hex("#f2dcc8"), hex("#d8dde3"), hex("#3f7a4a"), hex("#2f5e39"), hex("#3f7a4a"))
	c.rect(fYsolde, 5, 2, 1, 7, hex("#d8dde3"))
	c.rect(fYsolde, 10, 2, 1, 7, hex("#d8dde3"))
	c.px(fYsolde, 4, 4, hex("#f2dcc8"))
	c.px(fYsolde, 11, 4, hex("#f2dcc8"))
	c.rect(fYsolde, 5, 13, 6, 2, hex("#2f5e39"))
	// Pimprenelle: small, pink hair, translucent wings, a flute.
	wing := color.RGBA{0xcf, 0xe8, 0xf7, 0xb0}
	c.rect(fPimprenelle, 1, 7, 4, 4, wing)
	c.rect(fPimprenelle, 11, 7, 4, 4, wing)
	body(fPimprenelle, hex("#f6d6c2"), hex("#e86fa3"), hex("#f2a7c6"), hex("#d982a8"), hex("#7a4f6a"))
	c.px(fPimprenelle, 4, 1, hex("#e86fa3"))
	c.rect(fPimprenelle, 10, 9, 1, 4, hex("#c9a46a"))
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

	houseX, houseY, houseW, houseH = 4, 9, 6, 5
	houseDoorX                     = 7
	bedX, bedY                     = 5, 10
	chestX, chestY                 = 8, 10
	// The domain at the start: 12 × 12 tiles around the house.
	domainX, domainY, domainW, domainH = 2, 8, 12, 12
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
	// A path from the house east to a bridge across the river.
	pathY := 17
	for x := houseDoorX; x < mapW-2; x++ {
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
	// The house: wooden walls, a floor, a door to the south and a bed.
	for y := houseY; y < houseY+houseH; y++ {
		for x := houseX; x < houseX+houseW; x++ {
			edge := x == houseX || y == houseY || x == houseX+houseW-1 || y == houseY+houseH-1
			if edge {
				set(x, y, tWall)
			} else {
				set(x, y, tFloor)
			}
		}
	}
	set(houseDoorX, houseY+houseH-1, tFloor)
	for y := houseY + houseH; y < pathY; y++ {
		set(houseDoorX, y, tPath)
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
	bed := tiledObject{ID: nextID, Name: "lit", Type: "lit",
		X: bedX * tile, Y: bedY * tile, Width: tile, Height: tile, Visible: true}
	nextID++
	chest := tiledObject{ID: nextID, Name: "coffre", Type: "coffre",
		X: chestX * tile, Y: chestY * tile, Width: tile, Height: tile, Visible: true}
	nextID++
	domain := tiledObject{ID: nextID, Name: "domaine", Type: "domaine",
		X: domainX * tile, Y: domainY * tile, Width: domainW * tile, Height: domainH * tile, Visible: true}
	nextID++

	blocking := tiledProp{Name: "bloquant", Type: "bool", Value: true}
	cultivable := tiledProp{Name: "cultivable", Type: "bool", Value: true}
	return tiledMap{
		Type: "map", Version: "1.10", TiledVersion: "1.10.2",
		Orientation: "orthogonal", RenderOrder: "right-down",
		Width: mapW, Height: mapH, TileWidth: tile, TileHeight: tile,
		NextLayerID: 4, NextObjectID: nextID,
		Layers: []tiledLayer{
			{ID: 1, Name: "sol", Type: "tilelayer", Width: mapW, Height: mapH, Data: ground, Opacity: 1, Visible: true},
			{ID: 2, Name: "ressources", Type: "objectgroup", DrawOrder: "topdown", Objects: objects, Opacity: 1, Visible: true},
			{ID: 3, Name: "reperes", Type: "objectgroup", DrawOrder: "topdown", Objects: []tiledObject{spawn, bed, chest, domain}, Opacity: 1, Visible: true},
		},
		Tilesets: []tiledTileset{{
			FirstGID: 1, Name: "terrain", Image: "../tiles/terrain.png",
			ImageWidth: tile * terrainCount, ImageHeight: tile,
			TileWidth: tile, TileHeight: tile, TileCount: terrainCount, Columns: terrainCount,
			Tiles: []tiledTile{
				{ID: tGrass, Properties: []tiledProp{cultivable}},
				{ID: tGrassFlowers, Properties: []tiledProp{cultivable}},
				{ID: tForestFloor, Properties: []tiledProp{{Name: "lisiere", Type: "bool", Value: true}}},
				{ID: tWater, Properties: []tiledProp{blocking, {Name: "eau", Type: "bool", Value: true}}},
				{ID: tTilled, Properties: []tiledProp{cultivable}},
				{ID: tWall, Properties: []tiledProp{blocking}},
			},
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
