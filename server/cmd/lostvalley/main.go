// Command lostvalley runs the authoritative game server.
//
//	go run ./cmd/lostvalley -map ../assets/maps/valley.json
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
	"github.com/cdangeard/lostvalley/server/internal/transport"
	"github.com/cdangeard/lostvalley/server/internal/world"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	mapPath := flag.String("map", "../assets/maps/valley.json", "Tiled JSON map of the valley")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	inst := game.NewInstance(mapName, m, log)
	go inst.Run(ctx, 500*time.Millisecond)

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
		os.Exit(1)
	}
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
