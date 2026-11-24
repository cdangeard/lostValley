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
const Version = 1

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
	TypeHello  = "hello"
	TypeMove   = "move"
	TypeGather = "gather"
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
// the player to face it.
type Move struct {
	T   string `json:"t"`
	Seq int    `json:"seq"`
	Dir string `json:"dir"`
}

// Gather strikes the resource on the tile the player faces.
type Gather struct {
	T   string `json:"t"`
	Seq int    `json:"seq"`
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
	case TypeGather:
		msg = &Gather{}
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
	TypeAck        = "ack"
	TypeError      = "error"
)

// PlayerView is what everyone sees of a player.
type PlayerView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Facing string `json:"facing"`
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

// Welcome answers Hello with the whole state of the domain.
type Welcome struct {
	T         string         `json:"t"`
	Version   int            `json:"version"`
	You       string         `json:"you"`
	Map       string         `json:"map"`
	Players   []PlayerView   `json:"players"`
	Resources []ResourceView `json:"resources"`
	Inventory map[string]int `json:"inventory"`
}

// Player is sent when a player appears, moves or turns.
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

// Ack answers a Move or Gather on the connection that sent it. Reason is a
// short code when OK is false (blocked, too_fast, nothing_there…).
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
