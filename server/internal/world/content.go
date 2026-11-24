package world

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Content is the game's tunable data: crops, tools, the pace of time.
// It lives in assets/data/contenu.json so the game can be balanced without
// touching code, and the client reads the same file for names and labels.
type Content struct {
	Time struct {
		OnlineDaySeconds  float64  `json:"jourEnLigneSecondes"`   // real seconds per game day while someone plays
		OfflineDaySeconds float64  `json:"jourHorsLigneSecondes"` // real seconds per game day in an empty valley
		WakeHour          int      `json:"heureReveil"`           // morning starts here
		FaintHour         int      `json:"heureEvanouissement"`   // still awake at this hour: you faint
		DaysPerSeason     int      `json:"joursParSaison"`
		Seasons           []string `json:"saisons"`
		RainChance        float64  `json:"chancePluie"`
		MaxCatchUpDays    int      `json:"rattrapageMaxJours"` // longer absences are cut to this many simulated days
	} `json:"temps"`
	Energy struct {
		Max        int `json:"max"`
		AfterFaint int `json:"apresEvanouissement"`
	} `json:"energie"`
	Health struct {
		Max int `json:"max"`
	} `json:"vie"`
	WateringCanSize int                `json:"arrosoirCapacite"`
	Tools           map[string]ToolDef `json:"outils"`
	StarterTools    []string           `json:"outilsDeDepart"`
	StarterBag      map[string]int     `json:"sacDeDepart"`
	Plots           PlotRules          `json:"parcelles"`
	Mowing          struct {
		SeedChance float64  `json:"chanceGraine"`
		Seeds      []string `json:"graines"`
	} `json:"fauchage"`
	Crops     map[string]CropDef     `json:"cultures"`
	Food      map[string]int         `json:"nourriture"` // item → health restored when eaten
	Combat    CombatRules            `json:"combat"`
	Creatures map[string]CreatureDef `json:"creatures"`
	Edge      struct {
		Population int      `json:"population"` // creatures kept at the forest edge
		Species    []string `json:"especes"`
	} `json:"lisiere"`
	Wave      WaveRules              `json:"vague"`
	Buildings map[string]BuildingDef `json:"batiments"`
	Domain    struct {
		UprootRadius int `json:"rayonDessouchage"` // uprooting a stump claims this many tiles around it
	} `json:"domaine"`
	Items map[string]string `json:"objets"`

	seedToCrop map[string]string
}

// ToolDef is one tool the player carries.
type ToolDef struct {
	Name   string      `json:"nom"`
	Energy int         `json:"energie"`          // energy spent per use
	Combat *ToolAttack `json:"combat,omitempty"` // nil: not a weapon
}

// Attack patterns, relative to the attacker's position and facing.
const (
	PatternFront = "devant" // the tile in front
	PatternArc   = "arc"    // the tile in front and its two neighbours across
	PatternCross = "croix"  // the four tiles around
	PatternLine2 = "ligne2" // two tiles straight ahead
)

// Tool effects in combat.
const (
	EffectUnearth = "deterre"  // hits burrowed creatures and brings them up
	EffectKnock   = "repousse" // pushes the creature back one tile
	EffectStun    = "etourdit" // the creature loses its next turn
)

// ToolAttack is what a tool does to a creature.
type ToolAttack struct {
	Pattern string `json:"motif"`
	Damage  int    `json:"degats"`
	Ticks   int    `json:"ticks"` // combat time the swing takes
	Effect  string `json:"effet,omitempty"`
}

// CombatRules tune the step-by-step combat.
type CombatRules struct {
	EngageRadius int     `json:"rayonEngagement"`  // a hostile creature this close puts a player "au pas"
	MoveTicks    int     `json:"ticksDeplacement"` // a player's step costs this many ticks
	RealTickMs   int     `json:"tickTempsReelMs"`  // with nobody au pas, creatures act at this real pace per tick
	TurnSeconds  float64 `json:"delaiTourSecondes"`
	FleeRadius   int     `json:"rayonFuite"` // leaving the game with a creature this close counts as falling
}

