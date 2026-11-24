package transport

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/cdangeard/lostvalley/server/internal/game"
	"github.com/cdangeard/lostvalley/server/internal/protocol"
	"github.com/cdangeard/lostvalley/server/internal/store"
	"github.com/cdangeard/lostvalley/server/internal/world"
)

func startServer(t *testing.T) string {
	t.Helper()
	m, err := world.ParseMap([]byte(`{"width":3,"height":1,"tilewidth":16,"tileheight":16,"layers":[
	  {"name":"sol","type":"tilelayer","data":[1,1,1]},
	  {"name":"reperes","type":"objectgroup","objects":[{"name":"apparition","x":0,"y":0}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	content, err := world.LoadContent("../../../assets/data/contenu.json")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := game.Open(ctx, game.Config{DomainID: "test", MapName: "test", Map: m, Content: content, Store: store.NewMemory(), Log: log})
	if err != nil {
		t.Fatal(err)
	}
	go inst.Run(ctx, 50*time.Millisecond)
	srv := httptest.NewServer(&Handler{Instance: inst, Log: log})
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

type testConn struct {
	t    *testing.T
	conn *websocket.Conn
}

func dial(t *testing.T, url, playerID string) *testConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	c := &testConn{t, conn}
	c.send(protocol.Hello{T: protocol.TypeHello, Version: protocol.Version, PlayerID: playerID})
	return c
}

func (c *testConn) send(v any) {
	c.t.Helper()
	raw, _ := json.Marshal(v)
	if err := c.conn.Write(context.Background(), websocket.MessageText, raw); err != nil {
		c.t.Fatal(err)
	}
}

// expect reads until a message of type typ arrives and decodes it into out.
func (c *testConn) expect(typ string, out any) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		_, raw, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("waiting for %q: %v", typ, err)
		}
		var head struct{ T string }
		json.Unmarshal(raw, &head)
		if head.T == typ {
			if err := json.Unmarshal(raw, out); err != nil {
				c.t.Fatal(err)
			}
			return
		}
	}
}

// Phase 0 exit criterion: two tabs see the same character move.
func TestTwoTabsSeeTheSameCharacterMove(t *testing.T) {
	url := startServer(t)
	tab1 := dial(t, url, "player-aaaa")
	var w protocol.Welcome
	tab1.expect(protocol.TypeWelcome, &w)
	if w.You != "player-aaaa" || len(w.Resources) != 0 {
		t.Fatalf("welcome = %+v", w)
	}

	tab2 := dial(t, url, "player-aaaa")
	tab2.expect(protocol.TypeWelcome, &w)
	other := dial(t, url, "player-bbbb")
	other.expect(protocol.TypeWelcome, &w)
	if len(w.Players) != 2 {
		t.Fatalf("other player should see 2 players, got %+v", w.Players)
	}

	tab1.send(protocol.Move{T: protocol.TypeMove, Seq: 1, Dir: protocol.Right})
	var ack protocol.Ack
	tab1.expect(protocol.TypeAck, &ack)
	if !ack.OK || ack.Seq != 1 {
		t.Fatalf("ack = %+v", ack)
	}
	for _, c := range []*testConn{tab2, other} {
		for {
			var p protocol.Player
			c.expect(protocol.TypePlayer, &p)
			if p.Player.ID == "player-aaaa" && p.Player.X == 1 {
				break
			}
		}
	}

	// Closing one tab keeps the player online; closing both takes it away.
	tab1.conn.Close(websocket.StatusNormalClosure, "")
	tab2.conn.Close(websocket.StatusNormalClosure, "")
	var left protocol.PlayerLeft
	other.expect(protocol.TypePlayerLeft, &left)
	if left.ID != "player-aaaa" {
		t.Fatalf("left = %+v", left)
	}
}

func TestRejectsBadHello(t *testing.T) {
	url := startServer(t)
	c := dial(t, url, "short")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := c.conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("expected a policy violation close, got %v", err)
	}
}

func TestCleanName(t *testing.T) {
	if got := cleanName("  Ana\n ", "abcdefgh"); got != "Ana" {
		t.Errorf("got %q", got)
	}
	if got := cleanName("", "abcdefgh"); got != "Voyageur ABCD" {
		t.Errorf("got %q", got)
	}
	if got := cleanName(strings.Repeat("é", 40), "abcdefgh"); len([]rune(got)) != maxNameRunes {
		t.Errorf("got %q", got)
	}
}
