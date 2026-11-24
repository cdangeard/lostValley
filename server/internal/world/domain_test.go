package world

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// testMap builds a small Tiled map from ASCII art:
// '.' grass, '~' water (blocking), 'T' tree, 'R' rock, 'h' tall grass, '@' spawn.
func testMap(t *testing.T, rows ...string) *Map {
	t.Helper()
	w, h := len(rows[0]), len(rows)
	data := make([]string, 0, w*h)
	var objects []string
	var spawn string
	id := 1
	for y, row := range rows {
		for x, c := range row {
			gid := "1"
			if c == '~' {
				gid = "2"
			}
			data = append(data, gid)
			kind := map[rune]string{'T': "arbre", 'R': "rocher", 'h': "herbes"}[c]
			if kind != "" {
				objects = append(objects, fmt.Sprintf(`{"id":%d,"type":%q,"x":%d,"y":%d}`, id, kind, x*16, y*16))
				id++
			}
			if c == '@' {
				spawn = fmt.Sprintf(`{"id":%d,"name":"apparition","x":%d,"y":%d}`, id, x*16, y*16)
				id++
			}
		}
	}
	raw := fmt.Sprintf(`{
	  "width":%d,"height":%d,"tilewidth":16,"tileheight":16,
	  "layers":[
	    {"name":"sol","type":"tilelayer","data":[%s]},
	    {"name":"ressources","type":"objectgroup","objects":[%s]},
	    {"name":"reperes","type":"objectgroup","objects":[%s]}
	  ],
	  "tilesets":[{"firstgid":1,"tiles":[{"id":1,"properties":[{"name":"bloquant","type":"bool","value":true}]}]}]
	}`, w, h, strings.Join(data, ","), strings.Join(objects, ","), spawn)
	m, err := ParseMap([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

var t0 = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func joined(t *testing.T, m *Map, ids ...string) *Domain {
	t.Helper()
	d := NewDomain(m)
	for _, id := range ids {
		d.Join(id, id)
	}
	return d
}

func TestParseMap(t *testing.T) {
	m := testMap(t,
		"~~~~",
		".@T.",
		".Rh.",
	)
	if m.Width != 4 || m.Height != 3 {
		t.Fatalf("size = %dx%d", m.Width, m.Height)
	}
	if m.Spawn != (Point{1, 1}) {
		t.Fatalf("spawn = %v", m.Spawn)
	}
	if !m.Blocked(Point{0, 0}) || m.Blocked(Point{0, 1}) {
		t.Fatal("water should block, grass should not")
	}
	if !m.Blocked(Point{-1, 1}) || !m.Blocked(Point{4, 1}) {
		t.Fatal("outside the map should block")
	}
	if len(m.Resources) != 3 {
		t.Fatalf("resources = %v", m.Resources)
	}
}

func TestParseMapRejectsBadMaps(t *testing.T) {
	cases := map[string]string{
		"no spawn":     `{"width":1,"height":1,"tilewidth":16,"tileheight":16,"layers":[{"name":"sol","type":"tilelayer","data":[1]}]}`,
		"no ground":    `{"width":1,"height":1,"tilewidth":16,"tileheight":16,"layers":[]}`,
		"short ground": `{"width":2,"height":1,"tilewidth":16,"tileheight":16,"layers":[{"name":"sol","type":"tilelayer","data":[1]}]}`,
		"unknown kind": `{"width":1,"height":1,"tilewidth":16,"tileheight":16,"layers":[{"name":"sol","type":"tilelayer","data":[1]},{"name":"ressources","type":"objectgroup","objects":[{"type":"licorne","x":0,"y":0}]}]}`,
	}
	for name, raw := range cases {
		if _, err := ParseMap([]byte(raw)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseMapReadsTiledClassField(t *testing.T) {
	raw := `{"width":2,"height":1,"tilewidth":16,"tileheight":16,"layers":[
	  {"name":"sol","type":"tilelayer","data":[1,1]},
	  {"name":"ressources","type":"objectgroup","objects":[{"class":"arbre","x":16,"y":0}]},
	  {"name":"reperes","type":"objectgroup","objects":[{"name":"apparition","x":0,"y":0}]}]}`
	m, err := ParseMap([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Resources) != 1 || m.Resources[0].Kind != "arbre" {
		t.Fatalf("resources = %v", m.Resources)
	}
}

func TestMove(t *testing.T) {
	m := testMap(t,
		"~~~",
		".@T",
		"...",
	)
	d := joined(t, m, "ana")
	p := d.Player("ana")

	if r := d.Move("ana", protocol.Up, t0); r.OK || r.Reason != ReasonBlocked {
		t.Fatalf("into water: %+v", r)
	}
	if p.Facing != protocol.Up || p.Pos != (Point{1, 1}) {
		t.Fatalf("blocked move should only turn: %+v", p)
	}
	if r := d.Move("ana", protocol.Right, t0); r.OK {
		t.Fatalf("into a tree: %+v", r)
	}
	if r := d.Move("ana", protocol.Down, t0); !r.OK {
		t.Fatalf("onto grass: %+v", r)
	}
	if p.Pos != (Point{1, 2}) {
		t.Fatalf("pos = %v", p.Pos)
	}
	if r := d.Move("ana", protocol.Left, t0.Add(MoveCooldown/2)); r.Reason != ReasonTooFast {
		t.Fatalf("second step too soon: %+v", r)
	}
	if r := d.Move("ana", protocol.Down, t0.Add(time.Second)); r.Reason != ReasonBlocked {
		t.Fatalf("off the map: %+v", r)
	}
	if r := d.Move("ana", "north", t0.Add(time.Second)); r.Reason != ReasonBadDir {
		t.Fatalf("bad direction: %+v", r)
	}
	if r := d.Move("bob", protocol.Up, t0); r.Reason != ReasonNotInDomain {
		t.Fatalf("unknown player: %+v", r)
	}
}

func TestPlayersDoNotBlockEachOther(t *testing.T) {
	m := testMap(t, ".@.")
	d := joined(t, m, "ana", "bob")
	if r := d.Move("ana", protocol.Right, t0); !r.OK {
		t.Fatal(r)
	}
	if r := d.Move("bob", protocol.Right, t0); !r.OK {
		t.Fatal(r)
	}
}

func TestGatherTreeYieldsWoodAndLeavesAStump(t *testing.T) {
	m := testMap(t, "@T.")
	d := joined(t, m, "ana")
	d.Move("ana", protocol.Right, t0) // blocked: turns to face the tree

	now := t0
	for i := 0; i < ResourceKinds["arbre"].Hits; i++ {
		now = now.Add(time.Second)
		if r := d.Gather("ana", now); !r.OK {
			t.Fatalf("hit %d: %+v", i, r)
		}
	}
	if got := d.Player("ana").Inventory["bois"]; got != ResourceKinds["arbre"].Amount {
		t.Fatalf("bois = %d", got)
	}
	if r := d.Gather("ana", now.Add(time.Second)); r.Reason != ReasonNothing {
		t.Fatalf("depleted tree: %+v", r)
	}
	if d.Walkable(Point{1, 0}) {
		t.Fatal("a stump should still block")
	}
}

func TestGatherCooldownAndEmptyTile(t *testing.T) {
	m := testMap(t, "@R.")
	d := joined(t, m, "ana")
	if r := d.Gather("ana", t0); r.Reason != ReasonNothing {
		t.Fatalf("facing down onto nothing: %+v", r)
	}
	d.Move("ana", protocol.Right, t0)
	if r := d.Gather("ana", t0.Add(time.Second)); !r.OK {
		t.Fatal(r)
	}
	if r := d.Gather("ana", t0.Add(time.Second+GatherCooldown/2)); r.Reason != ReasonTooFast {
		t.Fatalf("gather spam: %+v", r)
	}
}

func TestInventoryEventGoesOnlyToGatherer(t *testing.T) {
	m := testMap(t, "@h.")
	d := joined(t, m, "ana", "bob")
	d.Move("ana", protocol.Right, t0)
	r := d.Gather("ana", t0.Add(time.Second))
	var inv, res int
	for _, e := range r.Events {
		switch e.Msg.(type) {
		case protocol.Inventory:
			inv++
			if e.To != "ana" {
				t.Fatalf("inventory sent to %q", e.To)
			}
		case protocol.Resource:
			res++
			if e.To != "" {
				t.Fatal("resource change should be broadcast")
			}
		}
	}
	if inv != 1 || res != 1 {
		t.Fatalf("events = %+v", r.Events)
	}
}

func TestRegrowth(t *testing.T) {
	m := testMap(t, "@h.")
	d := joined(t, m, "ana")
	d.Move("ana", protocol.Right, t0)
	d.Gather("ana", t0.Add(time.Second))

	// Cut grass can be walked on, and does not regrow under a player.
	if r := d.Move("ana", protocol.Right, t0.Add(2*time.Second)); !r.OK {
		t.Fatalf("walk onto cut grass: %+v", r)
	}
	later := t0.Add(time.Hour)
	if ev := d.Tick(later); len(ev) != 0 {
		t.Fatalf("regrew under the player: %+v", ev)
	}
	d.Move("ana", protocol.Right, later)
	if ev := d.Tick(later); len(ev) != 1 {
		t.Fatalf("expected regrowth once the tile is free, got %+v", ev)
	}
	if d.Walkable(Point{1, 0}) {
		t.Fatal("regrown grass should block again")
	}
}

func TestRegrowthHappensWhileOffline(t *testing.T) {
	m := testMap(t, "@T.")
	d := joined(t, m, "ana")
	d.Move("ana", protocol.Right, t0)
	now := t0
	for i := 0; i < ResourceKinds["arbre"].Hits; i++ {
		now = now.Add(time.Second)
		d.Gather("ana", now)
	}
	d.Leave("ana")
	if ev := d.Tick(now.Add(ResourceKinds["arbre"].Regrow + time.Second)); len(ev) != 1 {
		t.Fatalf("tree should regrow with nobody online: %+v", ev)
	}
}

func TestRejoinKeepsPositionAndBag(t *testing.T) {
	m := testMap(t, "@h.")
	d := joined(t, m, "ana")
	d.Move("ana", protocol.Right, t0)
	d.Gather("ana", t0.Add(time.Second))
	d.Move("ana", protocol.Right, t0.Add(2*time.Second))
	d.Leave("ana")

	w := d.Welcome("bob", "test")
	if len(w.Players) != 0 {
		t.Fatalf("offline players should not be listed: %+v", w.Players)
	}

	d.Join("ana", "")
	p := d.Player("ana")
	if p.Pos != (Point{1, 0}) || p.Inventory["fibre"] != 1 || p.Name != "ana" {
		t.Fatalf("player after rejoin: %+v", p)
	}
}

// The committed valley must load, and its spawn must not be boxed in.
func TestValleyMapLoads(t *testing.T) {
	m, err := LoadMap("../../../assets/maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	d := NewDomain(m)
	seen := map[Point]bool{m.Spawn: true}
	queue := []Point{m.Spawn}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, dir := range []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right} {
			n := p.Step(dir)
			if !seen[n] && d.Walkable(n) {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	if len(seen) < m.Width*m.Height/3 {
		t.Fatalf("only %d tiles reachable from spawn", len(seen))
	}
	kinds := map[string]int{}
	for _, r := range m.Resources {
		kinds[r.Kind]++
	}
	for k := range ResourceKinds {
		if kinds[k] == 0 {
			t.Errorf("no %q on the valley map", k)
		}
	}
}
