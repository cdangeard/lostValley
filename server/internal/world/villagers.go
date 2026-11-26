package world

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// VillagerDef is a villager who may settle in the valley (contenu.json,
// "villageois"). They come one by one as the domain grows, each when their
// condition is met, and each player builds a friendship with them that can
// turn into a romance, then a marriage.
type VillagerDef struct {
	Name    string `json:"nom"`
	Title   string `json:"titre"`
	Place   [2]int `json:"place"` // where they settle; the nearest free tile is used
	Arrival struct {
		Building  string `json:"batiment"`    // a building of this kind stands in the domain
		DomainMin int    `json:"domaineMin"`  // the domain covers at least this many tiles
		Tamed     int    `json:"apprivoises"` // at least this many creatures were tamed
	} `json:"arrivee"`
	ArrivalText string              `json:"texteArrivee"`
	Loves       []string            `json:"adore"`
	Likes       []string            `json:"aime"`
	Hates       []string            `json:"deteste"`
	Help        string              `json:"aide"`      // what they do each morning once married
	Lines       map[string][]string `json:"repliques"` // daily lines: from N hearts ("0", "2"…), "couple", "marie"
	Reactions   map[string]string   `json:"reactions"` // to gifts: protocol.Mood*
	Scenes      map[string][]string `json:"scenes"`    // heart scenes: hearts → lines, seen once

	lineTiers []int
	scenes    []int
}

// What a spouse does each morning.
const (
	HelpWater  = "arroser" // waters the driest crops
	HelpRepair = "reparer" // mends fences and buildings
	HelpFeed   = "nourrir" // feeds the tamed creatures
)

// RomanceRules tune friendship and romance (contenu.json, "romance").
type RomanceRules struct {
	PointsPerHeart int                       `json:"pointsParCoeur"`
	MaxHearts      int                       `json:"coeursMax"`
	RomanceHearts  int                       `json:"coeursRomance"` // the bouquet can be given from here; also the cap before it
	Talk           int                       `json:"parler"`        // points for the first chat of the day
	Gift           map[string]int            `json:"cadeau"`        // points by taste: adore, aime, neutre, deteste
	WeddingDays    int                       `json:"joursAvantMariage"`
	Bouquet        string                    `json:"bouquet"`
	Pendant        string                    `json:"pendentif"`
	Recipes        map[string]map[string]int `json:"recettes"`
	SpouseWatering int                       `json:"arrosageConjoint"` // plots a spouse waters each morning
}

func (c *Content) checkVillagers() error {
	r := c.Romance
	if len(c.Villagers) == 0 {
		return nil
	}
	if r.PointsPerHeart <= 0 || r.MaxHearts <= 0 || r.RomanceHearts <= 0 || r.RomanceHearts > r.MaxHearts {
		return fmt.Errorf("romance: need 0 < coeursRomance <= coeursMax and pointsParCoeur > 0")
	}
	for _, item := range []string{r.Bouquet, r.Pendant} {
		if c.Items[item] == "" || r.Recipes[item] == nil {
			return fmt.Errorf("romance: %q needs a name in objets and a recipe", item)
		}
	}
	for item, recipe := range r.Recipes {
		for in, n := range recipe {
			if c.Items[in] == "" || n <= 0 {
				return fmt.Errorf("romance: recipe of %q uses unknown %q", item, in)
			}
		}
	}
	for id, v := range c.Villagers {
		if v.Name == "" {
			return fmt.Errorf("villager %q has no name", id)
		}
		switch v.Help {
		case HelpWater, HelpRepair, HelpFeed:
		default:
			return fmt.Errorf("villager %q: unknown help %q", id, v.Help)
		}
		for _, list := range [][]string{v.Loves, v.Likes, v.Hates} {
			for _, item := range list {
				if c.Items[item] == "" {
					return fmt.Errorf("villager %q: unknown item %q", id, item)
				}
			}
		}
		if len(v.Lines["0"]) == 0 || len(v.Lines[protocol.StageCouple]) == 0 || len(v.Lines[protocol.StageMarried]) == 0 {
			return fmt.Errorf("villager %q needs lines for 0 hearts, couple and marie", id)
		}
		for _, mood := range []string{protocol.MoodLove, protocol.MoodLike, protocol.MoodNeutral, protocol.MoodHate,
			protocol.MoodAlready, protocol.MoodRefuse, protocol.MoodBouquet, protocol.MoodPendant} {
			if v.Reactions[mood] == "" {
				return fmt.Errorf("villager %q has no reaction %q", id, mood)
			}
		}
		v.lineTiers, v.scenes = nil, nil
		for k := range v.Lines {
			if n, err := strconv.Atoi(k); err == nil {
				v.lineTiers = append(v.lineTiers, n)
			}
		}
		for k, lines := range v.Scenes {
			n, err := strconv.Atoi(k)
			if err != nil || n <= 0 || len(lines) == 0 {
				return fmt.Errorf("villager %q: bad scene %q", id, k)
			}
			v.scenes = append(v.scenes, n)
		}
		sort.Ints(v.lineTiers)
		sort.Ints(v.scenes)
		c.Villagers[id] = v
	}
	return nil
}

