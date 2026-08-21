package agent

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/core"
	"github.com/patriceckhart/zot/packages/provider"
)

func TestParseSessionSelection(t *testing.T) {
	tests := []struct {
		input   string
		count   int
		want    []int
		wantErr bool
	}{
		{input: "", count: 3},
		{input: "none", count: 3},
		{input: "all", count: 3, want: []int{0, 1, 2}},
		{input: "1,3-4,3", count: 4, want: []int{0, 2, 3}},
		{input: "0", count: 3, wantErr: true},
		{input: "4", count: 3, wantErr: true},
		{input: "3-2", count: 3, wantErr: true},
		{input: "1,,2", count: 3, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseSessionSelection(tt.input, tt.count)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("selection = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionsPruneDryRunPreservesSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)
	missing := filepath.Join(home, "missing-project")
	existing := t.TempDir()
	missingPath := createPruneTestSession(t, home, missing)
	existingPath := createPruneTestSession(t, home, existing)

	var out, errOut bytes.Buffer
	if err := runSessionsPrune(sessionsPruneOptions{dryRun: true}, strings.NewReader(""), &out, &errOut, os.Stat); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), missing) || strings.Contains(out.String(), existing) {
		t.Fatalf("output = %q, want only missing cwd", out.String())
	}
	if !strings.Contains(out.String(), "dry run: 1 session in 1 directory would be deleted") {
		t.Fatalf("output = %q, want dry-run summary", out.String())
	}
	for _, path := range []string{missingPath, existingPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("dry run removed %s: %v", path, err)
		}
	}
}

func TestSessionsPruneInteractiveDeletesSelectedGroup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)
	firstCWD := filepath.Join(home, "gone-a")
	secondCWD := filepath.Join(home, "gone-b")
	first := createPruneTestSession(t, home, firstCWD)
	second := createPruneTestSession(t, home, secondCWD)

	var out, errOut bytes.Buffer
	if err := runSessionsPrune(sessionsPruneOptions{}, strings.NewReader("2\ny\n"), &out, &errOut, os.Stat); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("unselected session was removed: %v", err)
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatalf("selected session still exists or stat failed unexpectedly: %v", err)
	}
	if !strings.Contains(out.String(), "deleted 1 session from 1 directory") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSessionsPrunePreservesGroupWhenDirectoryCheckIsInconclusive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)
	cwd := filepath.Join(home, "unavailable-mount")
	path := createPruneTestSession(t, home, cwd)
	stat := func(string) (fs.FileInfo, error) { return nil, fs.ErrPermission }

	var out, errOut bytes.Buffer
	if err := runSessionsPrune(sessionsPruneOptions{all: true, yes: true}, strings.NewReader(""), &out, &errOut, stat); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session was removed after inconclusive stat: %v", err)
	}
	if !strings.Contains(errOut.String(), "permission denied") {
		t.Fatalf("stderr = %q, want stat warning", errOut.String())
	}
	if !strings.Contains(out.String(), "no stale session directories found") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSessionsPruneRechecksDirectoryBeforeDeleting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)
	cwd := filepath.Join(home, "temporarily-missing")
	path := createPruneTestSession(t, home, cwd)
	calls := 0
	stat := func(path string) (fs.FileInfo, error) {
		calls++
		if calls == 1 {
			return nil, fs.ErrNotExist
		}
		return nil, nil
	}

	var out, errOut bytes.Buffer
	err := runSessionsPrune(sessionsPruneOptions{all: true, yes: true}, strings.NewReader(""), &out, &errOut, stat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session was removed after cwd reappeared: %v", err)
	}
	if !strings.Contains(errOut.String(), "directory now exists") {
		t.Fatalf("stderr = %q, want recheck warning", errOut.String())
	}
}

func TestParseSessionsPruneOptionsRequiresAllForYes(t *testing.T) {
	if _, err := parseSessionsPruneOptions([]string{"--yes"}); err == nil {
		t.Fatal("--yes without --all was accepted")
	}
	opts, err := parseSessionsPruneOptions([]string{"--all", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.all || !opts.yes {
		t.Fatalf("options = %#v", opts)
	}
}

func createPruneTestSession(t *testing.T, root, cwd string) string {
	t.Helper()
	session, err := core.NewSession(root, cwd, "test", "test-model", "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AppendMessage(provider.Message{
		Role:    provider.RoleUser,
		Content: []provider.Content{provider.TextBlock{Text: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	return session.Path
}
