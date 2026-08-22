package tui

import "testing"

func TestReaderParsesCSIUShiftEnter(t *testing.T) {
	k := readKey(t, "\x1b[13;2u")
	if k.Kind != KeyEnter || !k.Shift || k.Alt {
		t.Fatalf("Read kind=%v shift=%v alt=%v, want shift+enter", k.Kind, k.Shift, k.Alt)
	}
}

func TestReaderParsesModifyOtherKeysShiftEnter(t *testing.T) {
	k := readKey(t, "\x1b[27;2;13~")
	if k.Kind != KeyEnter || !k.Shift || k.Alt {
		t.Fatalf("Read kind=%v shift=%v alt=%v, want shift+enter", k.Kind, k.Shift, k.Alt)
	}
}

func TestReaderParsesCSIUCtrlC(t *testing.T) {
	k := readKey(t, "\x1b[99;5u")
	if k.Kind != KeyCtrlC || !k.Ctrl {
		t.Fatalf("Read kind=%v ctrl=%v, want ctrl+c", k.Kind, k.Ctrl)
	}
}

func TestReaderParsesCSIUCtrlNumber(t *testing.T) {
	k := readKey(t, "\x1b[49;5u")
	if k.Kind != KeyRune || k.Rune != '1' || !k.Ctrl {
		t.Fatalf("Read kind=%v rune=%q ctrl=%v, want ctrl+1", k.Kind, k.Rune, k.Ctrl)
	}
}

func TestReaderParsesCSIUSuperNumber(t *testing.T) {
	k := readKey(t, "\x1b[50;9u")
	if k.Kind != KeyRune || k.Rune != '2' || !k.Super {
		t.Fatalf("Read kind=%v rune=%q super=%v, want super+2", k.Kind, k.Rune, k.Super)
	}
}

func TestReaderParsesCSIUSuperNumberWithEventType(t *testing.T) {
	k := readKey(t, "\x1b[51;9:3u")
	if k.Kind != KeyRune || k.Rune != '3' || !k.Super {
		t.Fatalf("Read kind=%v rune=%q super=%v, want super+3", k.Kind, k.Rune, k.Super)
	}
}

func TestReaderParsesCSIUHyperNumberAsSuper(t *testing.T) {
	k := readKey(t, "\x1b[52;33u")
	if k.Kind != KeyRune || k.Rune != '4' || !k.Super {
		t.Fatalf("Read kind=%v rune=%q super=%v, want hyper+4 as super", k.Kind, k.Rune, k.Super)
	}
}

func TestReaderParsesRawCtrlVAsClipboardPaste(t *testing.T) {
	k := readKey(t, "\x16")
	if k.Kind != KeyPasteClipboard || !k.Ctrl {
		t.Fatalf("Read kind=%v ctrl=%v, want ctrl+v clipboard paste", k.Kind, k.Ctrl)
	}
}

func TestReaderParsesCSIUCtrlVAsClipboardPaste(t *testing.T) {
	k := readKey(t, "\x1b[118;5u")
	if k.Kind != KeyPasteClipboard || !k.Ctrl {
		t.Fatalf("Read kind=%v ctrl=%v, want enhanced ctrl+v clipboard paste", k.Kind, k.Ctrl)
	}
}

func TestReaderParsesModifyOtherKeysCtrlC(t *testing.T) {
	k := readKey(t, "\x1b[27;5;99~")
	if k.Kind != KeyCtrlC || !k.Ctrl {
		t.Fatalf("Read kind=%v ctrl=%v, want ctrl+c", k.Kind, k.Ctrl)
	}
}

func TestReaderParsesCSIUEsc(t *testing.T) {
	k := readKey(t, "\x1b[27u")
	if k.Kind != KeyEsc {
		t.Fatalf("Read kind=%v, want esc", k.Kind)
	}
}

func TestReaderParsesCSIUTabAndBackspace(t *testing.T) {
	if k := readKey(t, "\x1b[9u"); k.Kind != KeyTab {
		t.Fatalf("Read kind=%v, want tab", k.Kind)
	}
	if k := readKey(t, "\x1b[9;2u"); k.Kind != KeyShiftTab {
		t.Fatalf("Read kind=%v, want shift-tab", k.Kind)
	}
	if k := readKey(t, "\x1b[127u"); k.Kind != KeyBackspace {
		t.Fatalf("Read kind=%v, want backspace", k.Kind)
	}
}

func TestReaderParsesSS3CursorKeys(t *testing.T) {
	cases := []struct {
		seq  string
		want KeyKind
	}{
		{"\x1bOA", KeyUp},
		{"\x1bOB", KeyDown},
		{"\x1bOC", KeyRight},
		{"\x1bOD", KeyLeft},
	}
	for _, tc := range cases {
		if k := readKey(t, tc.seq); k.Kind != tc.want {
			t.Fatalf("Read(%q) kind=%v, want %v", tc.seq, k.Kind, tc.want)
		}
	}
}

func TestReaderParsesSGRMouseWheel(t *testing.T) {
	cases := []struct {
		seq  string
		want KeyKind
	}{
		{"\x1b[<64;10;20M", KeyMouseWheelUp},
		{"\x1b[<65;10;20M", KeyMouseWheelDown},
	}
	for _, tc := range cases {
		k := readKey(t, tc.seq)
		if k.Kind != tc.want {
			t.Fatalf("Read(%q) kind=%v, want %v", tc.seq, k.Kind, tc.want)
		}
	}
}

func readKey(t *testing.T, seq string) Key {
	t.Helper()
	idx := 0
	r := NewReader(func() (byte, error) {
		b := seq[idx]
		idx++
		return b, nil
	})
	k, err := r.Read()
	if err != nil {
		t.Fatalf("Read(%q): %v", seq, err)
	}
	return k
}
