package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"daidai-panel/config"

	"github.com/google/uuid"
)

const (
	// TerminalTicketTTL 只覆盖「前端换票 → 立刻升级 WebSocket」这一瞬间。
	// 票据会出现在 URL 查询串里，可能被浏览器历史和反代访问日志记下来，不能放长。
	TerminalTicketTTL = 60 * time.Second
	// TerminalIdleTimeout 是「这段时间没有任何键盘输入」就关掉会话。
	// 输出本身不续期：一条卡住的长命令不该让空闲会话永远占着 PTY。
	TerminalIdleTimeout = 30 * time.Minute
	// TerminalMaxSessions 是全局面板同时打开的终端上限，防止误开很多 PTY 把主机打满。
	TerminalMaxSessions = 3
)

var (
	ErrTerminalTicketInvalid = errors.New("终端票据无效或已过期")
	ErrTerminalSessionLimit  = errors.New("同时打开的终端已达上限")
)

// TerminalInfo 是终端页顶部展示的运行环境摘要。
type TerminalInfo struct {
	Available bool   `json:"available"`
	WorkDir   string `json:"work_dir"`
	Shell     string `json:"shell"`
	Python    string `json:"python"`
	Message   string `json:"message"`
}

type terminalTicket struct {
	username  string
	expiresAt time.Time
}

// TerminalSession 是一条已经拉起的交互 shell。
type TerminalSession struct {
	ID       string
	Username string

	cmd    *exec.Cmd
	stream io.ReadWriteCloser
	resize func(cols, rows uint16) error
	token  *ScriptTokenInfo

	mu        sync.Mutex
	closed    bool
	lastInput time.Time
	idleTimer *time.Timer
	done      chan struct{}
}

var terminalTickets = struct {
	mu    sync.Mutex
	items map[string]terminalTicket
}{items: make(map[string]terminalTicket)}

var terminalSessions = struct {
	mu    sync.Mutex
	items map[string]*TerminalSession
}{items: make(map[string]*TerminalSession)}

// IssueTerminalTicket 签发一张一次性短票，给浏览器 WebSocket 握手用。
// 浏览器原生 WebSocket 带不了 Authorization 头，所以不能直接复用 JWT 中间件。
func IssueTerminalTicket(username string) (string, time.Time, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", time.Time{}, errors.New("用户名为空")
	}
	if TerminalSessionCount() >= TerminalMaxSessions {
		return "", time.Time{}, ErrTerminalSessionLimit
	}

	token := uuid.New().String()
	expiresAt := time.Now().Add(TerminalTicketTTL)

	terminalTickets.mu.Lock()
	purgeExpiredTerminalTicketsLocked(time.Now())
	terminalTickets.items[token] = terminalTicket{username: username, expiresAt: expiresAt}
	terminalTickets.mu.Unlock()

	return token, expiresAt, nil
}

// ConsumeTerminalTicket 校验并立刻作废票据。同一张票不能用来开第二条会话。
func ConsumeTerminalTicket(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrTerminalTicketInvalid
	}

	now := time.Now()
	terminalTickets.mu.Lock()
	defer terminalTickets.mu.Unlock()

	ticket, ok := terminalTickets.items[token]
	if !ok {
		return "", ErrTerminalTicketInvalid
	}
	delete(terminalTickets.items, token)
	if now.After(ticket.expiresAt) {
		return "", ErrTerminalTicketInvalid
	}
	return ticket.username, nil
}

func purgeExpiredTerminalTicketsLocked(now time.Time) {
	for token, ticket := range terminalTickets.items {
		if now.After(ticket.expiresAt) {
			delete(terminalTickets.items, token)
		}
	}
}

// TerminalRuntimeInfo 给前端展示工作目录、shell、托管 Python，方便用户对上 ddp shell。
func TerminalRuntimeInfo() TerminalInfo {
	workDir := terminalWorkDir()
	shell := ResolveInteractiveShell()
	python := ResolveManagedPythonBinary()
	info := TerminalInfo{
		Available: true,
		WorkDir:   workDir,
		Shell:     shell,
		Python:    python,
	}
	if python == "" {
		info.Message = "托管 Python 尚未就绪：先在依赖管理里装一次 Python 包，或确认服务器上有可用的 Python"
	}
	return info
}

func terminalWorkDir() string {
	if config.C == nil {
		return ""
	}
	return strings.TrimSpace(config.C.Data.ScriptsDir)
}

// ResolveInteractiveShell 与 ddp shell 用同一套候选：优先 $SHELL / %COMSPEC%，再退到 bash/sh。
func ResolveInteractiveShell() string {
	if runtime.GOOS == "windows" {
		if c := strings.TrimSpace(os.Getenv("COMSPEC")); c != "" {
			return c
		}
		return "cmd.exe"
	}
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" {
		return s
	}
	for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "sh"
}

