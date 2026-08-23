package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"panel/service"
	"panel/testutil"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func newTerminalTestRouter() *gin.Engine {
	engine := gin.New()
	NewTerminalHandler().RegisterRoutes(engine.Group("/api/v1"))
	return engine
}

func TestTerminalInfoRequiresAdmin(t *testing.T) {
	testutil.SetupTestEnv(t)
	service.ResetTerminalStateForTest()
	engine := newTerminalTestRouter()

	cases := []struct {
		name string
		role string
		want int
	}{
		{name: "admin", role: "admin", want: http.StatusOK},
		{name: "operator", role: "operator", want: http.StatusForbidden},
		{name: "viewer", role: "viewer", want: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := testutil.MustCreateAccessToken(t, tc.name+"-user", tc.role)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/terminal/info", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d body=%s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestTerminalTicketRejectsAppToken(t *testing.T) {
	testutil.SetupTestEnv(t)
	service.ResetTerminalStateForTest()
	engine := newTerminalTestRouter()

	token := testutil.MustCreateAppToken(t, "terminal-app", "*")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/terminal/ticket", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for app token, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTerminalTicketIssuesForAdmin(t *testing.T) {
	testutil.SetupTestEnv(t)
	service.ResetTerminalStateForTest()
	engine := newTerminalTestRouter()

	token := testutil.MustCreateAccessToken(t, "admin", "admin")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/terminal/ticket", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ticket, _ := payload["ticket"].(string)
	if ticket == "" {
		t.Fatalf("expected ticket, got %#v", payload)
	}
	if payload["ws_path"] != "/api/v1/terminal/ws" {
		t.Fatalf("unexpected ws_path: %#v", payload["ws_path"])
	}
}

func TestTerminalWSRequiresTicket(t *testing.T) {
	testutil.SetupTestEnv(t)
	service.ResetTerminalStateForTest()

	server := httptest.NewServer(newTerminalTestRouter())
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/terminal/ws"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected websocket without ticket to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got resp=%v err=%v", resp, err)
	}
}

func TestTerminalWSRejectsUsedTicket(t *testing.T) {
	testutil.SetupTestEnv(t)
	service.ResetTerminalStateForTest()
	engine := newTerminalTestRouter()

	ticket, _, err := service.IssueTerminalTicket("admin")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if _, err := service.ConsumeTerminalTicket(ticket); err != nil {
		t.Fatalf("consume ticket: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/terminal/ws?ticket="+ticket, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for used ticket, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDockerNginxUpgradesTerminalWebsocket(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docker", "nginx.conf"))
	if err != nil {
		t.Fatalf("read nginx.conf: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "location ^~ /api/v1/terminal/ws") {
		t.Fatal("docker/nginx.conf must pin a longer prefix for the terminal websocket")
	}
	if !strings.Contains(text, "proxy_set_header Upgrade $http_upgrade") {
		t.Fatal("docker/nginx.conf must upgrade the terminal websocket")
	}
	if !strings.Contains(text, `proxy_set_header Connection "upgrade"`) {
		t.Fatal("docker/nginx.conf must send Connection: upgrade for the terminal websocket")
	}
}
