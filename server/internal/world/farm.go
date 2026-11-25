package world

import (
	"sort"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

// Plot is one farmed tile: tilled soil, a crop, a withered crop or weeds.
type Plot struct {
	At      Point
	State   string // protocol.PlotTilled, PlotCrop, PlotWithered or PlotWild
	Crop    string // crop ID while State is PlotCrop or PlotWithered
	Growth  int    // watered days so far
	Watered bool   // watered today (or rained on)
	Neglect int    // consecutive days without care; meaning depends on State
	Sower   string // who sowed the crop: their talents shape how it grows
	DryDays int    // green thumb: dry days since it last grew
}

func (d *Domain) crop(p *Plot) CropDef { return d.content.Crops[p.Crop] }

func (d *Domain) ripe(p *Plot) bool {
	return p.State == protocol.PlotCrop && p.Growth >= d.crop(p).Days
}

func (d *Domain) plotView(p *Plot) protocol.PlotView {
	v := protocol.PlotView{X: p.At.X, Y: p.At.Y, State: p.State, Watered: p.Watered}
	if p.State == protocol.PlotCrop || p.State == protocol.PlotWithered {
		v.Crop, v.Growth, v.Days = p.Crop, p.Growth, d.crop(p).Days
	}
	return v
}

func (d *Domain) plotEvent(p *Plot) Event {
	return Event{Msg: protocol.Plot{T: protocol.TypePlot, Plot: d.plotView(p)}}
}

// plotGoneEvent tells clients a tile is plain ground again.
func plotGoneEvent(at Point) Event {
	return Event{Msg: protocol.Plot{T: protocol.TypePlot, Plot: protocol.PlotView{X: at.X, Y: at.Y}}}
}

func (d *Domain) plotViews() []protocol.PlotView {
	out := make([]protocol.PlotView, 0, len(d.plots))
	for _, p := range d.plots {
		out = append(out, d.plotView(p))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

// morning ends a day: every plot moves on by one day, the weather of the new
// day is drawn and players get their energy back. fainted is true when the
// day ended because someone stayed up too late.
func (d *Domain) morning(now time.Time, fainted bool) []Event {
	d.mornings++
	d.dewPlots()
	d.growPlots()
	d.dryRacks()

	d.weather = protocol.WeatherSun
	if d.rng.Float64() < d.content.Time.RainChance {
		d.weather = protocol.WeatherRain
	}
	if d.weather == protocol.WeatherRain {
		for _, p := range d.plots {
			if p.State == protocol.PlotTilled || p.State == protocol.PlotCrop {
				p.Watered = true
			}
		}
	}

	events := []Event{d.clockEvent(), {Msg: protocol.Plots{T: protocol.TypePlots, Plots: d.plotViews()}}}
	events = append(events, d.dawn()...)
	events = append(events, d.petsMorning()...)
	for _, b := range d.sortedBuildings() {
		if b.Kind == BuildingRack {
			events = append(events, d.buildingEvent(b))
		}
	}
	for _, p := range d.sortedPlayers() {
		p.HP = d.maxHP(p)
		switch {
		case p.Online && fainted && !p.Sleeping:
			p.Energy = d.content.Energy.AfterFaint
			d.wakeAtHome(p)
			events = append(events, Event{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeFainted}})
		default:
			p.Energy = d.maxEnergy(p)
			if p.Online {
				events = append(events, Event{To: p.ID, Msg: protocol.Notice{T: protocol.TypeNotice, Code: protocol.NoticeMorning}})
			}
		}
		p.Sleeping = false
		p.readyAt = now
		p.turnAt = d.ct
		if p.Online {
			events = append(events,
				Event{Msg: protocol.Player{T: protocol.TypePlayer, Player: p.view()}},
				Event{To: p.ID, Msg: protocol.Stats{T: protocol.TypeStats, Stats: d.stats(p)}})
		}
	}
	events = append(events, d.topUpEdge()...)
	events = append(events, d.topUpFauna()...)
	return append(events, d.turnEvents()...)
}

// sower returns the player who sowed a plot, or nil.
func (d *Domain) sower(p *Plot) *Player {
	if p.Sower == "" {
		return nil
	}
	return d.players[p.Sower]
}

// greenThumb reports whether a plot was sown by a player with Main verte.
func (d *Domain) greenThumb(p *Plot) bool {
	s := d.sower(p)
	return s != nil && s.mod(ModGreenThumb) > 0
}

// dewPlots waters some dry seedlings of players with the Rosée talent.
func (d *Domain) dewPlots() {
	for _, p := range d.sortedPlots() {
		if p.State != protocol.PlotCrop || p.Watered {
			continue
		}
		if s := d.sower(p); s != nil && d.chance(s.mod(ModDew)) {
			p.Watered = true
		}
	}
}

// wakeAtHome puts a player in front of their bed.
func (d *Domain) wakeAtHome(p *Player) {
	spot := d.Map.WakeSpot()
	if d.Walkable(spot) {
		p.Pos = spot
	}
	p.Facing = protocol.Down
}

// growPlots applies one day to every plot. Plots are visited in a fixed
// order so that a seeded random source gives the same valley every time.
func (d *Domain) growPlots() {
	rules := d.content.Plots
	for _, p := range d.sortedPlots() {
		switch p.State {
		case protocol.PlotTilled:
			if p.Watered {
				p.Neglect = 0
			} else if p.Neglect++; p.Neglect >= rules.IdleDaysToWild {
				d.goWild(p)
			}
		case protocol.PlotCrop:
			def := d.crop(p)
			ripe := p.Growth >= def.Days
			green := d.greenThumb(p)
			if green && !ripe && !p.Watered {
				// Main verte: one dry day in two, it grows anyway; it never withers.
				if p.DryDays++; p.DryDays < 2 {
					break
				}
				p.DryDays = 0
				p.Watered = true
			}
			switch {
			case ripe:
				// Nobody harvests: it ends up dropping its seeds.
				if p.Neglect++; p.Neglect >= rules.RipeDaysToSeed {
					d.dropSeeds(p)
				}
			case p.Watered || def.Wild:
				p.Growth++
				p.Neglect = 0
				if p.Growth >= def.Days {
					p.Neglect = 0 // counts days spent ripe from now on
				}
			default:
				if p.Neglect++; p.Neglect >= rules.DryDaysToWither {
					p.State = protocol.PlotWithered
					p.Neglect = 0
				}
			}
		case protocol.PlotWithered:
			if p.Neglect++; p.Neglect >= rules.WitheredDaysToSeed {
				d.dropSeeds(p)
			}
		}
		p.Watered = false
	}
}

// dropSeeds handles a crop left in the ground: its seeds fall, and some
// sprout again as a rare wild variant. The others leave the plot to weeds.
func (d *Domain) dropSeeds(p *Plot) {
	def := d.crop(p)
	bonus := 0.0
	if s := d.sower(p); s != nil {
		bonus = float64(s.mod(ModMutation)) / 100
	}
	if def.Mutation != "" && d.rng.Float64() < def.MutationChance+bonus {
		p.State = protocol.PlotCrop
		p.Crop = def.Mutation
		p.Growth = 0
		p.Neglect = 0
		return
	}
	if def.Wild {
		// A wild crop reseeds itself.
		p.State = protocol.PlotCrop
		p.Growth = 0
		p.Neglect = 0
		return
	}
	d.goWild(p)
}

func (d *Domain) goWild(p *Plot) {
	p.State = protocol.PlotWild
	p.Sower = ""
	p.Crop = ""
	p.Growth = 0
	p.Neglect = 0
}

func (d *Domain) sortedPlots() []*Plot {
	out := make([]*Plot, 0, len(d.plots))
	for _, p := range d.plots {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At.Y != out[j].At.Y {
			return out[i].At.Y < out[j].At.Y
		}
		return out[i].At.X < out[j].At.X
	})
	return out
}
