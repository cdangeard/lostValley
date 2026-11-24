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
	WateringCanSize int                `json:"arrosoirCapacite"`
	Tools           map[string]ToolDef `json:"outils"`
	StarterTools    []string           `json:"outilsDeDepart"`
	StarterBag      map[string]int     `json:"sacDeDepart"`
	Plots           PlotRules          `json:"parcelles"`
	Mowing          struct {
		SeedChance float64  `json:"chanceGraine"`
		Seeds      []string `json:"graines"`
	} `json:"fauchage"`
	Crops map[string]CropDef `json:"cultures"`
	Items map[string]string  `json:"objets"`

	seedToCrop map[string]string
}

// ToolDef is one tool the player carries.
type ToolDef struct {
	Name   string `json:"nom"`
	Energy int    `json:"energie"` // energy spent per use
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
	for _, s := range c.Mowing.Seeds {
		if _, ok := c.seedToCrop[s]; !ok {
			return fmt.Errorf("fauchage gives unknown seed %q", s)
		}
	}
	return nil
}

// CropForSeed returns the crop a seed item grows, or "".
func (c *Content) CropForSeed(item string) string { return c.seedToCrop[item] }

// onlineMinute is how much real time one game minute takes while someone plays.
func (c *Content) onlineMinute() time.Duration {
	return time.Duration(c.Time.OnlineDaySeconds / minutesPerDay * float64(time.Second))
}
