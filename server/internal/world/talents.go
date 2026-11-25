package world

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// The Constellation des Racines is the passive talent graph every player
// shares. Everyone starts on the heart; each level gives one point, and a
// point takes a node linked to one already taken. Taking a point back costs
// seeds of oblivion and is only allowed when the rest stays linked to the
// heart. The graph is data (assets/data/constellation.json), checked here.

// Node types.
const (
	NodeHeart    = "coeur"
	NodeSmall    = "petit"
	NodeNotable  = "notable"
	NodeKeystone = "angulaire"
)

// Modifiers a node can grant. Percentages are whole numbers (5 = 5 %).
const (
	ModYield        = "rendement"            // % chance a harvest gives one more
	ModSeedBack     = "graineRetour"         // % chance a harvest gives a seed back
	ModMutation     = "mutation"             // % added to the mutation chance of what you sow
	ModCan          = "arrosoir"             // watering can size
	ModDew          = "rosee"                // % chance each morning that your dry seedlings get watered
	ModSaveEnergy   = "economieEnergie"      // % chance a tool use costs no energy
	ModTrust        = "confiance"            // % more trust per meal
	ModProduction   = "production"           // % chance a production creature gives double
	ModPetHP        = "vieCompagnons"        // health of your creatures
	ModPetDamage    = "degatsCompagnons"     // damage of your creatures
	ModFollowers    = "compagnons"           // creatures that may follow you at once
	ModMaterials    = "materiaux"            // % chance a tree or rock gives one more
	ModEnergyMax    = "energieMax"           // energy
	ModBuildSaving  = "economieConstruction" // % off building costs
	ModFence        = "cloture"              // health of the fences you build
	ModHPMax        = "vieMax"               // health
	ModDodge        = "esquive"              // % chance to dodge a creature's blow
	ModNight        = "nuit"                 // damage at night
	ModGreenThumb   = "mainVerte"            // keystone: see the constellation file
	ModPackLeader   = "meneurDeMeute"        // keystone: see the constellation file
	modToolDmgStart = "degats"               // degatsHoue, degatsFaux… damage with one tool
)

var knownMods = map[string]bool{
	ModYield: true, ModSeedBack: true, ModMutation: true, ModCan: true, ModDew: true, ModSaveEnergy: true,
	ModTrust: true, ModProduction: true, ModPetHP: true, ModPetDamage: true, ModFollowers: true,
	ModMaterials: true, ModEnergyMax: true, ModBuildSaving: true, ModFence: true,
	ModHPMax: true, ModDodge: true, ModNight: true, ModGreenThumb: true, ModPackLeader: true,
	"degatsHoue": true, "degatsFaux": true, "degatsHache": true, "degatsPioche": true,
}

// TalentNode is one node of the constellation.
type TalentNode struct {
	ID          string         `json:"id"`
	Name        string         `json:"nom"`
	Type        string         `json:"type"`
	Region      string         `json:"region"`
	X           float64        `json:"x"`
	Y           float64        `json:"y"`
	Mods        map[string]int `json:"mods"`
	Description string         `json:"description,omitempty"`
}

// Constellation is the talent graph.
type Constellation struct {
	Nodes []TalentNode `json:"noeuds"`
	Links [][2]string  `json:"liens"`

	byID  map[string]*TalentNode
	adj   map[string][]string
	Heart string
}

// LoadConstellation reads and checks the talent graph.
func LoadConstellation(path string) (*Constellation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseConstellation(raw)
}

// ParseConstellation parses and checks a talent graph: unique nodes, known
// modifiers, links between existing nodes, one heart, and every node
// reachable from it.
func ParseConstellation(raw []byte) (*Constellation, error) {
	var c Constellation
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse constellation: %w", err)
	}
	c.byID = map[string]*TalentNode{}
	c.adj = map[string][]string{}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		if n.ID == "" || c.byID[n.ID] != nil {
			return nil, fmt.Errorf("constellation: empty or duplicate node id %q", n.ID)
		}
		switch n.Type {
		case NodeHeart:
			if c.Heart != "" {
				return nil, fmt.Errorf("constellation: two hearts (%q and %q)", c.Heart, n.ID)
			}
			c.Heart = n.ID
		case NodeSmall, NodeNotable, NodeKeystone:
		default:
			return nil, fmt.Errorf("constellation: node %q has unknown type %q", n.ID, n.Type)
		}
		for k := range n.Mods {
			if !knownMods[k] {
				return nil, fmt.Errorf("constellation: node %q has unknown modifier %q", n.ID, k)
			}
		}
		c.byID[n.ID] = n
	}
	if c.Heart == "" {
		return nil, fmt.Errorf("constellation: no heart")
	}
	for _, l := range c.Links {
		a, b := l[0], l[1]
		if c.byID[a] == nil || c.byID[b] == nil || a == b {
			return nil, fmt.Errorf("constellation: bad link %q–%q", a, b)
		}
		c.adj[a] = append(c.adj[a], b)
		c.adj[b] = append(c.adj[b], a)
	}
	if seen := c.connected(map[string]bool{}, ""); len(seen) != len(c.Nodes) {
		return nil, fmt.Errorf("constellation: %d of %d nodes cannot be reached from the heart", len(c.Nodes)-len(seen), len(c.Nodes))
	}
	return &c, nil
}

// Node returns a node by ID, or nil.
func (c *Constellation) Node(id string) *TalentNode { return c.byID[id] }

// Linked reports whether two nodes share a link.
func (c *Constellation) Linked(a, b string) bool {
	for _, n := range c.adj[a] {
		if n == b {
			return true
		}
	}
	return false
}