// Villager is a villager living in the valley.
type Villager struct {
	ID     string
	Pos    Point
	Spot   Point // where they stand and wander around
	Facing string
	Spouse string // player ID once married
	Bonds  map[string]*Bond
}

// Bond is one player's friendship with one villager.
type Bond struct {
	Points  int
	Talked  int   // day of the last chat that counted
	Gifted  int   // day of the last gift
	Seen    []int // heart scenes already played
	Stage   string
	Wedding int // day of the wedding, once engaged
}

func (d *Domain) vdef(v *Villager) VillagerDef { return d.content.Villagers[v.ID] }

func (d *Domain) bondWith(v *Villager, playerID string) *Bond {
	b := v.Bonds[playerID]
	if b == nil {
		b = &Bond{}
		v.Bonds[playerID] = b
	}
	return b
}

func (d *Domain) hearts(b *Bond) int { return b.Points / d.content.Romance.PointsPerHeart }

func (d *Domain) heartCap(b *Bond) int {
	if b.Stage == protocol.StageNone {
		return d.content.Romance.RomanceHearts
	}
	return d.content.Romance.MaxHearts
}

func (d *Domain) addPoints(b *Bond, n int) int {
	before := b.Points
	b.Points = max(0, min(b.Points+n, d.heartCap(b)*d.content.Romance.PointsPerHeart))
	return b.Points - before
}

func (d *Domain) villagerAt(p Point) *Villager {
	for _, v := range d.villagers {
		if v.Pos == p {
			return v
		}
	}
	return nil
}

func (d *Domain) sortedVillagers() []*Villager {
	out := make([]*Villager, 0, len(d.villagers))
	for _, v := range d.villagers {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *Domain) villagerView(v *Villager) protocol.VillagerView {
	vv := protocol.VillagerView{ID: v.ID, X: v.Pos.X, Y: v.Pos.Y, Facing: v.Facing}
	if p := d.players[v.Spouse]; p != nil {
		vv.Spouse = p.Name
	}
	return vv
}

func (d *Domain) villagerViews() []protocol.VillagerView {
	out := []protocol.VillagerView{}
	for _, v := range d.sortedVillagers() {
		out = append(out, d.villagerView(v))
	}
	return out
}

func (d *Domain) villagerEvent(v *Villager) Event {
	return Event{Msg: protocol.Villager{T: protocol.TypeVillager, Villager: d.villagerView(v)}}
}

func (d *Domain) bondViews(playerID string) []protocol.BondView {
	out := []protocol.BondView{}
	day := d.Day()
	for _, v := range d.sortedVillagers() {
		b := v.Bonds[playerID]
		if b == nil {
			b = &Bond{}
		}
		bv := protocol.BondView{Villager: v.ID, Points: b.Points, Hearts: d.hearts(b), MaxHearts: d.heartCap(b),
			Stage: b.Stage, Talked: b.Talked == day, Gifted: b.Gifted == day}
		if b.Stage == protocol.StageEngaged {
			bv.Wedding = b.Wedding
		}
		out = append(out, bv)
	}
	return out
}

func (d *Domain) bondsEvent(playerID string) Event {
	return Event{To: playerID, Msg: protocol.Bonds{T: protocol.TypeBonds, Bonds: d.bondViews(playerID)}}
}

// partner returns the villager a player is in a romance with, if any.
func (d *Domain) partner(playerID string) *Villager {
	for _, v := range d.villagers {
		if b := v.Bonds[playerID]; b != nil && b.Stage != protocol.StageNone {
			return v
		}
	}
	return nil
}

// taken reports whether a villager is in a romance with someone else.
func (d *Domain) taken(v *Villager, playerID string) bool {
	for id, b := range v.Bonds {
		if id != playerID && b.Stage != protocol.StageNone {
			return true
		}
	}
	return false
}

// ---------- talking and gifts ----------

func dialogue(to string, v *Villager, mood string, delta int, lines ...string) Event {
	return Event{To: to, Msg: protocol.Dialogue{T: protocol.TypeDialogue, Villager: v.ID, Mood: mood, Lines: lines, Delta: delta}}
}

// useOnVillager is Use facing a villager: an item from the bag is a gift,
// anything else (bare hands, a tool) is a chat.
func (d *Domain) useOnVillager(p *Player, v *Villager, item string) Result {
	v.Facing = dirTo(v.Pos, p.Pos)
	events := []Event{d.villagerEvent(v)}
	if item != "" && !p.hasTool(item) && p.Bag[item] > 0 {
		events = append(events, d.gift(p, v, item)...)
	} else {
		events = append(events, d.talk(p, v)...)
	}
	return Result{OK: true, Events: append(events, d.bondsEvent(p.ID))}
}

func (d *Domain) talk(p *Player, v *Villager) []Event {
	def := d.vdef(v)
	b := d.bondWith(v, p.ID)
	day := d.Day()
	delta := 0
	if b.Talked != day {
		b.Talked = day
		delta = d.addPoints(b, d.content.Romance.Talk)
	}
	// A heart scene the player has not seen yet comes first.
	for _, n := range def.scenes {
		if d.hearts(b) >= n && !seen(b.Seen, n) {
			b.Seen = append(b.Seen, n)
			return []Event{dialogue(p.ID, v, protocol.MoodScene, delta, def.Scenes[strconv.Itoa(n)]...)}
		}
	}
	return []Event{dialogue(p.ID, v, protocol.MoodTalk, delta, d.dailyLine(v, b, day))}
}

func seen(list []int, n int) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}

