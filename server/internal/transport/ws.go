// Package transport connects browsers to game instances over WebSocket.
package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/cdangeard/lostvalley/server/internal/game"
	"github.com/cdangeard/lostvalley/server/internal/protocol"
)

const (
	helloTimeout = 10 * time.Second
	writeTimeout = 5 * time.Second
	maxMessage   = 4 << 10
	maxNameRunes = 24
)

var playerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// Handler upgrades HTTP requests to WebSocket game connections.
type Handler struct {
	Instance *game.Instance
	// OriginPatterns lists extra browser origins allowed to connect, such as
	// "localhost:5173" for the Vite dev server. Same-origin is always allowed.
	OriginPatterns []string
	Log            *slog.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.OriginPatterns})
	if err != nil {
		h.Log.Warn("websocket accept", "err", err)
		return
	}
	conn.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	hello, err := readHello(ctx, conn)
	if err != nil {
		h.Log.Info("bad hello", "err", err)
		conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}

	client := game.NewClient(hello.PlayerID)
	h.Instance.Join(client, cleanName(hello.Name, hello.PlayerID))
	defer h.Instance.Leave(client)

	go func() {
		defer cancel()
		writeLoop(ctx, conn, client)
	}()

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) == -1 {
				h.Log.Debug("read", "player", client.PlayerID, "err", err)
			}
			return
		}
		msg, err := protocol.DecodeClient(raw)
		if err != nil {
			conn.Close(websocket.StatusUnsupportedData, "bad message")
			return
		}
		if _, again := msg.(*protocol.Hello); again {
			continue
		}
		h.Instance.Handle(client, msg)
	}
}

func readHello(ctx context.Context, conn *websocket.Conn) (*protocol.Hello, error) {
	ctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	_, raw, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	msg, err := protocol.DecodeClient(raw)
	if err != nil {
		return nil, err
	}
	hello, ok := msg.(*protocol.Hello)
	if !ok {
		return nil, errors.New("first message must be hello")
	}
	if hello.Version != protocol.Version {
		return nil, fmt.Errorf("protocol version %d not supported, server speaks %d", hello.Version, protocol.Version)
	}
	if !playerIDPattern.MatchString(hello.PlayerID) {
		return nil, errors.New("invalid player id")
	}
	return hello, nil
}

func writeLoop(ctx context.Context, conn *websocket.Conn, c *game.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.Done():
			conn.Close(websocket.StatusTryAgainLater, "too slow")
			return
		case raw := <-c.Send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(wctx, websocket.MessageText, raw)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// cleanName trims a display name and falls back to one derived from the ID.
// Character creation (phase 1+) will replace this.
func cleanName(name, id string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name))
	if utf8.RuneCountInString(name) > maxNameRunes {
		name = string([]rune(name)[:maxNameRunes])
	}
	if name == "" {
		name = "Voyageur " + strings.ToUpper(id[:4])
	}
	return name
}
