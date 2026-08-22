package service

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"daidai-panel/testutil"
)

func TestIssueAndConsumeTerminalTicket(t *testing.T) {
	ResetTerminalStateForTest()

	token, expiresAt, err := IssueTerminalTicket("admin")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty ticket")
	}
	if time.Until(expiresAt) > TerminalTicketTTL || time.Until(expiresAt) < TerminalTicketTTL-2*time.Second {
		t.Fatalf("unexpected ticket ttl, expires at %v", expiresAt)
	}

	username, err := ConsumeTerminalTicket(token)
	if err != nil {
		t.Fatalf("consume ticket: %v", err)
	}
	if username != "admin" {
		t.Fatalf("expected admin, got %q", username)
	}

	if _, err := ConsumeTerminalTicket(token); err != ErrTerminalTicketInvalid {
		t.Fatalf("expected one-time ticket, got %v", err)
	}
}

func TestConsumeTerminalTicketRejectsEmptyAndUnknown(t *testing.T) {
	ResetTerminalStateForTest()

	if _, err := ConsumeTerminalTicket(""); err != ErrTerminalTicketInvalid {
		t.Fatalf("expected empty ticket rejected, got %v", err)
	}
	if _, err := ConsumeTerminalTicket("missing"); err != ErrTerminalTicketInvalid {
		t.Fatalf("expected unknown ticket rejected, got %v", err)
	}
}

func TestConsumeExpiredTerminalTicket(t *testing.T) {
	ResetTerminalStateForTest()

	token, _, err := IssueTerminalTicket("admin")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}

	terminalTickets.mu.Lock()
	ticket := terminalTickets.items[token]
	ticket.expiresAt = time.Now().Add(-time.Second)
	terminalTickets.items[token] = ticket
	terminalTickets.mu.Unlock()

	if _, err := ConsumeTerminalTicket(token); err != ErrTerminalTicketInvalid {
		t.Fatalf("expected expired ticket rejected, got %v", err)
	}
}

func TestTerminalRuntimeInfoUsesScriptsDir(t *testing.T) {
	testutil.SetupTestEnv(t)
	ResetTerminalStateForTest()

	info := TerminalRuntimeInfo()
	if !info.Available {
		t.Fatal("expected terminal to be available")
	}
	if info.WorkDir == "" || !strings.HasSuffix(info.WorkDir, "scripts") {
		t.Fatalf("unexpected work dir: %q", info.WorkDir)
	}
	if info.Shell == "" {
		t.Fatal("expected a shell")
	}
}

func TestMergeTerminalEnvironOverridesAndKeepsBase(t *testing.T) {
	got := mergeTerminalEnviron([]string{"PATH=/bin", "FOO=1"}, map[string]string{
		"PATH": "/opt/venv/bin:/bin",
		"BAR":  "2",
	})
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "PATH=/opt/venv/bin:/bin") {
		t.Fatalf("expected PATH override, got %q", joined)
	}
	if !strings.Contains(joined, "FOO=1") || !strings.Contains(joined, "BAR=2") {
		t.Fatalf("expected base and override keys, got %q", joined)
	}
	if strings.Count(joined, "PATH=") != 1 {
		t.Fatalf("expected single PATH entry, got %q", joined)
	}
}

func TestStartTerminalSessionUsesManagedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("interactive pty echo is unix-only")
	}
	testutil.SetupTestEnv(t)
	ResetTerminalStateForTest()
	t.Cleanup(ResetTerminalStateForTest)

	session, err := StartTerminalSession("admin", 80, 24)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer session.Close()

	if _, err := session.Write([]byte("echo daidai-terminal-ok\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	type chunk struct {
		data []byte
		err  error
	}
	reads := make(chan chunk, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := session.Read(buf)
			if n > 0 {
				reads <- chunk{data: append([]byte(nil), buf[:n]...)}
			}
			if readErr != nil {
				reads <- chunk{err: readErr}
				return
			}
		}
	}()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("expected echo output, got %q", output.String())
		case item := <-reads:
			output.Write(item.data)
			if strings.Contains(output.String(), "daidai-terminal-ok") {
				return
			}
			if item.err != nil {
				t.Fatalf("read session: %v; output=%q", item.err, output.String())
			}
		}
	}
}