// dailyLine picks today's line for this bond: by romance stage, else by hearts.
func (d *Domain) dailyLine(v *Villager, b *Bond, day int) string {
	def := d.vdef(v)
	lines := def.Lines[b.Stage]
	if b.Stage == protocol.StageEngaged {
		lines = def.Lines[protocol.StageCouple]
	}
	if b.Stage == protocol.StageNone {
		h := d.hearts(b)
		for _, tier := range def.lineTiers {
			if tier <= h {
				lines = def.Lines[strconv.Itoa(tier)]
			}
		}
	}
	return lines[(day+len(v.ID))%len(lines)]
}

func (d *Domain) taste(def VillagerDef, item string) string {
	for _, l := range []struct {
		mood  string
		items []string
	}{{protocol.MoodLove, def.Loves}, {protocol.MoodLike, def.Likes}, {protocol.MoodHate, def.Hates}} {
		for _, it := range l.items {
			if it == item {
				return l.mood
			}
		}
	}
	return protocol.MoodNeutral
}

func (d *Domain) gift(p *Player, v *Villager, item string) []Event {
	def := d.vdef(v)
	r := d.content.Romance
	b := d.bondWith(v, p.ID)
	day := d.Day()
	say := func(mood string, delta int) []Event {
		return []Event{dialogue(p.ID, v, mood, delta, def.Reactions[mood])}
	}
	switch item {
	case r.Bouquet:
		if b.Stage != protocol.StageNone || d.hearts(b) < r.RomanceHearts || d.taken(v, p.ID) || d.partner(p.ID) != nil {
			return say(protocol.MoodRefuse, 0)
		}
		p.Bag[item]--
		b.Stage = protocol.StageCouple
		return append(say(protocol.MoodBouquet, 0), d.bagEvent(p), notice(p.ID, protocol.NoticeCouple, v.ID))
	case r.Pendant:
		if b.Stage != protocol.StageCouple || d.hearts(b) < r.MaxHearts {
			return say(protocol.MoodRefuse, 0)
		}
		p.Bag[item]--
		b.Stage = protocol.StageEngaged
		b.Wedding = day + max(1, r.WeddingDays)
		return append(say(protocol.MoodPendant, 0), d.bagEvent(p), notice(p.ID, protocol.NoticeEngaged, v.ID))
	}
	if b.Gifted == day {
		return say(protocol.MoodAlready, 0)
	}
	p.Bag[item]--
	b.Gifted = day
	mood := d.taste(def, item)
	delta := d.addPoints(b, r.Gift[mood])
	return append(say(mood, delta), d.bagEvent(p))
}

