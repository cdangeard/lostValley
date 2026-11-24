// Package store saves and loads domains. PostgreSQL is the real store; the
// in-memory one serves tests and quick local runs without a database.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cdangeard/lostvalley/server/internal/world"
)

// Store keeps domain snapshots.
type Store interface {
	// Load returns the last snapshot of a domain, or nil if it was never saved.
	Load(ctx context.Context, domainID string) (*world.Snapshot, error)
	// Save writes a snapshot of a domain and its players.
	Save(ctx context.Context, domainID string, s *world.Snapshot) error
	Close()
}

// ---------- memory ----------

// Memory keeps snapshots in memory: nothing survives a restart.
type Memory struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory { return &Memory{data: map[string][]byte{}} }

func (m *Memory) Load(_ context.Context, domainID string) (*world.Snapshot, error) {
	m.mu.Lock()
	raw, ok := m.data[domainID]
	m.mu.Unlock()
	if !ok {
		return nil, nil
	}
	var s world.Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (m *Memory) Save(_ context.Context, domainID string, s *world.Snapshot) error {
	raw, err := json.Marshal(s) // a deep copy, like a real store
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.data[domainID] = raw
	m.mu.Unlock()
	return nil
}

func (m *Memory) Close() {}

// ---------- PostgreSQL ----------

// The valley is one JSONB document; players live in their own table because
// they will travel between valleys (visits, residents) in phase 4.
const schema = `
CREATE TABLE IF NOT EXISTS domains (
	id       text PRIMARY KEY,
	state    jsonb NOT NULL,
	saved_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS players (
	id        text PRIMARY KEY,
	domain_id text NOT NULL REFERENCES domains(id),
	name      text NOT NULL,
	state     jsonb NOT NULL,
	saved_at  timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS players_domain_id ON players (domain_id);
`

// Postgres stores snapshots in PostgreSQL.
type Postgres struct {
	pool *pgxpool.Pool
}

// OpenPostgres connects and creates the tables if needed.
func OpenPostgres(ctx context.Context, url string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Load(ctx context.Context, domainID string) (*world.Snapshot, error) {
	s := &world.Snapshot{}
	err := p.pool.QueryRow(ctx, `SELECT state, saved_at FROM domains WHERE id = $1`, domainID).
		Scan(&s.Domain, &s.SavedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load domain %s: %w", domainID, err)
	}
	rows, err := p.pool.Query(ctx, `SELECT state FROM players WHERE domain_id = $1 ORDER BY id`, domainID)
	if err != nil {
		return nil, fmt.Errorf("load players of %s: %w", domainID, err)
	}
	s.Players, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (world.PlayerState, error) {
		var ps world.PlayerState
		err := row.Scan(&ps)
		return ps, err
	})
	if err != nil {
		return nil, fmt.Errorf("load players of %s: %w", domainID, err)
	}
	return s, nil
}

func (p *Postgres) Save(ctx context.Context, domainID string, s *world.Snapshot) error {
	return pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO domains (id, state, saved_at) VALUES ($1, $2, $3)
			ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, saved_at = EXCLUDED.saved_at`,
			domainID, s.Domain, s.SavedAt)
		if err != nil {
			return fmt.Errorf("save domain %s: %w", domainID, err)
		}
		batch := &pgx.Batch{}
		for _, ps := range s.Players {
			batch.Queue(`
				INSERT INTO players (id, domain_id, name, state, saved_at) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (id) DO UPDATE SET domain_id = EXCLUDED.domain_id, name = EXCLUDED.name,
					state = EXCLUDED.state, saved_at = EXCLUDED.saved_at`,
				ps.ID, domainID, ps.Name, ps, s.SavedAt)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return fmt.Errorf("save players of %s: %w", domainID, err)
		}
		return nil
	})
}

func (p *Postgres) Close() { p.pool.Close() }
