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
const Version = 4

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
	TypeHello     = "hello"
	TypeMove      = "move"
	TypeUse       = "use"
	TypeWait      = "wait"
	TypeBuild     = "build"
	TypeDismantle = "dismantle"
	TypeChestMove = "chest_move"
	TypeTalent    = "talent"
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

// Wait spends one combat tick doing nothing. Outside combat it does nothing.
type Wait struct {
	T   string `json:"t"`
	Seq int    `json:"seq"`
}

// Build places a building whose top-left corner is X, Y. The footprint must
// cover the tile the player faces, inside the domain.
type Build struct {
	T    string `json:"t"`
	Seq  int    `json:"seq"`
	Kind string `json:"kind"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

// Dismantle takes down the building the player faces and gives back its cost.
type Dismantle struct {
	T   string `json:"t"`
	Seq int    `json:"seq"`
}

// Chest operations.
const (
	ChestPut  = "deposer"
	ChestTake = "prendre"
)

// ChestMove moves Count of Item between the bag and the house chest. The
// player must face the chest. Count <= 0 moves the whole stack.
type ChestMove struct {
	T     string `json:"t"`
	Seq   int    `json:"seq"`
	Op    string `json:"op"`
	Item  string `json:"item"`
	Count int    `json:"count"`
}

// Talent operations.
const (
	TalentTake   = "prendre"
	TalentRefund = "retirer"
)

// Talent takes a node of the Constellation des Racines (it must be linked to
// one already taken, and cost a point), or gives one back (it costs seeds of
// oblivion, and the others must stay linked to the heart).
type Talent struct {
	T    string `json:"t"`
	Seq  int    `json:"seq"`
	Node string `json:"node"`
	Op   string `json:"op"`
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
	case TypeWait:
		msg = &Wait{}
	case TypeBuild:
		msg = &Build{}
	case TypeDismantle:
		msg = &Dismantle{}
	case TypeChestMove:
		msg = &ChestMove{}
	case TypeTalent:
		msg = &Talent{}
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
	TypeWelcome      = "welcome"
	TypePlayer       = "player"
	TypePlayerLeft   = "player_left"
	TypeResource     = "resource"
	TypeInventory    = "inventory"
	TypePlot         = "plot"
	TypePlots        = "plots"
	TypeClock        = "clock"
	TypeStats        = "stats"
	TypeNotice       = "notice"
	TypeCreature     = "creature"
	TypeCreatureGone = "creature_gone"
	TypeBuilding     = "building"
	TypeBuildingGone = "building_gone"
	TypeClaim        = "claim"
	TypeChest        = "chest"
	TypeTurn         = "turn"
	TypeHit          = "hit"
	TypeProgress     = "progress"
	TypeAck          = "ack"
	TypeError        = "error"
)

// PlayerView is what everyone sees of a player.
type PlayerView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Facing   string `json:"facing"`
	Sleeping bool   `json:"sleeping,omitempty"`
	HP       int    `json:"hp"`
}

// ResourceView is a resource node as clients draw it.
type ResourceView struct {
	ID       int    `json:"id"`
	Kind     string `json:"kind"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Hits     int    `json:"hits"`              // strikes left before it is depleted
	Depleted bool   `json:"depleted"`          // a stump, rubble or cut grass, waiting to regrow
	Removed  bool   `json:"removed,omitempty"` // uprooted for good: forget it
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

// StatsView is what a player sees of their own health, energy and watering can.
type StatsView struct {
	HP        int `json:"hp"`
	MaxHP     int `json:"maxHp"`
	Energy    int `json:"energy"`
	MaxEnergy int `json:"maxEnergy"`
	Water     int `json:"water"`
	WaterMax  int `json:"waterMax"`
}

// Tile is a tile coordinate.
type Tile struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// CreatureView is a creature as clients draw it. Warn lists the tiles its
// next blow will hit: they are shown in red so players can step away.
type CreatureView struct {
	ID       int    `json:"id"`
	Kind     string `json:"kind"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Facing   string `json:"facing"`
	HP       int    `json:"hp"`
	MaxHP    int    `json:"maxHp"`
	Armor    int    `json:"armor,omitempty"`
	Burrowed bool   `json:"burrowed,omitempty"`
	Stunned  bool   `json:"stunned,omitempty"`
	Wave     bool   `json:"wave,omitempty"` // came with the night wave, after the crops
	Warn     []Tile `json:"warn,omitempty"`
	// Taming. Peaceful creatures never attack; a submissive one has given in
	// and can be fed. Trust fills up to TrustMax, then the creature is tamed.
	Peaceful   bool   `json:"peaceful,omitempty"`
	Submissive bool   `json:"submissive,omitempty"`
	Trust      int    `json:"trust,omitempty"`
	TrustMax   int    `json:"trustMax,omitempty"`
	Owner      string `json:"owner,omitempty"` // tamed: the player it is bound to
	OwnerName  string `json:"ownerName,omitempty"`
	Mode       string `json:"mode,omitempty"`    // tamed: "suit" (follows its player) or "maison" (stays home)
	Resting    bool   `json:"resting,omitempty"` // tamed and worn out: home until morning
}

// Modes of a tamed creature.
const (
	ModeFollow = "suit"
	ModeHome   = "maison"
)

// ProgressView is a player's level and talents.
type ProgressView struct {
	Level   int      `json:"level"`
	XP      int      `json:"xp"`      // total experience
	LevelXP int      `json:"levelXp"` // total experience at the start of this level
	NextXP  int      `json:"nextXp"`  // total experience for the next level; 0 at the top
	Points  int      `json:"points"`  // talent points not spent yet
	Talents []string `json:"talents"` // nodes taken, besides the heart
}

// BuildingView is a building; X, Y is its top-left tile.
type BuildingView struct {
	ID    int    `json:"id"`
	Kind  string `json:"kind"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	W     int    `json:"w"`
	H     int    `json:"h"`
	HP    int    `json:"hp,omitempty"`    // fences
	Load  int    `json:"load,omitempty"`  // drying rack: crops drying tonight
	Ready int    `json:"ready,omitempty"` // drying rack: rations to collect
}

// TurnView tells a player whether they are "au pas" (step-by-step combat)
// and whether the world is waiting for them.
type TurnView struct {
	Engaged bool `json:"engaged"`
	Ready   bool `json:"ready"` // your move: the world waits for you
	Tick    int  `json:"tick"`
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
	Creatures []CreatureView `json:"creatures"`
	Buildings []BuildingView `json:"buildings"`
	Claim     string         `json:"claim"` // one char per tile, row by row: '1' inside the domain
	Turn      TurnView       `json:"turn"`
	Progress  ProgressView   `json:"progress"`
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
	NoticeMorning  = "morning"   // a new day after sleeping
	NoticeFainted  = "fainted"   // stayed up too late: woke at home with less energy
	NoticeWaiting  = "waiting"   // in bed, waiting for the others to go to bed too
	NoticeKO       = "ko"        // fell in combat: woke at home, the bag is lost
	NoticeWave     = "wave"      // pests are coming for the fields
	NoticeWaveOver = "wave_over" // the night's pests ran off at dawn
	NoticeUprooted = "uprooted"  // a stump came out and the domain grew
	NoticeWorkshop = "workshop"  // facing the workshop: open the build menu
	NoticeDried    = "dried"     // rations collected from the drying rack
	NoticeDrying   = "drying"    // crops hung on the drying rack
	NoticeLevelUp  = "level_up"  // Arg: the new level; a talent point to spend
	NoticeFed      = "fed"       // Arg: the species; it trusts you a little more
	NoticeFull     = "not_hungry"
	NoticeTamed    = "tamed"         // Arg: the species
	NoticeHabitat  = "needs_habitat" // Arg: the building it needs, free, in the domain
	NoticeSubdued  = "submissive"    // Arg: the species; it gives in and can be fed
	NoticeFollow   = "follow"        // Arg: the species
	NoticeStay     = "stay"          // Arg: the species
	NoticeTooMany  = "too_many_followers"
	NoticeTired    = "pet_tired" // Arg: the species; worn out, back home until morning
	NoticeProduce  = "produce"   // collected what a tamed creature made
	NoticeScared   = "scared"    // Arg: the species; you hit it and it no longer trusts you
)

// Notice is a short event for one player's screen. Arg completes some codes.
type Notice struct {
	T    string `json:"t"`
	Code string `json:"code"`
	Arg  string `json:"arg,omitempty"`
}

// Progress is sent to a player when their experience, level or talents change.
type Progress struct {
	T        string       `json:"t"`
	Progress ProgressView `json:"progress"`
}

// Creature is sent when a creature appears, moves, warns, is hit or changes.
type Creature struct {
	T        string       `json:"t"`
	Creature CreatureView `json:"creature"`
}

// Ways a creature leaves.
const (
	GoneKilled = "mort"
	GoneFled   = "fuite"
)

// CreatureGone is sent when a creature dies or runs off.
type CreatureGone struct {
	T   string `json:"t"`
	ID  int    `json:"id"`
	How string `json:"how"`
}

// Building is sent when a building is placed or changes.
type Building struct {
	T        string       `json:"t"`
	Building BuildingView `json:"building"`
}

// BuildingGone is sent when a building is taken down or broken.
type BuildingGone struct {
	T  string `json:"t"`
	ID int    `json:"id"`
}

// Claim is sent when the domain grows.
type Claim struct {
	T     string `json:"t"`
	Claim string `json:"claim"`
}

// Chest is sent to a player who opens the house chest or moves things in it.
type Chest struct {
	T     string         `json:"t"`
	Items map[string]int `json:"items"`
	Open  bool           `json:"open,omitempty"` // true when the player just opened it
}

// Turn is sent to a player when their combat state changes.
type Turn struct {
	T    string   `json:"t"`
	Turn TurnView `json:"turn"`
}

// Hit targets.
const (
	HitPlayer   = "player"
	HitCreature = "creature"
	HitBuilding = "building"
)

// Hit is sent when something takes damage, so clients can show it.
type Hit struct {
	T      string `json:"t"`
	Target string `json:"target"`
	ID     string `json:"id"` // player ID, or creature/building ID as text
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Amount int    `json:"amount"` // 0: blocked by armour
	Dodged bool   `json:"dodged,omitempty"`
}

// Ack answers a Move or Use on the connection that sent it. Reason is a
// short code when OK is false (blocked, too_fast, nothing_there, wrong_tool,
// exhausted, empty_can, not_your_turn, cannot_build…).
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
