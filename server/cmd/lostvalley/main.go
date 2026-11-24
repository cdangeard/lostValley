// Command lostvalley runs the authoritative game server.
//
//	go run ./cmd/lostvalley -db postgres://localhost/lostvalley
//
// Without -db (or LV_DATABASE_URL) the valley lives in memory only.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cdangeard/lostvalley/server/internal/game"
	"github.com/cdangeard/lostvalley/server/internal/store"
	"github.com/cdangeard/lostvalley/server/internal/transport"
	"github.com/cdangeard/lostvalley/server/internal/world"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	mapPath := flag.String("map", "../assets/maps/valley.json", "Tiled JSON map of the valley")
	contentPath := flag.String("content", "../assets/data/contenu.json", "game content: crops, tools, pace of time")
	dbURL := flag.String("db", os.Getenv("LV_DATABASE_URL"), "PostgreSQL URL; empty keeps everything in memory")
	static := flag.String("static", "", "optional directory of the built client (client/dist) to serve at /")
	origins := flag.String("origins", "localhost:*,127.0.0.1:*", "comma-separated extra origins allowed to open the WebSocket")
	debug := flag.Bool("debug", false, "verbose logs")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	m, err := world.LoadMap(*mapPath)
	if err != nil {
		log.Error("load map", "path", *mapPath, "err", err)
		os.Exit(1)
	}
	mapName := strings.TrimSuffix(filepath.Base(*mapPath), filepath.Ext(*mapPath))
	log.Info("map loaded", "name", mapName, "size", [2]int{m.Width, m.Height}, "resources", len(m.Resources))

	content, err := world.LoadContent(*contentPath)
	if err != nil {
		log.Error("load content", "path", *contentPath, "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store
	if *dbURL == "" {
		log.Warn("no database: the valley lives in memory and is lost on restart (set -db or LV_DATABASE_URL)")
		st = store.NewMemory()
	} else {
		pg, err := store.OpenPostgres(ctx, *dbURL)
		if err != nil {
			log.Error("open database", "err", err)
			os.Exit(1)
		}
		st = pg
	}
	defer st.Close()

	// One valley for now; per-player domains come with accounts in phase 4.
	inst, err := game.Open(ctx, game.Config{DomainID: mapName, MapName: mapName, Map: m, Content: content, Store: st, Log: log})
	if err != nil {
		log.Error("open domain", "err", err)
		os.Exit(1)
	}
	stopped := make(chan struct{})
	go func() {
		inst.Run(ctx, 250*time.Millisecond)
		close(stopped)
	}()

	mux := http.NewServeMux()
	mux.Handle("/ws", &transport.Handler{Instance: inst, OriginPatterns: splitList(*origins), Log: log})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	if *static != "" {
		mux.Handle("/", http.FileServer(http.Dir(*static)))
	}

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Info("listening", "addr", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		stop()
	}
	<-stopped // the instance saves on its way out
	log.Info("valley saved, bye")
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
