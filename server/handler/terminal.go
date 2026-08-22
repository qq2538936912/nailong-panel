package handler

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"daidai-panel/middleware"
	"daidai-panel/pkg/response"
	"daidai-panel/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	terminalWSWriteWait  = 10 * time.Second
	terminalWSPongWait   = 60 * time.Second
	terminalWSPingPeriod = 30 * time.Second
	terminalWSReadLimit  = 64 * 1024
)

type TerminalHandler struct{}

func NewTerminalHandler() *TerminalHandler {
	return &TerminalHandler{}
}

type terminalControlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (h *TerminalHandler) RegisterRoutes(r *gin.RouterGroup) {
	auth := r.Group("/terminal", middleware.JWTAuth(), middleware.RequireUserToken(), middleware.RequireAdmin())
	{
		auth.GET("/info", h.Info)
		auth.POST("/ticket", h.IssueTicket)
	}
	// WebSocket 带不了 Authorization 头，鉴权改走一次性短票。
	r.GET("/terminal/ws", h.WS)
}

func (h *TerminalHandler) Info(c *gin.Context) {
	response.Success(c, gin.H{"data": service.TerminalRuntimeInfo()})
}

func (h *TerminalHandler) IssueTicket(c *gin.Context) {
	username := strings.TrimSpace(c.GetString("username"))
	ticket, expiresAt, err := service.IssueTerminalTicket(username)
	if errors.Is(err, service.ErrTerminalSessionLimit) {
		response.TooManyRequests(c, "同时打开的终端已达上限，请先关掉不用的窗口")
		return
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"ticket":     ticket,
		"expires_at": expiresAt,
		"ws_path":    "/api/v1/terminal/ws",
	})
}

func (h *TerminalHandler) WS(c *gin.Context) {
	username, err := service.ConsumeTerminalTicket(c.Query("ticket"))
	if err != nil {
		response.Unauthorized(c, "终端票据无效或已过期")
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     middleware.AllowWebsocketOrigin,
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	cols := parseTerminalSize(c.Query("cols"), 80)
	rows := parseTerminalSize(c.Query("rows"), 24)
	session, err := service.StartTerminalSession(username, cols, rows)
	if err != nil {
		_ = writeTerminalJSON(conn, nil, map[string]any{
			"type":    "error",
			"message": err.Error(),
		})
		return
	}
	defer session.Close()

	info := service.TerminalRuntimeInfo()
	if err := writeTerminalJSON(conn, nil, map[string]any{
		"type":     "ready",
		"work_dir": info.WorkDir,
		"shell":    info.Shell,
		"python":   info.Python,
	}); err != nil {
		return
	}

	var writeMu sync.Mutex
	conn.SetReadLimit(terminalWSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(terminalWSPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(terminalWSPongWait))
	})

	go pumpTerminalOutput(conn, session, &writeMu)
	go pingTerminalConnection(conn, session, &writeMu)

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if handleTerminalClientMessage(session, messageType, payload) != nil {
			return
		}
	}
}

func handleTerminalClientMessage(session *service.TerminalSession, messageType int, payload []byte) error {
	if messageType == websocket.TextMessage && looksLikeTerminalControl(payload) {
		var control terminalControlMessage
		if err := json.Unmarshal(payload, &control); err == nil && strings.EqualFold(control.Type, "resize") {
			return session.Resize(control.Cols, control.Rows)
		}
	}
	_, err := session.Write(payload)
	return err
}

func looksLikeTerminalControl(payload []byte) bool {
	trimmed := strings.TrimSpace(string(payload))
	return strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"type"`)
}

func pumpTerminalOutput(conn *websocket.Conn, session *service.TerminalSession, writeMu *sync.Mutex) {
	buf := make([]byte, 4096)
	for {
		n, err := session.Read(buf)
		if n > 0 {
			if writeErr := writeTerminalMessage(conn, writeMu, websocket.BinaryMessage, buf[:n]); writeErr != nil {
				session.Close()
				return
			}
		}
		if err != nil {
			code := session.ExitCode()
			_ = writeTerminalJSON(conn, writeMu, map[string]any{
				"type": "exit",
				"code": code,
			})
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(terminalWSWriteWait))
			return
		}
	}
}

func pingTerminalConnection(conn *websocket.Conn, session *service.TerminalSession, writeMu *sync.Mutex) {
	ticker := time.NewTicker(terminalWSPingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-session.Done():
			return
		case <-ticker.C:
			if err := writeTerminalMessage(conn, writeMu, websocket.PingMessage, nil); err != nil {
				session.Close()
				return
			}
		}
	}
}

func writeTerminalJSON(conn *websocket.Conn, writeMu *sync.Mutex, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeTerminalMessage(conn, writeMu, websocket.TextMessage, data)
}

func writeTerminalMessage(conn *websocket.Conn, writeMu *sync.Mutex, messageType int, payload []byte) error {
	if writeMu != nil {
		writeMu.Lock()
		defer writeMu.Unlock()
	}
	if err := conn.SetWriteDeadline(time.Now().Add(terminalWSWriteWait)); err != nil {
		return err
	}
	if err := conn.WriteMessage(messageType, payload); err != nil {
		if errors.Is(err, io.ErrClosedPipe) {
			return err
		}
		return err
	}
	return nil
}

func parseTerminalSize(raw string, fallback uint16) uint16 {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 || value > 1000 {
		return fallback
	}
	return uint16(value)
}