// Creature traits: each one is undone by one tool.
const (
	TraitBurrow = "enfoui"   // hides underground between attacks; only the hoe reaches it there
	TraitFly    = "vol"      // flies over fences and obstacles; scarecrows keep it away
	TraitBark   = "ecorce"   // armour that the axe breaks for good
	TraitShell  = "carapace" // armour that the pickaxe goes through
)

// CreatureDef is one species.
type CreatureDef struct {
	Name      string `json:"nom"`
	HP        int    `json:"pv"`
	Armor     int    `json:"armure"`
	MoveTicks int    `json:"ticksDeplacement"`
	Attack    struct {
		Pattern string `json:"motif"`
		Damage  int    `json:"degats"`
		Windup  int    `json:"preparation"` // ticks between the warning and the blow
	} `json:"attaque"`
	Weakness string             `json:"faiblesse"` // this tool deals double and ignores armour
	Trait    string             `json:"trait"`
	Loot     map[string]float64 `json:"butin"` // item → count; a fraction is a chance of one
}

// WaveRules size the night attack on the fields.
type WaveRules struct {
	Hour         int      `json:"heure"`
	Base         int      `json:"base"`
	PerCrops     int      `json:"parCultures"` // one more creature per this many crops
	PerDays      int      `json:"parJours"`    // one more creature per this many days
	Max          int      `json:"max"`
	EarlySpecies []string `json:"especesDebut"`
	Species      []string `json:"especes"`
	EarlyDays    int      `json:"joursAvantGrosses"` // before this day, only EarlySpecies come
}

// BuildingDef is something a player can build in the domain.
type BuildingDef struct {
	Name     string         `json:"nom"`
	W        int            `json:"largeur"`
	H        int            `json:"hauteur"`
	Cost     map[string]int `json:"cout"`
	Requires string         `json:"requiert,omitempty"` // another building that must stand in the domain
	HP       int            `json:"pv,omitempty"`       // > 0: creatures can break it
	Capacity int            `json:"capacite,omitempty"` // drying rack: crops per night
	Radius   int            `json:"rayon,omitempty"`    // scarecrow: how far crows keep away
}

// PlotRules says how fast untended plots decay. Counts are in game days.
type PlotRules struct {
	IdleDaysToWild     int `json:"joursSansSoinAvantFriche"` // tilled soil left empty and dry
	DryDaysToWither    int `json:"joursSecsAvantFane"`       // a growing crop left unwatered
	WitheredDaysToSeed int `json:"joursFaneAvantGraines"`    // a withered crop drops its seeds
	RipeDaysToSeed     int `json:"joursMurAvantGraines"`     // a ripe crop nobody harvests
}

// CropDef is one kind of crop.
type CropDef struct {
	Name           string  `json:"nom"`
	Seed           string  `json:"graine"`
	Days           int     `json:"jours"` // watered days to ripen
	Harvest        string  `json:"recolte"`
	Amount         int     `json:"quantite"`
	Mutation       string  `json:"mutation,omitempty"`       // wild variant its fallen seeds may turn into
	MutationChance float64 `json:"chanceMutation,omitempty"` // chance per fall of seeds
	Wild           bool    `json:"sauvage,omitempty"`        // grows without water and never withers
	SeedChance     float64 `json:"chanceGraine,omitempty"`   // chance a harvest also gives a seed back
}

// LoadContent reads and checks the content file.
func LoadContent(path string) (*Content, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseContent(raw)
}

// ParseContent parses and checks content JSON.
func ParseContent(raw []byte) (*Content, error) {
	var c Content
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse content: %w", err)
	}
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("parse content: %w", err)
	}
	return &c, nil
}