// connected floods from the heart. With a non-empty taken set, only the
// taken nodes are walked; without, the whole graph. skip is left out.
func (c *Constellation) connected(taken map[string]bool, skip string) map[string]bool {
	seen := map[string]bool{c.Heart: true}
	queue := []string{c.Heart}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, m := range c.adj[n] {
			if seen[m] || m == skip || (len(taken) > 0 && !taken[m]) {
				continue
			}
			seen[m] = true
			queue = append(queue, m)
		}
	}
	return seen
}

// ---------- a player's progress ----------

// XPForLevel is the total experience needed to reach level n.
func (c *Content) XPForLevel(n int) int {
	r := c.Progress
	if n <= 1 {
		return 0
	}
	k := n - 1
	return k*r.FirstLevelXP + r.LevelStepXP*k*(k-1)/2
}

// Points is how many talent points p has not spent yet.
func (p *Player) Points() int { return p.Level - 1 - len(p.Talents) }

func (p *Player) mod(key string) int { return p.mods[key] }

// recomputeMods sums the modifiers of every node p took.
func (d *Domain) recomputeMods(p *Player) {
	p.mods = map[string]int{}
	tree := d.content.Constellation
	if tree == nil {
		return
	}
	for id := range p.Talents {
		if n := tree.Node(id); n != nil {
			for k, v := range n.Mods {
				p.mods[k] += v
			}
		}
	}
}

func (d *Domain) maxHP(p *Player) int     { return d.content.Health.Max + p.mod(ModHPMax) }
func (d *Domain) maxEnergy(p *Player) int { return d.content.Energy.Max + p.mod(ModEnergyMax) }
func (d *Domain) canSize(p *Player) int   { return d.content.WateringCanSize + p.mod(ModCan) }

// chance rolls a percentage.
func (d *Domain) chance(pct int) bool { return pct > 0 && d.rng.Intn(100) < pct }

// clampStats keeps health, energy and water within the player's maximums
// after talents changed them.
func (d *Domain) clampStats(p *Player) {
	p.HP = min(p.HP, d.maxHP(p))
	p.Energy = min(p.Energy, d.maxEnergy(p))
	p.Water = min(p.Water, d.canSize(p))
}

func (d *Domain) progressView(p *Player) protocol.ProgressView {
	v := protocol.ProgressView{Level: p.Level, XP: p.XP, LevelXP: d.content.XPForLevel(p.Level),
		NextXP: d.content.XPForLevel(p.Level + 1), Points: p.Points(), Talents: []string{}}
	if p.Level >= d.content.Progress.MaxLevel {
		v.NextXP = 0
	}
	for id := range p.Talents {
		v.Talents = append(v.Talents, id)
	}
	sort.Strings(v.Talents)
	return v
}

func (d *Domain) progressEvent(p *Player) Event {
	return Event{To: p.ID, Msg: protocol.Progress{T: protocol.TypeProgress, Progress: d.progressView(p)}}
}

// gainXP rewards an activity. Each new level gives one talent point.
func (d *Domain) gainXP(p *Player, activity string) []Event {
	amount := d.content.Progress.XP[activity]
	if p == nil || amount <= 0 {
		return nil
	}
	p.XP += amount
	var events []Event
	for p.Level < d.content.Progress.MaxLevel && p.XP >= d.content.XPForLevel(p.Level+1) {
		p.Level++
		events = append(events, Event{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeLevelUp, Arg: strconv.Itoa(p.Level)}})
	}
	return append([]Event{d.progressEvent(p)}, events...)
}

// Reasons a talent change is refused.
const (
	ReasonNoPoint    = "no_point"    // no talent point left
	ReasonNotLinked  = "not_linked"  // the node is not next to one already taken
	ReasonNotLeaf    = "not_leaf"    // taking it back would cut others off from the heart
	ReasonNoOblivion = "no_oblivion" // no seed of oblivion to take a point back
	ReasonBadNode    = "unknown_node"
)

// Talent takes (protocol.TalentTake) or gives back (protocol.TalentRefund) a
// node of the constellation. It is a menu action: it costs no turn.
func (d *Domain) Talent(id, node, op string, now time.Time) Result {
	p := d.players[id]
	if p == nil || !p.Online {
		return Result{Reason: ReasonNotInDomain}
	}
	tree := d.content.Constellation
	if tree == nil || tree.Node(node) == nil || node == tree.Heart {
		return Result{Reason: ReasonBadNode}
	}
	var events []Event
	switch op {
	case protocol.TalentTake:
		if p.Talents[node] {
			return Result{Reason: ReasonNothing}
		}
		if p.Points() <= 0 {
			return Result{Reason: ReasonNoPoint}
		}
		linked := false
		for _, m := range tree.adj[node] {
			if m == tree.Heart || p.Talents[m] {
				linked = true
				break
			}
		}
		if !linked {
			return Result{Reason: ReasonNotLinked}
		}
		p.Talents[node] = true
	case protocol.TalentRefund:
		if !p.Talents[node] {
			return Result{Reason: ReasonNothing}
		}
		if reach := tree.connected(p.Talents, node); len(reach) != len(p.Talents) {
			// The heart plus every other taken node must stay linked.
			return Result{Reason: ReasonNotLeaf}
		}
		cost := d.content.Progress.RefundCost
		if p.Bag[OblivionSeed] < cost {
			return Result{Reason: ReasonNoOblivion}
		}
		p.Bag[OblivionSeed] -= cost
		delete(p.Talents, node)
		events = append(events, d.bagEvent(p))
	default:
		return Result{Reason: ReasonNothing}
	}
	d.recomputeMods(p)
	d.clampStats(p)
	events = append(events, d.progressEvent(p), d.statsEvent(p), d.playerEvent(p))
	events = append(events, d.followerLimit(p)...)
	return Result{OK: true, Events: events}
}