// Craft makes a romance token from its recipe, if the workshop stands.
func (d *Domain) Craft(id, item string, now time.Time) Result {
	p := d.players[id]
	if p == nil || !p.Online {
		return Result{Reason: ReasonNotInDomain}
	}
	recipe := d.content.Romance.Recipes[item]
	if recipe == nil {
		return Result{Reason: ReasonBadKind}
	}
	if !d.hasBuilding(BuildingWorkshop) {
		return Result{Reason: ReasonNeedsPlan}
	}
	for in, n := range recipe {
		if p.Bag[in] < n {
			return Result{Reason: ReasonNoItem}
		}
	}
	for in, n := range recipe {
		p.Bag[in] -= n
	}
	p.Bag[item]++
	return Result{OK: true, Events: []Event{d.bagEvent(p), notice(p.ID, protocol.NoticeCrafted, item)}}
}

// ---------- arrivals, weddings and help, each morning ----------

func (d *Domain) claimSize() int {
	n := 0
	for _, c := range d.claim {
		if c {
			n++
		}
	}
	return n
}

func (d *Domain) tamedCount() int {
	n := 0
	for _, c := range d.creatures {
		if c.Owner != "" {
			n++
		}
	}
	return n
}

func (d *Domain) wantsToCome(def VillagerDef) bool {
	a := def.Arrival
	if a.Building != "" && !d.hasBuilding(a.Building) {
		return false
	}
	if a.DomainMin > 0 && d.claimSize() < a.DomainMin {
		return false
	}
	if a.Tamed > 0 && d.tamedCount() < a.Tamed {
		return false
	}
	return true
}

// freeSpotNear finds the free tile closest to want (none if the map is full).
func (d *Domain) freeSpotNear(want Point, avoid ...Point) (Point, bool) {
	free := func(t Point) bool {
		if !d.Map.InBounds(t) || !d.Walkable(t) || d.villagerAt(t) != nil || d.occupied(t) {
			return false
		}
		if d.Map.HasBed && t == d.Map.Bed || d.Map.HasChest && t == d.Map.Chest {
			return false
		}
		for _, a := range avoid {
			if t == a {
				return false
			}
		}
		return true
	}
	want.X = max(0, min(want.X, d.Map.Width-1))
	want.Y = max(0, min(want.Y, d.Map.Height-1))
	for r := 0; r < max(d.Map.Width, d.Map.Height); r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if max(abs(dx), abs(dy)) != r {
					continue
				}
				if t := (Point{want.X + dx, want.Y + dy}); free(t) {
					return t, true
				}
			}
		}
	}
	return Point{}, false
}

// villagersMorning settles one newcomer whose condition is met, marries the
// engaged whose day has come, and lets spouses help.
func (d *Domain) villagersMorning() []Event {
	var events []Event
	ids := make([]string, 0, len(d.content.Villagers))
	for id := range d.content.Villagers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		def := d.content.Villagers[id]
		if d.villagers[id] != nil || !d.wantsToCome(def) {
			continue
		}
		spot, ok := d.freeSpotNear(Point{def.Place[0], def.Place[1]})
		if !ok {
			continue
		}
		v := &Villager{ID: id, Pos: spot, Spot: spot, Facing: protocol.Down, Bonds: map[string]*Bond{}}
		d.villagers[id] = v
		events = append(events, d.villagerEvent(v), Event{Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeArrival, Arg: id}})
		break // one newcomer a morning
	}

	day := d.Day()
	for _, v := range d.sortedVillagers() {
		for pid, b := range v.Bonds {
			if b.Stage != protocol.StageEngaged || day < b.Wedding {
				continue
			}
			b.Stage = protocol.StageMarried
			v.Spouse = pid
			if spot, ok := d.freeSpotNear(d.Map.WakeSpot(), d.Map.WakeSpot()); ok {
				v.Spot, v.Pos = spot, spot
			}
			events = append(events, d.villagerEvent(v), Event{Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeWedding, Arg: v.ID}})
		}
		if v.Spouse != "" {
			events = append(events, d.spouseHelp(v)...)
		}
	}
	for _, p := range d.sortedPlayers() {
		if p.Online && len(d.villagers) > 0 {
			events = append(events, d.bondsEvent(p.ID))
		}
	}
	return events
}

