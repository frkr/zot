package agent

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParseArgsExplicitEmptySystemPromptIsSet(t *testing.T) {
	args, err := ParseArgs([]string{"--system-prompt", ""})
	if err != nil {
		t.Fatalf("ParseArgs returned %v", err)
	}
	if !args.SystemPromptSet {
		t.Fatal("SystemPromptSet = false; want true for an explicitly empty flag value")
	}
	if args.SystemPrompt != "" {
		t.Fatalf("SystemPrompt = %q; want empty", args.SystemPrompt)
	}
}

func TestParseArgsTemperatureAllowsZero(t *testing.T) {
	args, err := ParseArgs([]string{"--temperature", "0"})
	if err != nil {
		t.Fatalf("ParseArgs returned %v", err)
	}
	if args.Temperature == nil || *args.Temperature != 0 {
		t.Fatalf("Temperature = %v; want 0", args.Temperature)
	}
}

func TestParseArgsTemperatureRejectsOutOfRange(t *testing.T) {
	if _, err := ParseArgs([]string{"--temperature", "2.1"}); err == nil {
		t.Fatal("ParseArgs accepted out-of-range temperature")
	}
}

func TestParseArgsYes(t *testing.T) {
	for _, flag := range []string{"-y", "--yes"} {
		args, err := ParseArgs([]string{flag, "--print", "hi"})
		if err != nil {
			t.Fatalf("ParseArgs(%q): %v", flag, err)
		}
		if !args.Yes {
			t.Fatalf("ParseArgs(%q): Yes = false", flag)
		}
		if args.Mode != ModePrint || args.Prompt != "hi" {
			t.Fatalf("ParseArgs(%q): Mode=%q Prompt=%q", flag, args.Mode, args.Prompt)
		}
	}
}

func TestParseArgsStatsRequiresPrintMode(t *testing.T) {
	args, err := ParseArgs([]string{"-p", "--stats", "stats.json", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if args.StatsPath != "stats.json" || args.Mode != ModePrint {
		t.Fatalf("StatsPath=%q Mode=%q", args.StatsPath, args.Mode)
	}

	if _, err := ParseArgs([]string{"--stats", "stats.json", "hi"}); err == nil {
		t.Fatal("ParseArgs accepted --stats without print mode")
	}
}

func TestParseArgsNoContextFiles(t *testing.T) {
	for _, flag := range []string{"--no-context-files", "-nc"} {
		args, err := ParseArgs([]string{flag})
		if err != nil {
			t.Fatalf("ParseArgs(%q): %v", flag, err)
		}
		if !args.NoContextFiles {
			t.Fatalf("ParseArgs(%q): NoContextFiles = false", flag)
		}
	}
}

func TestParseArgsStream(t *testing.T) {
	args, err := ParseArgs([]string{"--stream", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if args.Mode != ModeStream || args.Prompt != "hi" {
		t.Fatalf("Mode=%q Prompt=%q", args.Mode, args.Prompt)
	}
}

func TestRunHelpHelperProcess(t *testing.T) {
	if os.Getenv("ZOT_HELP_HELPER") == "" {
		return
	}
	if err := runWithArgsRaw(strings.Fields(os.Getenv("ZOT_HELP_HELPER")), "test"); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestHelpOutputStreams(t *testing.T) {
	run := func(args string) (stdout, stderr string, err error) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestRunHelpHelperProcess$")
		cmd.Env = append(os.Environ(), "ZOT_HELP_HELPER="+args)
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		err = cmd.Run()
		return outBuf.String(), errBuf.String(), err
	}

	stdout, stderr, err := run("--help")
	if err != nil {
		t.Fatalf("--help returned %v; stderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "zot. yet another coding agent harness.") {
		t.Fatalf("stdout does not contain help text:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("redirected help contains ANSI escapes: %q", stdout)
	}

	stdout, stderr, err = run("--unknown")
	if err == nil {
		t.Fatal("unknown flag exited successfully")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty for an argument error", stdout)
	}
	if !strings.Contains(stderr, "zot. yet another coding agent harness.") {
		t.Fatalf("stderr does not contain help text:\n%s", stderr)
	}
}
