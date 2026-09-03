package tui

import (
	"bytes"
	"strings"
	"testing"
)

// TestDrawLogIdleNoOpEmitsNothing pins the cursor-blink fix: when
// DrawLog is called with the exact same buffer and cursor position
// as the previous call, it must emit ZERO bytes.
//
// The bug this regresses: at the 120ms animation tick the renderer
// used to always emit SeqHideCursor + cursor-position +
// SeqShowCursor, which resets the terminal's blink timer. Faster
// than the OS blink interval, so an idle dialog editor (e.g. a
// re-opened swarm transcript whose agent isn't producing output)
// rendered the caret as a solid non-blinking block.
//
// With the no-op fast path the renderer leaves the screen alone
// on idle frames, letting the terminal run its own blink cycle.
func TestDrawLogIdleNoOpEmitsNothing(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 24)

	chat := []string{"hello", "world"}
	bottom := []string{"▌ "}
	// First draw populates the renderer's cached buffer.
	r.DrawLog(chat, bottom, 0, 2)
	first := buf.Len()
	if first == 0 {
		t.Fatal("first DrawLog wrote nothing; setup is broken")
	}
	buf.Reset()

	// Identical second draw: same content, same cursor placement.
	r.DrawLog(chat, bottom, 0, 2)
	if buf.Len() != 0 {
		t.Fatalf("idle re-draw emitted %d bytes; expected 0 so terminal blink keeps ticking\n%q",
			buf.Len(), buf.String())
	}
}

// TestDrawLogContentChangeBreaksFastPath proves the no-op fast path
// only fires when nothing changed. A buffer mutation must still
// produce output, otherwise streaming agent replies would freeze on
// screen.
func TestDrawLogContentChangeBreaksFastPath(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 24)

	r.DrawLog([]string{"hello"}, []string{"▌ "}, 0, 2)
	buf.Reset()

	// New chat row lands.
	r.DrawLog([]string{"hello", "world"}, []string{"▌ "}, 0, 2)
	if buf.Len() == 0 {
		t.Fatal("content change suppressed by fast path; streaming output would freeze")
	}
}

func TestDrawLogLargeBottomShrinkKeepsVisibleRowsAddressable(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 4)

	// A long dialog, such as a swarm transcript, scrolls beyond the viewport.
	r.DrawLog(nil, []string{"transcript 1", "transcript 2", "transcript 3", "transcript 4", "transcript 5", "transcript 6"}, -1, 0)
	buf.Reset()

	// Returning to the short dashboard requires a full repaint because one
	// viewport of logical rows disappeared.
	r.DrawLog(nil, []string{"dashboard", "selected agent 1"}, -1, 0)
	buf.Reset()

	// The dashboard remains interactive after that repaint. A cursor move must
	// update its visible selection instead of being discarded as inaccessible.
	r.DrawLog(nil, []string{"dashboard", "selected agent 2"}, -1, 0)
	if got := buf.String(); !strings.Contains(got, "selected agent 2") {
		t.Fatalf("visible update after large dialog shrink was suppressed: %q", got)
	}
}

func TestDrawLogPartialViewportShrinkRepaintsFromDashboardStart(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 4)

	// This transcript overflows the viewport, but returning to the dashboard
	// removes fewer than one viewport of rows.
	r.DrawLog(nil, []string{"transcript 1", "transcript 2", "transcript 3", "transcript 4", "transcript 5"}, -1, 0)
	buf.Reset()

	r.DrawLog(nil, []string{"dashboard", "selected agent 1"}, -1, 0)
	got := buf.String()
	if !strings.Contains(got, SeqClearScreenNoHome) {
		t.Fatalf("partial viewport shrink did not repaint the screen: %q", got)
	}
	for _, want := range []string{"dashboard", "selected agent 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("partial viewport repaint missing %q: %q", want, got)
		}
	}
}

func TestDrawLogChatShrinkDoesNotForceDialogRepaint(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 4)

	r.DrawLog([]string{"chat 1", "chat 2", "chat 3", "chat 4", "chat 5", "chat 6"}, []string{"input"}, 0, 0)
	buf.Reset()

	// Chat reflow has its own coordinate-rebasing path, which preserves native
	// terminal selections. Only a shrinking bottom frame should force the
	// repaint used when returning from a transcript to the dashboard.
	r.DrawLog([]string{"chat 1", "chat 2", "chat 3", "chat 4", "chat 5"}, []string{"input"}, 0, 0)
	if got := buf.String(); strings.Contains(got, SeqClearScreenNoHome) {
		t.Fatalf("chat shrink unnecessarily repainted the screen: %q", got)
	}
}

// TestDrawLogCursorMoveBreaksFastPath proves a cursor-only change
// (no buffer change) still produces output. Without this, typing in
// the editor would visually move the caret but the terminal would
// keep drawing it at the old column.
func TestDrawLogCursorMoveBreaksFastPath(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 24)

	r.DrawLog([]string{"hi"}, []string{"▌ "}, 0, 2)
	buf.Reset()

	// Same buffer, different cursor column.
	r.DrawLog([]string{"hi"}, []string{"▌ "}, 0, 3)
	if buf.Len() == 0 {
		t.Fatal("cursor-only change suppressed by fast path; caret would lag behind typing")
	}
	// And the emitted bytes must at least reposition the cursor.
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("cursor move emission missing CSI escapes: %q", buf.String())
	}
}

