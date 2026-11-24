package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/world"
)

func sample() *world.Snapshot {
	return &world.Snapshot{
		SavedAt: time.Date(2026, 11, 24, 18, 30, 0, 0, time.UTC),
		Domain: world.DomainState{
			Version: world.SnapshotVersion, Minutes: 2000.5, Weather: "pluie",
			Plots:     []world.PlotState{{X: 3, Y: 4, State: "culture", Crop: "navet", Growth: 2, Watered: true}},
			Resources: []world.ResourceState{{ID: 7, Kind: "arbre", X: 1, Y: 1, Hits: 0, Depleted: true, RegrowAt: time.Date(2026, 11, 24, 18, 33, 0, 0, time.UTC)}},
			Buildings: []world.BuildingState{{ID: 2, Kind: "cloture", X: 4, Y: 4, HP: 3, Owner: "ana-123456"}},
			Claim:     "0110",
			Chest:     map[string]int{"pierre": 9},
			WaveDay:   3,
		},
		Players: []world.PlayerState{
			{ID: "ana-123456", Name: "Ana", X: 5, Y: 6, Facing: "up", Tools: []string{"houe"}, Bag: map[string]int{"navet": 3}, Energy: 42, HP: 6, Water: 7},
			{ID: "bob-123456", Name: "Bob", X: 1, Y: 2, Facing: "down", Tools: []string{}, Bag: map[string]int{}, Energy: 100, Water: 20},
		},
	}
}

func roundTrip(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	if got, err := s.Load(ctx, "vallee-test"); err != nil || got != nil {
		t.Fatalf("empty store: %+v, %v", got, err)
	}
	want := sample()
	if err := s.Save(ctx, "vallee-test", want); err != nil {
		t.Fatal(err)
	}
	// Saving again updates in place.
	want.Players[0].Energy = 41
	if err := s.Save(ctx, "vallee-test", want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx, "vallee-test")
	if err != nil {
		t.Fatal(err)
	}
	if !got.SavedAt.Equal(want.SavedAt) || got.Domain.Minutes != want.Domain.Minutes || got.Domain.Weather != "pluie" {
		t.Fatalf("domain: %+v", got)
	}
	if len(got.Domain.Plots) != 1 || got.Domain.Plots[0] != want.Domain.Plots[0] {
		t.Fatalf("plots: %+v", got.Domain.Plots)
	}
	if len(got.Domain.Resources) != 1 || !got.Domain.Resources[0].RegrowAt.Equal(want.Domain.Resources[0].RegrowAt) {
		t.Fatalf("resources: %+v", got.Domain.Resources)
	}
	d := got.Domain
	if len(d.Buildings) != 1 || d.Buildings[0] != want.Domain.Buildings[0] || d.Claim != "0110" || d.Chest["pierre"] != 9 || d.WaveDay != 3 {
		t.Fatalf("phase 2 state: %+v", d)
	}
	if len(got.Players) != 2 || got.Players[0].HP != 6 || got.Players[0].Energy != 41 || got.Players[0].Bag["navet"] != 3 || got.Players[1].Name != "Bob" {
		t.Fatalf("players: %+v", got.Players)
	}
}

func TestMemory(t *testing.T) { roundTrip(t, NewMemory()) }

// TestPostgres runs against LV_TEST_DATABASE_URL, a throwaway database.
func TestPostgres(t *testing.T) {
	url := os.Getenv("LV_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LV_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pg, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if _, err := pg.pool.Exec(ctx, `DELETE FROM players WHERE domain_id = 'vallee-test'; DELETE FROM domains WHERE id = 'vallee-test'`); err != nil {
		t.Fatal(err)
	}
	roundTrip(t, pg)
}
