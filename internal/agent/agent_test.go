package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	pngenc "image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/microbot"
)

type fakeGame struct {
	mu      sync.Mutex
	running bool
	worlds  []int
	// requests records the Agent Server requests of the plugin commands.
	requests []string
}

// JSON answers like the Agent Server for the paths the plugin commands use,
// and records each request as "METHOD path?query body".
func (g *fakeGame) JSON(_ context.Context, method, path string, query url.Values, body any, _ time.Duration) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	req := method + " " + path
	if len(query) > 0 {
		req += "?" + query.Encode()
	}
	if body != nil {
		b, _ := json.Marshal(body)
		req += " " + string(b)
	}
	g.requests = append(g.requests, req)
	switch path {
	case "/scripts":
		return json.RawMessage(`{"count":1,"scripts":[{"name":"QoL","className":"a.QoLPlugin","active":true,"enabled":true}]}`), nil
	case "/settings/plugin":
		if method == http.MethodGet {
			if query.Get("key") == "unset" {
				return json.RawMessage(`{"group":"qol","key":"unset"}`), nil
			}
			return json.RawMessage(`{"group":"qol","key":"` + query.Get("key") + `","value":"v"}`), nil
		}
	case "/scripts/health":
		return nil, errors.New("HTTP 404: no script")
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (g *fakeGame) State(context.Context) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.running {
		return nil, microbot.ErrNotRunning
	}
	return json.RawMessage(`{"loggedIn":false}`), nil
}

func (g *fakeGame) LoginState(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"currentWorld":303}`), nil
}

func (g *fakeGame) Login(_ context.Context, world int) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.worlds = append(g.worlds, world)
	if world == 666 {
		return nil, errors.New("Non-member account cannot login to members world")
	}
	return json.RawMessage(`{"success":true}`), nil
}

func (g *fakeGame) Logout(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"success":true}`), nil
}

func (g *fakeGame) Screenshot(context.Context) ([]byte, error) { return []byte("\x89PNG"), nil }

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// linked starts a server with a Link (token checked like httpapi does) and
// an agent linked to it.
func linked(t *testing.T, game *fakeGame) *Link { return linkedWith(t, game, "") }

// linkedWith is linked with the agent's x11vnc address.
func linkedWith(t *testing.T, game *fakeGame, vncAddr string) *Link {
	t.Helper()
	link := &Link{Log: quiet}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer machine-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/vnc") {
			link.ServeVNC(w, r)
			return
		}
		link.Serve(w, r)
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := &Agent{
		URL: srv.URL, Token: StaticToken("machine-token"), Log: quiet, Hello: Hello{Host: "zaros", Version: "test"}, VNCAddr: vncAddr,
		// The fake mb-client echoes its arguments.
		Executor: &Executor{Game: game, Home: t.TempDir(), ClientCmd: []string{"echo", "mb-client"}},
	}
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for !link.Info().Connected {
		if time.Now().After(deadline) {
			t.Fatal("agent never linked")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return link
}

func TestLinkRunsTheClosedList(t *testing.T) {
	game := &fakeGame{}
	link := linked(t, game)
	ctx := context.Background()
	if info := link.Info(); info.Host != "zaros" || info.Version != "test" {
		t.Fatalf("info = %+v", info)
	}

	var st Status
	if err := link.Call(ctx, CmdStatus, nil, &st); err != nil || st.Running || st.Error != "" {
		t.Fatalf("status with the client down = %+v %v", st, err)
	}
	game.mu.Lock()
	game.running = true
	game.mu.Unlock()
	if err := link.Call(ctx, CmdStatus, nil, &st); err != nil || !st.Running || string(st.Login) != `{"currentWorld":303}` {
		t.Fatalf("status = %+v %v", st, err)
	}

	var out ClientOutput
	if err := link.Call(ctx, CmdClientStart, nil, &out); err != nil || out.Output != "mb-client start --headless" {
		t.Fatalf("client_start = %+v %v", out, err)
	}
	if err := link.Call(ctx, CmdLogin, LoginParams{World: 303}, nil); err != nil {
		t.Fatal(err)
	}
	var cmdErr *CommandError
	if err := link.Call(ctx, CmdLogin, LoginParams{World: 666}, nil); !errors.As(err, &cmdErr) || !strings.Contains(cmdErr.Message, "Non-member") {
		t.Fatalf("failed login = %v", err)
	}
	if err := link.Call(ctx, CmdLogin, LoginParams{World: 5000}, nil); !errors.As(err, &cmdErr) {
		t.Fatalf("world out of range = %v", err)
	}
	game.mu.Lock()
	worlds := append([]int(nil), game.worlds...)
	game.mu.Unlock()
	if len(worlds) != 2 || worlds[0] != 303 {
		t.Fatalf("worlds the game saw = %v", worlds)
	}
	// Not a decodable PNG: it goes out as it came.
	var shot Screenshot
	if err := link.Call(ctx, CmdScreenshot, nil, &shot); err != nil || string(shot.Image) != "\x89PNG" || shot.Type != "image/png" {
		t.Fatalf("screenshot = %q %s %v", shot.Image, shot.Type, err)
	}
	if err := link.Call(ctx, "scripts_deploy", nil, nil); !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("command outside the list = %v", err)
	}
}

func TestLinkWithoutAgent(t *testing.T) {
	link := &Link{Log: quiet}
	if err := link.Call(context.Background(), CmdStatus, nil, nil); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("call without an agent = %v", err)
	}
	if link.Info().Connected {
		t.Fatal("connected without an agent")
	}
}

// The agent refuses a command outside its list even if a server sends one.
func TestExecutorRefusesUnknownCommands(t *testing.T) {
	e := &Executor{Game: &fakeGame{}}
	if _, err := e.Execute(context.Background(), "keyboard", nil); err == nil {
		t.Fatal("executor ran a command outside the closed list")
	}
}

func TestLinkURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://localhost:8080/":     "ws://localhost:8080/api/microbot/agent",
		"https://planner.example.ts": "wss://planner.example.ts/api/microbot/agent",
	} {
		if got := LinkURL(in); got != want {
			t.Errorf("LinkURL(%q) = %q", in, got)
		}
	}
}

func TestScreenshotBecomesJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range img.Pix {
		img.Pix[i] = byte(i)
	}
	var png bytes.Buffer
	if err := pngenc.Encode(&png, img); err != nil {
		t.Fatal(err)
	}
	shot := toJPEG(png.Bytes())
	if shot.Type != "image/jpeg" || !bytes.HasPrefix(shot.Image, []byte{0xFF, 0xD8}) {
		t.Fatalf("screenshot = %s %x", shot.Type, shot.Image[:4])
	}
}