// StartTerminalSession 拉起一条与任务执行相同环境的交互 shell。
func StartTerminalSession(username string, cols, rows uint16) (*TerminalSession, error) {
	if TerminalSessionCount() >= TerminalMaxSessions {
		return nil, ErrTerminalSessionLimit
	}

	workDir := terminalWorkDir()
	if workDir == "" {
		return nil, errors.New("脚本目录未配置")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建工作目录失败: %w", err)
	}

	envMap, scriptToken, envErr := BuildManagedRuntimeEnvMapWithScriptToken(
		workDir, workDir, nil, UntimedTaskScriptTokenTTL, "")
	if envMap == nil {
		envMap = map[string]string{}
	}
	if envErr != nil && len(envMap) == 0 {
		RevokeScriptToken(scriptToken)
		return nil, fmt.Errorf("构建运行环境失败: %w", envErr)
	}

	env := mergeTerminalEnviron(os.Environ(), envMap)
	env = WritableHomeEnv(env)
	env = ensureTerminalEnvDefault(env, "TERM", "xterm-256color")
	env = ensureTerminalEnvDefault(env, "COLORTERM", "truecolor")
	env = ensureTerminalEnvDefault(env, "PYTHONUTF8", "1")
	env = ensureTerminalEnvDefault(env, "PYTHONIOENCODING", "utf-8")

	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	cmd, stream, resize, err := startTerminalProcess(ResolveInteractiveShell(), workDir, env, cols, rows)
	if err != nil {
		RevokeScriptToken(scriptToken)
		return nil, err
	}

	session := &TerminalSession{
		ID:        uuid.New().String(),
		Username:  username,
		cmd:       cmd,
		stream:    stream,
		resize:    resize,
		token:     scriptToken,
		lastInput: time.Now(),
		done:      make(chan struct{}),
	}
	session.idleTimer = time.AfterFunc(TerminalIdleTimeout, session.Close)

	if err := registerTerminalSession(session); err != nil {
		session.Close()
		return nil, err
	}

	go session.waitProcess()
	return session, nil
}

func (s *TerminalSession) Read(p []byte) (int, error) {
	if s == nil || s.stream == nil {
		return 0, io.EOF
	}
	return s.stream.Read(p)
}

func (s *TerminalSession) Write(p []byte) (int, error) {
	if s == nil || s.stream == nil {
		return 0, io.ErrClosedPipe
	}
	s.touchInput()
	return s.stream.Write(p)
}

func (s *TerminalSession) Resize(cols, rows uint16) error {
	if s == nil || s.resize == nil {
		return nil
	}
	if cols == 0 || rows == 0 {
		return nil
	}
	return s.resize(cols, rows)
}

func (s *TerminalSession) Done() <-chan struct{} {
	return s.done
}

func (s *TerminalSession) ExitCode() int {
	if s == nil || s.cmd == nil || s.cmd.ProcessState == nil {
		return -1
	}
	return s.cmd.ProcessState.ExitCode()
}

func (s *TerminalSession) touchInput() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.idleTimer == nil {
		return
	}
	s.lastInput = time.Now()
	s.idleTimer.Reset(TerminalIdleTimeout)
}

func (s *TerminalSession) waitProcess() {
	if s.cmd != nil {
		_ = s.cmd.Wait()
	}
	s.Close()
}

// Close 关掉 PTY、杀掉进程组，并立刻吊销注入进这个 shell 的面板凭据。
func (s *TerminalSession) Close() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.mu.Unlock()

	unregisterTerminalSession(s.ID)
	if s.cmd != nil && s.cmd.Process != nil {
		KillProcessGroup(s.cmd.Process)
	}
	if s.stream != nil {
		_ = s.stream.Close()
	}
	RevokeScriptToken(s.token)

	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func registerTerminalSession(session *TerminalSession) error {
	terminalSessions.mu.Lock()
	defer terminalSessions.mu.Unlock()
	if len(terminalSessions.items) >= TerminalMaxSessions {
		return ErrTerminalSessionLimit
	}
	terminalSessions.items[session.ID] = session
	return nil
}

func unregisterTerminalSession(id string) {
	terminalSessions.mu.Lock()
	delete(terminalSessions.items, id)
	terminalSessions.mu.Unlock()
}

func TerminalSessionCount() int {
	terminalSessions.mu.Lock()
	defer terminalSessions.mu.Unlock()
	return len(terminalSessions.items)
}

// ResetTerminalStateForTest 只给测试用：清掉上一例残留的票据和会话，避免上限计数串台。
func ResetTerminalStateForTest() {
	terminalTickets.mu.Lock()
	terminalTickets.items = make(map[string]terminalTicket)
	terminalTickets.mu.Unlock()

	terminalSessions.mu.Lock()
	sessions := make([]*TerminalSession, 0, len(terminalSessions.items))
	for _, session := range terminalSessions.items {
		sessions = append(sessions, session)
	}
	terminalSessions.items = make(map[string]*TerminalSession)
	terminalSessions.mu.Unlock()

	for _, session := range sessions {
		session.Close()
	}
}

func mergeTerminalEnviron(base []string, overrides map[string]string) []string {
	result := append([]string(nil), base...)
	index := make(map[string]int, len(result))
	for i, kv := range result {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			index[kv[:eq]] = i
		}
	}

	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		entry := k + "=" + overrides[k]
		if i, ok := index[k]; ok {
			result[i] = entry
		} else {
			index[k] = len(result)
			result = append(result, entry)
		}
	}
	return result
}

func ensureTerminalEnvDefault(env []string, key, value string) []string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return env
		}
	}
	return append(env, prefix+value)
}
