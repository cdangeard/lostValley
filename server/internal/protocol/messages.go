// Package protocol defines the JSON messages exchanged over the WebSocket.
//
// Every message is a JSON object with a "t" field naming its type. The
// TypeScript mirror lives in client/src/net/protocol.ts: change both together.
package protocol

import (
	"encoding/json"
	"fmt"
)

// Version is bumped whenever a message changes shape.
const Version = 2

// Directions a player can face or step in.
const (
	Up    = "up"
	Down  = "down"
	Left  = "left"
	Right = "right"
)

// ValidDir reports whether d is one of the four directions.
func ValidDir(d string) bool {
	return d == Up || d == Down || d == Left || d == Right
}

// ---------- client → server ----------

const (
	TypeHello = "hello"
	TypeMove  = "move"
	TypeUse   = "use"
)

// Hello is the first message on a connection. PlayerID is a random token the
// browser keeps; it stands in for real accounts until phase 4.
type Hello struct {
	T        string `json:"t"`
	Version  int    `json:"version"`
	PlayerID string `json:"playerId"`
	Name     string `json:"name,omitempty"`
}

// Move asks to step one tile in Dir. Stepping into an obstacle only turns
// the player to face it. Moving also gets a sleeping player out of bed.
type Move struct {
	T   string `json:"t"`
	Seq int    `json:"seq"`
	Dir string `json:"dir"`
}

// Use acts on the tile the player faces with Item: a tool the player carries
// ("houe", "hache"…), a seed from the bag ("graine-navet"…), or "" for bare
// hands. Whatever the item, facing a ripe crop harvests it and facing the
// bed goes to sleep.
type Use struct {
	T    string `json:"t"`
	Seq  int    `json:"seq"`
	Item string `json:"item"`
}

// DecodeClient parses a client message into its concrete type.
func DecodeClient(raw []byte) (any, error) {
	var head struct {
		T string `json:"t"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	var msg any
	switch head.T {
	case TypeHello:
		msg = &Hello{}
	case TypeMove:
		msg = &Move{}
	case TypeUse:
		msg = &Use{}
	default:
		return nil, fmt.Errorf("decode message: unknown type %q", head.T)
	}
	if err := json.Unmarshal(raw, msg); err != nil {
		return nil, fmt.Errorf("decode %s: %w", head.T, err)
	}
	return msg, nil
}

// ---------- server → client ----------

const (
	TypeWelcome    = "welcome"
	TypePlayer     = "player"
	TypePlayerLeft = "player_left"
	TypeResource   = "resource"
	TypeInventory  = "inventory"
	TypePlot       = "plot"
	TypePlots      = "plots"
	TypeClock      = "clock"
	TypeStats      = "stats"
	TypeNotice     = "notice"
	TypeAck        = "ack"
	TypeError      = "error"
)

// PlayerView is what everyone sees of a player.
type PlayerView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Facing   string `json:"facing"`
	Sleeping bool   `json:"sleeping,omitempty"`
}

// ResourceView is a resource node as clients draw it.
type ResourceView struct {
	ID       int    `json:"id"`
	Kind     string `json:"kind"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Hits     int    `json:"hits"`     // strikes left before it is depleted
	Depleted bool   `json:"depleted"` // a stump, rubble or cut grass, waiting to regrow
}

// Plot states.
const (
	PlotTilled   = "labour"  // bare tilled soil
	PlotCrop     = "culture" // something growing or ripe
	PlotWithered = "fane"    // a dead crop; the scythe clears it
	PlotWild     = "friche"  // weeds; the scythe clears them
)

// PlotView is one farmed tile.
type PlotView struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	State   string `json:"state"`
	Crop    string `json:"crop,omitempty"`
	Growth  int    `json:"growth,omitempty"` // watered days so far
	Days    int    `json:"days,omitempty"`   // days to ripen; ripe when Growth >= Days
	Watered bool   `json:"watered,omitempty"`
}

// Weather of the day.
const (
	WeatherSun  = "soleil"
	WeatherRain = "pluie"
)

// ClockView is the valley's time. Clients run the clock themselves between
// two updates, at MinuteMs real milliseconds per game minute.
type ClockView struct {
	Day       int    `json:"day"`    // 1 on the first morning, counts up forever
	Minute    int    `json:"minute"` // minutes since midnight, 0..1439
	Year      int    `json:"year"`
	Season    string `json:"season"`
	SeasonDay int    `json:"seasonDay"`
	Weather   string `json:"weather"`
	MinuteMs  int    `json:"minuteMs"`
}

// StatsView is what a player sees of their own energy and watering can.
type StatsView struct {
	Energy    int `json:"energy"`
	MaxEnergy int `json:"maxEnergy"`
	Water     int `json:"water"`
	WaterMax  int `json:"waterMax"`
}

// Welcome answers Hello with the whole state of the domain.
type Welcome struct {
	T         string         `json:"t"`
	Version   int            `json:"version"`
	You       string         `json:"you"`
	Map       string         `json:"map"`
	Players   []PlayerView   `json:"players"`
	Resources []ResourceView `json:"resources"`
	Plots     []PlotView     `json:"plots"`
	Clock     ClockView      `json:"clock"`
	Tools     []string       `json:"tools"`     // equipped: kept on death
	Inventory map[string]int `json:"inventory"` // the bag: lost on death
	Stats     StatsView      `json:"stats"`
}

// Player is sent when a player appears, moves, turns, sleeps or wakes.
type Player struct {
	T      string     `json:"t"`
	Player PlayerView `json:"player"`
}

// PlayerLeft is sent when a player's last connection closes.
type PlayerLeft struct {
	T  string `json:"t"`
	ID string `json:"id"`
}

// Resource is sent when a resource is struck, depleted or regrows.
type Resource struct {
	T        string       `json:"t"`
	Resource ResourceView `json:"resource"`
}

// Inventory is sent to a player when their bag changes.
type Inventory struct {
	T     string         `json:"t"`
	Items map[string]int `json:"items"`
}

// Plot is sent when one plot changes. State "" means the plot is gone and
// the tile is plain ground again.
type Plot struct {
	T    string   `json:"t"`
	Plot PlotView `json:"plot"`
}

// Plots replaces every plot at once, sent each morning.
type Plots struct {
	T     string     `json:"t"`
	Plots []PlotView `json:"plots"`
}

// Clock is sent each morning and regularly in between to keep clients in step.
type Clock struct {
	T     string    `json:"t"`
	Clock ClockView `json:"clock"`
}

// Stats is sent to a player when their energy or water changes.
type Stats struct {
	T     string    `json:"t"`
	Stats StatsView `json:"stats"`
}

// Notices tell a player something happened to them.
const (
	NoticeMorning = "morning" // a new day after sleeping
	NoticeFainted = "fainted" // stayed up too late: woke at home with less energy
	NoticeWaiting = "waiting" // in bed, waiting for the others to go to bed too
)

// Notice is a short event for one player's screen.
type Notice struct {
	T    string `json:"t"`
	Code string `json:"code"`
}

// Ack answers a Move or Use on the connection that sent it. Reason is a
// short code when OK is false (blocked, too_fast, nothing_there, wrong_tool,
// exhausted, empty_can…).
type Ack struct {
	T      string `json:"t"`
	Seq    int    `json:"seq"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// Error reports a protocol problem; the server usually closes afterwards.
type Error struct {
	T       string `json:"t"`
	Message string `json:"message"`
}
