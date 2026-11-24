package world

import (
	"math"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

const minutesPerDay = 24 * 60

// The valley's clock counts game minutes since midnight before day 1.
// It runs at one pace while someone is in the valley and at a slower,
// configurable pace while it is empty, so time passes offline too.
//
// A day ends one of three ways, and each runs the same morning (crops grow,
// wither, mutate; the weather is drawn; everyone's energy comes back):
//   - every player in the valley is in bed: the clock jumps to the next morning;
//   - someone is still up at the faint hour: they faint, the clock jumps;
//   - the valley is empty: the clock simply runs through the night.

// wakeAt returns the minute of the first wake-up strictly after m.
func (d *Domain) wakeAt(m float64) float64 {
	w := float64(d.content.Time.WakeHour * 60)
	k := math.Floor((m-w)/minutesPerDay) + 1
	return k*minutesPerDay + w
}

// faintAt returns the minute of the first faint hour strictly after m.
func (d *Domain) faintAt(m float64) float64 {
	f := float64(d.content.Time.FaintHour * 60)
	k := math.Floor((m-f)/minutesPerDay) + 1
	return k*minutesPerDay + f
}

// rate is game minutes per real second right now.
func (d *Domain) rate() float64 {
	secs := d.content.Time.OfflineDaySeconds
	if d.onlineCount() > 0 {
		secs = d.content.Time.OnlineDaySeconds
	}
	return minutesPerDay / secs
}

// advanceClock moves the clock to now and runs every morning on the way.
// It returns the events those mornings produced.
func (d *Domain) advanceClock(now time.Time) []Event {
	if d.lastTick.IsZero() || !now.After(d.lastTick) {
		d.lastTick = now
		return nil
	}
	elapsed := now.Sub(d.lastTick).Seconds()
	d.lastTick = now
	target := d.minutes + elapsed*d.rate()

	// A very long absence: skip the oldest days without simulating them.
	if max := float64(d.content.Time.MaxCatchUpDays * minutesPerDay); target-d.minutes > max {
		d.minutes = target - max
	}

	var events []Event
	for {
		wake := d.wakeAt(d.minutes)
		if d.anyoneAwake() {
			if faint := d.faintAt(d.minutes); faint <= target && faint < wake {
				// Whatever real time is left keeps running after the jump.
				target = wake + (target - faint)
				d.minutes = wake
				events = append(events, d.morning(now, true)...)
				continue
			}
		}
		if wake > target {
			break
		}
		d.minutes = wake
		events = append(events, d.morning(now, false)...)
	}
	d.minutes = target
	return events
}

// sleepThrough jumps to the next morning because everyone is in bed.
func (d *Domain) sleepThrough(now time.Time) []Event {
	d.minutes = d.wakeAt(d.minutes)
	return d.morning(now, false)
}

func (d *Domain) anyoneAwake() bool {
	for _, p := range d.players {
		if p.Online && !p.Sleeping {
			return true
		}
	}
	return false
}

func (d *Domain) everyoneAsleep() bool {
	return d.onlineCount() > 0 && !d.anyoneAwake()
}

func (d *Domain) onlineCount() int {
	n := 0
	for _, p := range d.players {
		if p.Online {
			n++
		}
	}
	return n
}

// Day is the current day number, 1 on the first morning.
func (d *Domain) Day() int {
	// Days change at the wake hour, not at midnight.
	w := float64(d.content.Time.WakeHour * 60)
	return int(math.Floor((d.minutes-w)/minutesPerDay)) + 1
}

// MinuteOfDay is minutes since midnight.
func (d *Domain) MinuteOfDay() int {
	return int(math.Mod(d.minutes, minutesPerDay))
}

// ClockView describes the clock for clients.
func (d *Domain) ClockView() protocol.ClockView {
	t := d.content.Time
	day := d.Day()
	idx := day - 1
	if idx < 0 {
		idx = 0
	}
	perYear := t.DaysPerSeason * len(t.Seasons)
	return protocol.ClockView{
		Day:       day,
		Minute:    d.MinuteOfDay(),
		Year:      idx/perYear + 1,
		Season:    t.Seasons[(idx%perYear)/t.DaysPerSeason],
		SeasonDay: idx%t.DaysPerSeason + 1,
		Weather:   d.weather,
		MinuteMs:  int(d.content.onlineMinute() / time.Millisecond),
	}
}

func (d *Domain) clockEvent() Event {
	return Event{Msg: protocol.Clock{T: protocol.TypeClock, Clock: d.ClockView()}}
}