func (c *Content) check() error {
	t := c.Time
	if t.OnlineDaySeconds <= 0 || t.OfflineDaySeconds <= 0 {
		return fmt.Errorf("day lengths must be positive")
	}
	if t.WakeHour < 0 || t.WakeHour > 23 || t.FaintHour < 0 || t.FaintHour >= t.WakeHour {
		return fmt.Errorf("need 0 <= faint hour < wake hour <= 23")
	}
	if t.DaysPerSeason <= 0 || len(t.Seasons) == 0 {
		return fmt.Errorf("need seasons")
	}
	if t.MaxCatchUpDays <= 0 {
		c.Time.MaxCatchUpDays = 120
	}
	if c.Energy.Max <= 0 || c.WateringCanSize <= 0 {
		return fmt.Errorf("energy max and watering can size must be positive")
	}
	for _, name := range c.StarterTools {
		if _, ok := c.Tools[name]; !ok {
			return fmt.Errorf("starter tool %q is not in outils", name)
		}
	}
	c.seedToCrop = map[string]string{}
	for id, crop := range c.Crops {
		if crop.Days <= 0 || crop.Harvest == "" || crop.Amount <= 0 || crop.Seed == "" {
			return fmt.Errorf("crop %q: needs graine, jours, recolte and quantite", id)
		}
		if crop.Mutation != "" {
			if _, ok := c.Crops[crop.Mutation]; !ok {
				return fmt.Errorf("crop %q mutates into unknown crop %q", id, crop.Mutation)
			}
		}
		if other, dup := c.seedToCrop[crop.Seed]; dup {
			return fmt.Errorf("crops %q and %q share seed %q", id, other, crop.Seed)
		}
		c.seedToCrop[crop.Seed] = id
	}
	if c.Health.Max <= 0 {
		return fmt.Errorf("vie.max must be positive")
	}
	for name, t := range c.Tools {
		if a := t.Combat; a != nil && (!validPattern(a.Pattern) || a.Ticks <= 0) {
			return fmt.Errorf("tool %q: bad combat pattern or ticks", name)
		}
	}
	if c.Combat.EngageRadius <= 0 || c.Combat.MoveTicks <= 0 || c.Combat.RealTickMs <= 0 || c.Combat.TurnSeconds <= 0 {
		return fmt.Errorf("combat: radius, ticks and delays must be positive")
	}
	for id, cr := range c.Creatures {
		if cr.HP <= 0 || cr.MoveTicks <= 0 || cr.Attack.Windup <= 0 || !validPattern(cr.Attack.Pattern) {
			return fmt.Errorf("creature %q: needs pv, ticksDeplacement, attaque.preparation and a known motif", id)
		}
		if cr.Weakness != "" {
			if _, ok := c.Tools[cr.Weakness]; !ok {
				return fmt.Errorf("creature %q is weak to unknown tool %q", id, cr.Weakness)
			}
		}
	}
	for _, list := range [][]string{c.Edge.Species, c.Wave.Species, c.Wave.EarlySpecies} {
		for _, sp := range list {
			if _, ok := c.Creatures[sp]; !ok {
				return fmt.Errorf("unknown creature %q in lisiere or vague", sp)
			}
		}
	}
	if len(c.Wave.Species) == 0 || c.Wave.Hour < 0 || c.Wave.Hour > 23 {
		return fmt.Errorf("vague: needs especes and an hour")
	}
	for id, b := range c.Buildings {
		if b.W <= 0 || b.H <= 0 || len(b.Cost) == 0 {
			return fmt.Errorf("building %q: needs largeur, hauteur and cout", id)
		}
		if b.Requires != "" {
			if _, ok := c.Buildings[b.Requires]; !ok {
				return fmt.Errorf("building %q requires unknown building %q", id, b.Requires)
			}
		}
	}
	for _, s := range c.Mowing.Seeds {
		if _, ok := c.seedToCrop[s]; !ok {
			return fmt.Errorf("fauchage gives unknown seed %q", s)
		}
	}
	return nil
}

func validPattern(p string) bool {
	return p == PatternFront || p == PatternArc || p == PatternCross || p == PatternLine2
}

// CropForSeed returns the crop a seed item grows, or "".
func (c *Content) CropForSeed(item string) string { return c.seedToCrop[item] }

// onlineMinute is how much real time one game minute takes while someone plays.
func (c *Content) onlineMinute() time.Duration {
	return time.Duration(c.Time.OnlineDaySeconds / minutesPerDay * float64(time.Second))
}
