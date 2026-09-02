package modes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/core"
	"github.com/patriceckhart/zot/packages/tui"
)

func TestSessionDialogDeleteRequiresConfirmation(t *testing.T) {
	d := newSessionDialog()
	d.active = true
	d.sessions = []core.SessionSummary{{Path: "/sessions/one.jsonl"}}

	if act := d.HandleKey(tui.Key{Kind: tui.KeyRune, Rune: 'd'}); act.Delete {
		t.Fatal("delete hotkey immediately returned a delete action")
	}
	if !d.deleting {
		t.Fatal("delete hotkey did not open confirmation")
	}
	if act := d.HandleKey(tui.Key{Kind: tui.KeyEnter}); act.Delete {
		t.Fatal("enter confirmed deletion; want safe default cancellation")
	}
	if d.deleting {
		t.Fatal("enter did not close deletion confirmation")
	}

	d.HandleKey(tui.Key{Kind: tui.KeyRune, Rune: 'd'})
	act := d.HandleKey(tui.Key{Kind: tui.KeyRune, Rune: 'y'})
	if !act.Delete || act.Path != "/sessions/one.jsonl" {
		t.Fatalf("confirmed action = %+v, want delete for selected path", act)
	}
	if len(d.sessions) != 1 {
		t.Fatal("dialog removed row before the host confirmed filesystem deletion")
	}
}

func TestSessionDialogDeleteConfirmationRender(t *testing.T) {
	d := newSessionDialog()
	d.active = true
	d.sessions = []core.SessionSummary{{Path: "/sessions/one.jsonl", Title: "keep or delete"}}
	d.HandleKey(tui.Key{Kind: tui.KeyRune, Rune: 'd'})

	plain := strings.Join(d.Render(tui.Theme{}, 100), "\n")
	for _, want := range []string{"permanently delete", "keep or delete", "press y to delete"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("confirmation render missing %q:\n%s", want, plain)
		}
	}
}

func TestSessionDialogRemoveKeepsCursorValid(t *testing.T) {
	tests := []struct {
		name       string
		cursor     int
		removePath string
		wantCursor int
		wantPath   string
	}{
		{name: "first", cursor: 0, removePath: "a", wantCursor: 0, wantPath: "b"},
		{name: "middle", cursor: 1, removePath: "b", wantCursor: 1, wantPath: "c"},
		{name: "last", cursor: 2, removePath: "c", wantCursor: 1, wantPath: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &sessionDialog{
				sessions: []core.SessionSummary{{Path: "a"}, {Path: "b"}, {Path: "c"}},
				cursor:   tt.cursor,
			}
			d.Remove(tt.removePath)
			if d.cursor != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", d.cursor, tt.wantCursor)
			}
			if got := d.sessions[d.cursor].Path; got != tt.wantPath {
				t.Fatalf("selected path = %q, want %q", got, tt.wantPath)
			}
		})
	}

	d := &sessionDialog{sessions: []core.SessionSummary{{Path: "only"}}}
	d.Remove("only")
	if len(d.sessions) != 0 || d.cursor != 0 {
		t.Fatalf("empty dialog has len %d and cursor %d, want 0 and 0", len(d.sessions), d.cursor)
	}
}

func TestApplySessionDeletionRemovesFileAndRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	i := &Interactive{
		cfg:           InteractiveConfig{CurrentSessionPath: func() string { return "" }},
		sessionDialog: &sessionDialog{active: true, sessions: []core.SessionSummary{{Path: path}}},
	}

	i.applySessionDeletion(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("deleted session still exists: %v", err)
	}
	if len(i.sessionDialog.sessions) != 0 {
		t.Fatal("deleted session remains in picker")
	}
	if !strings.Contains(i.statusOK, "deleted session") || i.statusErr != "" {
		t.Fatalf("statusOK = %q, statusErr = %q", i.statusOK, i.statusErr)
	}
}

func TestApplySessionDeletionProtectsActiveSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	i := &Interactive{
		cfg:           InteractiveConfig{CurrentSessionPath: func() string { return path }},
		sessionDialog: &sessionDialog{active: true, sessions: []core.SessionSummary{{Path: path}}},
	}

	i.applySessionDeletion(path)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active session was deleted: %v", err)
	}
	if len(i.sessionDialog.sessions) != 1 {
		t.Fatal("active session was removed from picker")
	}
	if !strings.Contains(i.statusErr, "active session") {
		t.Fatalf("statusErr = %q, want active-session explanation", i.statusErr)
	}
}

func TestApplySessionDeletionRequiresCurrentSessionWiring(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	i := &Interactive{
		sessionDialog: &sessionDialog{active: true, sessions: []core.SessionSummary{{Path: path}}},
	}

	i.applySessionDeletion(path)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session was deleted without active-session wiring: %v", err)
	}
	if !strings.Contains(i.statusErr, "not wired") {
		t.Fatalf("statusErr = %q, want wiring error", i.statusErr)
	}
}

func TestApplySessionDeletionPreservesRowOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.jsonl")
	i := &Interactive{
		cfg:           InteractiveConfig{CurrentSessionPath: func() string { return "" }},
		sessionDialog: &sessionDialog{active: true, sessions: []core.SessionSummary{{Path: path}}},
	}

	i.applySessionDeletion(path)

	if len(i.sessionDialog.sessions) != 1 {
		t.Fatal("failed deletion removed session from picker")
	}
	if !strings.Contains(i.statusErr, "delete session") {
		t.Fatalf("statusErr = %q, want deletion error", i.statusErr)
	}
}