func (d *Domain) spouseHelp(v *Villager) []Event {
	var events []Event
	switch d.vdef(v).Help {
	case HelpWater:
		left := d.content.Romance.SpouseWatering
		for _, plot := range d.sortedPlots() {
			if left == 0 {
				break
			}
			if plot.State == protocol.PlotCrop && !plot.Watered {
				plot.Watered = true
				events = append(events, d.plotEvent(plot))
				left--
			}
		}
	case HelpRepair:
		for _, b := range d.sortedBuildings() {
			if def := d.content.Buildings[b.Kind]; def.HP > 0 && b.HP < def.HP {
				b.HP = def.HP
				events = append(events, d.buildingEvent(b))
			}
		}
	case HelpFeed:
		for _, c := range d.sortedCreatures() {
			if c.Owner != "" && !c.Fed {
				c.Fed = true
				events = append(events, d.creatureEvent(c))
			}
		}
	}
	return append(events, notice(v.Spouse, protocol.NoticeHelp, v.ID))
}

// wanderVillagers lets each villager take a step around their spot.
func (d *Domain) wanderVillagers() []Event {
	var events []Event
	dirs := []string{protocol.Up, protocol.Down, protocol.Left, protocol.Right}
	for _, v := range d.sortedVillagers() {
		if d.rng.Intn(2) == 0 {
			continue
		}
		dir := dirs[d.rng.Intn(len(dirs))]
		to := v.Pos.Step(dir)
		v.Facing = dir
		if cheb(to, v.Spot) <= 2 && d.Walkable(to) && !d.occupied(to) && d.villagerAt(to) == nil {
			v.Pos = to
		}
		events = append(events, d.villagerEvent(v))
	}
	return events
}

// ---------- saving ----------

// VillagerState is a saved villager.
type VillagerState struct {
	ID     string               `json:"id"`
	X      int                  `json:"x"`
	Y      int                  `json:"y"`
	SpotX  int                  `json:"spotX"`
	SpotY  int                  `json:"spotY"`
	Spouse string               `json:"spouse,omitempty"`
	Bonds  map[string]BondState `json:"bonds,omitempty"`
}

// BondState is a saved friendship.
type BondState struct {
	Points  int    `json:"points"`
	Talked  int    `json:"talked,omitempty"`
	Gifted  int    `json:"gifted,omitempty"`
	Seen    []int  `json:"seen,omitempty"`
	Stage   string `json:"stage,omitempty"`
	Wedding int    `json:"wedding,omitempty"`
}

func (d *Domain) villagerStates() []VillagerState {
	var out []VillagerState
	for _, v := range d.sortedVillagers() {
		vs := VillagerState{ID: v.ID, X: v.Pos.X, Y: v.Pos.Y, SpotX: v.Spot.X, SpotY: v.Spot.Y, Spouse: v.Spouse,
			Bonds: map[string]BondState{}}
		for pid, b := range v.Bonds {
			vs.Bonds[pid] = BondState{Points: b.Points, Talked: b.Talked, Gifted: b.Gifted,
				Seen: append([]int(nil), b.Seen...), Stage: b.Stage, Wedding: b.Wedding}
		}
		out = append(out, vs)
	}
	return out
}

func (d *Domain) restoreVillagers(states []VillagerState) {
	for _, vs := range states {
		if _, ok := d.content.Villagers[vs.ID]; !ok {
			continue // no longer in the content
		}
		v := &Villager{ID: vs.ID, Pos: Point{vs.X, vs.Y}, Spot: Point{vs.SpotX, vs.SpotY}, Facing: protocol.Down,
			Spouse: vs.Spouse, Bonds: map[string]*Bond{}}
		for pid, bs := range vs.Bonds {
			v.Bonds[pid] = &Bond{Points: bs.Points, Talked: bs.Talked, Gifted: bs.Gifted,
				Seen: append([]int(nil), bs.Seen...), Stage: bs.Stage, Wedding: bs.Wedding}
		}
		if !d.Walkable(v.Pos) || d.villagerAt(v.Pos) != nil {
			spot, ok := d.freeSpotNear(v.Spot)
			if !ok {
				continue
			}
			v.Pos = spot
		}
		d.villagers[v.ID] = v
	}
}
