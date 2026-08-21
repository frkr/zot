package agent

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/patriceckhart/zot/packages/core"
)

type sessionsPruneOptions struct {
	dryRun bool
	all    bool
	yes    bool
}

type statPathFunc func(string) (fs.FileInfo, error)

// runSessionsCommand dispatches standalone session management commands before
// normal agent startup, so they do not require a provider or credentials.
func runSessionsCommand(rawArgs []string) (handled bool, err error) {
	if len(rawArgs) == 0 || rawArgs[0] != "sessions" {
		return false, nil
	}
	if len(rawArgs) == 1 {
		printSessionsHelp(os.Stdout)
		return true, nil
	}
	switch rawArgs[1] {
	case "help", "-h", "--help":
		printSessionsHelp(os.Stdout)
		return true, nil
	case "prune":
		opts, err := parseSessionsPruneOptions(rawArgs[2:])
		if errors.Is(err, errSessionsPruneHelp) {
			printSessionsHelp(os.Stdout)
			return true, nil
		}
		if err != nil {
			printSessionsHelp(os.Stderr)
			return true, err
		}
		return true, runSessionsPrune(opts, os.Stdin, os.Stdout, os.Stderr, os.Stat)
	default:
		printSessionsHelp(os.Stderr)
		return true, fmt.Errorf("unknown sessions subcommand: %s", rawArgs[1])
	}
}

func parseSessionsPruneOptions(args []string) (sessionsPruneOptions, error) {
	var opts sessionsPruneOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			opts.dryRun = true
		case "--all":
			opts.all = true
		case "-y", "--yes":
			opts.yes = true
		case "-h", "--help":
			return opts, errSessionsPruneHelp
		default:
			return opts, fmt.Errorf("unknown sessions prune flag: %s", arg)
		}
	}
	if opts.yes && !opts.all {
		return opts, fmt.Errorf("--yes requires --all")
	}
	if opts.yes && opts.dryRun {
		return opts, fmt.Errorf("--yes cannot be used with --dry-run")
	}
	return opts, nil
}

var errSessionsPruneHelp = errors.New("sessions prune help requested")

func printSessionsHelp(out io.Writer) {
	fmt.Fprintln(out, `zot sessions - manage stored sessions

usage:
  zot sessions prune             select stale directory groups and confirm deletion
  zot sessions prune --dry-run   list stale groups without deleting anything
  zot sessions prune --all       select every stale group, then confirm
  zot sessions prune --all --yes delete every stale group without prompting

A group is stale only when its recorded working directory no longer exists.
Unreadable, malformed, and temporarily inaccessible entries are preserved.`)
}

func runSessionsPrune(opts sessionsPruneOptions, in io.Reader, out, errOut io.Writer, stat statPathFunc) error {
	groups, scanIssues := core.ScanStoredSessionGroups(SessionsPath())
	for _, issue := range scanIssues {
		fmt.Fprintf(errOut, "warning: preserving %s: %v\n", issue.Path, issue.Err)
	}

	stale := make([]core.StoredSessionGroup, 0, len(groups))
	for _, group := range groups {
		if !filepath.IsAbs(group.CWD) {
			fmt.Fprintf(errOut, "warning: preserving sessions with non-absolute cwd %q\n", group.CWD)
			continue
		}
		_, err := stat(group.CWD)
		switch {
		case err == nil:
			continue
		case errors.Is(err, fs.ErrNotExist):
			stale = append(stale, group)
		default:
			fmt.Fprintf(errOut, "warning: preserving sessions for %s: %v\n", group.CWD, err)
		}
	}
	if len(stale) == 0 {
		fmt.Fprintln(out, "no stale session directories found")
		return nil
	}

	fmt.Fprintln(out, "stale session directories:")
	for idx, group := range stale {
		fmt.Fprintf(out, "  %d. %s (%s, %d bytes)\n", idx+1, group.CWD, sessionCount(len(group.Paths)), group.SizeBytes)
	}
	if opts.dryRun {
		fmt.Fprintf(out, "dry run: %s in %s would be deleted\n", sessionCount(totalSessions(stale)), directoryCount(len(stale)))
		return nil
	}

	reader := bufio.NewReader(in)
	selected := make([]int, 0, len(stale))
	if opts.all {
		for idx := range stale {
			selected = append(selected, idx)
		}
	} else {
		fmt.Fprint(out, "select groups to delete (for example 1,3-5; all; none): ")
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read selection: %w", err)
		}
		selected, err = parseSessionSelection(line, len(stale))
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			fmt.Fprintln(out, "no sessions deleted")
			return nil
		}
	}

	selectedSessions := 0
	for _, idx := range selected {
		selectedSessions += len(stale[idx].Paths)
	}
	if !opts.yes {
		fmt.Fprintf(out, "permanently delete %s in %s? [y/N]: ", sessionCount(selectedSessions), directoryCount(len(selected)))
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read confirmation: %w", err)
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "no sessions deleted")
			return nil
		}
	}

	deletedSessions := 0
	deletedGroups := 0
	var deleteErrors []error
	for _, idx := range selected {
		group := stale[idx]
		_, err := stat(group.CWD)
		if err == nil {
			fmt.Fprintf(errOut, "warning: preserving sessions for %s because the directory now exists\n", group.CWD)
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(errOut, "warning: preserving sessions for %s after recheck: %v\n", group.CWD, err)
			continue
		}

		groupDeleted := 0
		for _, path := range group.Paths {
			if err := core.DeleteSession(path); err != nil {
				deleteErrors = append(deleteErrors, fmt.Errorf("delete %s: %w", path, err))
				continue
			}
			groupDeleted++
			_ = os.Remove(filepath.Dir(path))
		}
		deletedSessions += groupDeleted
		if groupDeleted == len(group.Paths) {
			deletedGroups++
		}
	}
	fmt.Fprintf(out, "deleted %s from %s\n", sessionCount(deletedSessions), directoryCount(deletedGroups))
	return errors.Join(deleteErrors...)
}

func parseSessionSelection(input string, count int) ([]int, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	switch input {
	case "", "none", "n":
		return nil, nil
	case "all", "a":
		selected := make([]int, count)
		for idx := range selected {
			selected[idx] = idx
		}
		return selected, nil
	}

	seen := make(map[int]bool)
	for _, field := range strings.Split(input, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, fmt.Errorf("invalid empty selection")
		}
		startText, endText, isRange := strings.Cut(field, "-")
		start, err := strconv.Atoi(strings.TrimSpace(startText))
		if err != nil {
			return nil, fmt.Errorf("invalid selection %q", field)
		}
		end := start
		if isRange {
			end, err = strconv.Atoi(strings.TrimSpace(endText))
			if err != nil || end < start {
				return nil, fmt.Errorf("invalid selection range %q", field)
			}
		}
		if start < 1 || end > count {
			return nil, fmt.Errorf("selection %q is outside 1-%d", field, count)
		}
		for value := start; value <= end; value++ {
			seen[value-1] = true
		}
	}
	selected := make([]int, 0, len(seen))
	for idx := range seen {
		selected = append(selected, idx)
	}
	sort.Ints(selected)
	return selected, nil
}

func totalSessions(groups []core.StoredSessionGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Paths)
	}
	return total
}

func sessionCount(count int) string {
	if count == 1 {
		return "1 session"
	}
	return fmt.Sprintf("%d sessions", count)
}

func directoryCount(count int) string {
	if count == 1 {
		return "1 directory"
	}
	return fmt.Sprintf("%d directories", count)
}