// TestDrawLogResizeForcesFullRedraw confirms a resize invalidates
// the cache so the next DrawLog with identical inputs still emits.
// Resize sets logInit=false; without that, a resize followed by an
// identical buffer would falsely no-op and leave a stale frame.
func TestDrawLogResizeForcesFullRedraw(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 24)
	r.DrawLog([]string{"hi"}, []string{"▌ "}, 0, 2)
	buf.Reset()

	r.Resize(100, 30)
	r.DrawLog([]string{"hi"}, []string{"▌ "}, 0, 2)
	if buf.Len() == 0 {
		t.Fatal("post-resize redraw skipped; the new frame would never reach the terminal")
	}
}

// TestDrawLogInaccessibleChangePreservesScrollbackSelection covers long
// streaming output whose changing first row has already scrolled above the
// viewport. That row is immutable terminal history: clearing or replaying it
// either destroys a native mouse selection or duplicates stale tool frames.
func TestDrawLogInaccessibleChangePreservesScrollbackSelection(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 3)
	r.DrawLog([]string{"selected partial", "line 2", "line 3", "line 4"}, []string{"input"}, 0, 0)
	buf.Reset()

	// Only the historical row changed. DrawLog must leave the terminal alone
	// instead of clearing and replaying the retained scrollback.
	r.DrawLog([]string{"selected partial response", "line 2", "line 3", "line 4"}, []string{"input"}, 0, 0)
	got := buf.String()
	if strings.Contains(got, SeqClearScreenNoHome) || strings.Contains(got, SeqClearScrollback) {
		t.Fatalf("inaccessible change cleared selected scrollback: %q", got)
	}
	if strings.Contains(got, "selected partial response") {
		t.Fatalf("inaccessible row was replayed into retained scrollback: %q", got)
	}
}

func TestDrawLogStructuralReflowAboveViewportRebasesWithoutReplay(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 4)
	oldChat := []string{
		"user prompt",
		"┌ bash first ─────", "│ old output", "└──────────────────",
		"┌ bash second ────", "│ second output", "└──────────────────",
	}
	r.DrawLog(oldChat, []string{"input"}, 0, 0)
	buf.Reset()

	// The first tool gains wrapped rows after its top has scrolled away.
	// Clearing the viewport and replaying the complete logical transcript
	// leaves the old prompt in native scrollback and prints a second copy.
	// Rebase the logical row coordinates instead, leaving inaccessible rows
	// as historical snapshots and keeping the visible tail aligned.
	newChat := []string{
		"user prompt",
		"┌ bash first ─────", "│ old output", "│ wrapped line 1", "│ wrapped line 2", "└──────────────────",
		"┌ bash second ────", "│ second output", "└──────────────────",
	}
	r.DrawLog(newChat, []string{"input"}, 0, 0)
	got := buf.String()
	if strings.Contains(got, SeqClearScreenNoHome) || strings.Contains(got, SeqClearScrollback) {
		t.Fatalf("structural reflow cleared the terminal: %q", got)
	}
	for _, replayed := range []string{"user prompt", "┌ bash first", "┌ bash second"} {
		if strings.Contains(got, replayed) {
			t.Fatalf("structural reflow replayed %q into scrollback: %q", replayed, got)
		}
	}

	buf.Reset()
	appended := append(append([]string(nil), newChat...), "┌ bash third ─────", "│ third output", "└──────────────────")
	r.DrawLog(appended, []string{"input"}, 0, 0)
	got = buf.String()
	if !strings.Contains(got, "┌ bash third") || !strings.Contains(got, "│ third output") {
		t.Fatalf("new output was not appended after coordinate rebase: %q", got)
	}
	for _, replayed := range []string{"user prompt", "┌ bash first", "┌ bash second"} {
		if strings.Contains(got, replayed) {
			t.Fatalf("append after rebase replayed %q: %q", replayed, got)
		}
	}
}

func TestDrawLogInaccessibleChangeStillAppendsNewOutput(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 3)
	r.DrawLog([]string{"selected partial", "line 2", "line 3", "line 4"}, []string{"input"}, 0, 0)
	buf.Reset()

	// A streaming reflow changes inaccessible history while a tool result is
	// appended. Only the new suffix should be emitted, naturally scrolling the
	// selected old text upward without replaying the complete frame.
	r.DrawLog([]string{"selected partial response", "line 2", "line 3", "line 4", "tool output"}, []string{"input"}, 0, 0)
	got := buf.String()
	if strings.Contains(got, SeqClearScreenNoHome) || strings.Contains(got, SeqClearScrollback) {
		t.Fatalf("append after inaccessible change cleared selected scrollback: %q", got)
	}
	if strings.Contains(got, "selected partial response") || strings.Contains(got, "line 2") {
		t.Fatalf("append replayed historical rows: %q", got)
	}
	if !strings.Contains(got, "tool output") {
		t.Fatalf("new tool output was not appended: %q", got)
	}
}

// TestDrawLogInvalidationPreservesScrollbackSelection pins the same rule for
// cache invalidations, which can happen during an active turn independently
// of an inaccessible changed row.
func TestDrawLogInvalidationPreservesScrollbackSelection(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	r.Resize(80, 3)
	r.DrawLog([]string{"one", "two", "three", "four"}, []string{"input"}, 0, 0)
	buf.Reset()

	r.Invalidate()
	r.DrawLog([]string{"one", "two", "three", "four"}, []string{"input"}, 0, 0)
	if got := buf.String(); strings.Contains(got, SeqClearScrollback) {
		t.Fatalf("invalidation repaint erased scrollback and native selection: %q", got)
	}
}
