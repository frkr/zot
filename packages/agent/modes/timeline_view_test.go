package modes

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/provider"
	"github.com/patriceckhart/zot/packages/tui"
)

func timelineTestData() timelineData {
	started := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	finished := started.Add(1250 * time.Millisecond)
	return timelineData{
		System: "You are zot.",
		Tools: []provider.Tool{{
			Name: "bash", Schema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		}},
		Messages: []provider.Message{
			{Role: provider.RoleUser, Time: started.Add(-time.Second), Content: []provider.Content{provider.TextBlock{Text: "list files"}}},
			{Role: provider.RoleAssistant, Time: started, Content: []provider.Content{
				provider.TextBlock{Text: "I will inspect them."},
				provider.ToolCallBlock{ID: "call-1", Name: "bash", Arguments: json.RawMessage(`{"command":"ls -la"}`)},
			}},
			{Role: provider.RoleTool, Time: finished, Content: []provider.Content{
				provider.ToolResultBlock{CallID: "call-1", Content: []provider.Content{provider.TextBlock{Text: "README.md"}}},
			}},
		},
		ContextUsed: 1200,
		ContextMax:  200000,
	}
}

func TestSessionTimelineOpensAlternateView(t *testing.T) {
	interactive := &Interactive{timeline: newTimelineView()}
	interactive.runSlash(context.Background(), "/session timeline")
	if !interactive.timeline.Active() {
		t.Fatal("/session timeline did not open the timeline view")
	}
}

func TestSessionPickerIncludesTimeline(t *testing.T) {
	interactive := &Interactive{sessionOpsDialog: newSessionOpsDialog()}
	interactive.openSessionOpsDialog()
	if len(interactive.sessionOpsDialog.items) == 0 || interactive.sessionOpsDialog.items[0].action != "timeline" {
		t.Fatalf("session actions = %+v, want timeline first", interactive.sessionOpsDialog.items)
	}
}

func TestBuildTimelineEntriesPairsToolDetails(t *testing.T) {
	entries := buildTimelineEntries(timelineTestData())
	if len(entries) != 4 {
		t.Fatalf("entries = %d, want system, user, assistant, and tool", len(entries))
	}
	tool := entries[3]
	if tool.Kind != "tool" || tool.ToolCall != "call-1" {
		t.Fatalf("tool entry = %+v", tool)
	}
	if !strings.Contains(tool.Payload, "ls -la") || !strings.Contains(tool.Result, "README.md") {
		t.Fatalf("tool details missing: %+v", tool)
	}
	if !strings.Contains(tool.Schema, "command") {
		t.Fatalf("tool schema missing: %q", tool.Schema)
	}
	if tool.Duration != "1.25s" {
		t.Fatalf("duration = %q, want 1.25s", tool.Duration)
	}
}

func TestTimelineSearchAndTabs(t *testing.T) {
	data := timelineTestData()
	view := newTimelineView()
	view.Open(data)
	view.HandleKey(tui.Key{Kind: tui.KeyRune, Rune: '/'}, data)
	for _, r := range "readme" {
		view.HandleKey(tui.Key{Kind: tui.KeyRune, Rune: r}, data)
	}
	matches := filterTimelineEntries(buildTimelineEntries(data), view.filter)
	if len(matches) != 1 || matches[0].Kind != "tool" {
		t.Fatalf("matches = %+v, want tool result", matches)
	}
	view.HandleKey(tui.Key{Kind: tui.KeyTab}, data)
	if view.tab != 1 {
		t.Fatalf("tab = %d, want payload tab", view.tab)
	}
	view.HandleKey(tui.Key{Kind: tui.KeyEsc}, data)
	if view.filter != "" || !view.Active() {
		t.Fatal("first esc should clear search without closing timeline")
	}
}

func TestTimelineRenderShowsContextAndControls(t *testing.T) {
	data := timelineTestData()
	if contextLines := renderTimelineContext(tui.Theme{}, 100, data); len(contextLines) != 2 {
		t.Fatalf("context rows = %d, want usage and composition only", len(contextLines))
	}
	view := newTimelineView()
	view.Open(data)
	rendered := strings.Join(view.Render(tui.Theme{}, 100, 40, data), "\n")
	for _, want := range []string{"timeline", "context 1.2k / 200.0k", "estimated composition", "ctrl+e export", "TOOL bash"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render missing %q:\n%s", want, rendered)
		}
	}
}

func TestTimelineExportOmitsImageDataAndUsesPrivatePermissions(t *testing.T) {
	data := timelineTestData()
	data.Messages = append(data.Messages, provider.Message{
		Role:    provider.RoleUser,
		Content: []provider.Content{provider.ImageBlock{MimeType: "image/png", Data: []byte("secret-image-bytes")}},
	})
	view := newTimelineView()
	path, err := view.Export(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "secret-image-bytes") {
		t.Fatal("export embedded image data")
	}
	if !strings.Contains(string(contents), "image/png") {
		t.Fatal("export omitted image metadata")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 600", info.Mode().Perm())
	}
}
