package game

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/protocol"
	"github.com/cdangeard/lostvalley/server/internal/store"
	"github.com/cdangeard/lostvalley/server/internal/world"
)

func testConfig(t *testing.T, st store.Store) Config {
	t.Helper()
	m, err := world.ParseMap([]byte(`{"width":3,"height":1,"tilewidth":16,"tileheight":16,"layers":[
	  {"name":"sol","type":"tilelayer","data":[1,1,1]},
	  {"name":"reperes","type":"objectgroup","objects":[{"name":"apparition","x":0,"y":0}]}],
	  "tilesets":[{"firstgid":1,"tiles":[{"id":0,"properties":[{"name":"cultivable","type":"bool","value":true}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := world.LoadContent("../../../assets/data/contenu.json")
	if err != nil {
		t.Fatal(err)
	}
	return Config{DomainID: "test", MapName: "test", Map: m, Content: c, Store: st,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// waitFor reads c's messages until one of type typ arrives.
func waitFor(t *testing.T, c *Client, typ string, out any) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case raw := <-c.Send:
			var head struct{ T string }
			json.Unmarshal(raw, &head)
			if head.T == typ {
				if out != nil {
					json.Unmarshal(raw, out)
				}
				return
			}
		case <-timeout:
			t.Fatalf("no %q message", typ)
		}
	}
}

// Phase 1 exit criterion, in miniature: what you farm is still there after
// you leave and the server restarts.
func TestFarmSurvivesLeavingAndRestart(t *testing.T) {
	st := store.NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	in, err := Open(ctx, testConfig(t, st))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { in.Run(ctx, 20*time.Millisecond); close(done) }()

	c := NewClient("player-aaaa")
	in.Join(c, "Ana")
	waitFor(t, c, protocol.TypeWelcome, nil)
	in.Handle(c, &protocol.Move{T: protocol.TypeMove, Seq: 1, Dir: protocol.Left}) // face left: the map edge
	waitFor(t, c, protocol.TypeAck, nil)
	in.Handle(c, &protocol.Move{T: protocol.TypeMove, Seq: 2, Dir: protocol.Right})
	waitFor(t, c, protocol.TypeAck, nil)
	time.Sleep(world.MoveCooldown)
	// Now on tile 1 facing right: till tile 2 and sow.
	for i, item := range []string{"houe", "graine-navet"} {
		in.Handle(c, &protocol.Use{T: protocol.TypeUse, Seq: 3 + i, Item: item})
		var ack protocol.Ack
		waitFor(t, c, protocol.TypeAck, &ack)
		if !ack.OK {
			t.Fatalf("%s: %+v", item, ack)
		}
		time.Sleep(world.UseCooldown)
	}
	in.Leave(c) // saves
	cancel()
	<-done

	snap, err := st.Load(context.Background(), "test")
	if err != nil || snap == nil {
		t.Fatalf("nothing saved: %v", err)
	}
	if len(snap.Domain.Plots) != 1 || snap.Domain.Plots[0].Crop != "navet" {
		t.Fatalf("saved plots: %+v", snap.Domain.Plots)
	}

	in2, err := Open(context.Background(), testConfig(t, st))
	if err != nil {
		t.Fatal(err)
	}
	w := in2.domain.Welcome("player-aaaa", "test")
	if len(w.Plots) != 1 || w.Plots[0].X != 2 || w.Inventory["graine-navet"] != 11 || len(w.Tools) == 0 {
		t.Fatalf("after restart: plots %+v bag %v tools %v", w.Plots, w.Inventory, w.Tools)
	}
}
