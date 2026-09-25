package processenv

import (
	"slices"
	"strings"
	"testing"
)

func TestMergeInheritsDaemonEnvironmentAndAppliesOverlay(t *testing.T) {
	// Windows has prefix-related names such as PROGRAMFILES and PROGRAMFILES(X86).
	t.Setenv("OPEN_AGENTS_PROCESSENV", "prefix")
	t.Setenv("OPEN_AGENTS_PROCESSENV(X86)", "related")
	t.Setenv("OPEN_AGENTS_PROCESSENV_INHERITED", "parent")
	t.Setenv("OPEN_AGENTS_PROCESSENV_REPLACED", "old")

	got := Merge(map[string]string{
		"OPEN_AGENTS_PROCESSENV_REPLACED": "new",
		"OPEN_AGENTS_PROCESSENV_SESSION":  "session",
	})
	if !slices.IsSorted(got) {
		t.Fatalf("environment is not sorted: %v", got)
	}
	want := map[string]string{
		"OPEN_AGENTS_PROCESSENV_INHERITED": "parent",
		"OPEN_AGENTS_PROCESSENV_REPLACED":  "new",
		"OPEN_AGENTS_PROCESSENV_SESSION":   "session",
	}
	for _, entry := range got {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			if expected, exists := want[key]; exists {
				if value != expected {
					t.Fatalf("%s = %q, want %q", key, value, expected)
				}
				delete(want, key)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing environment values: %v", want)
	}
}

func TestMergeWindowsExactPATHWinsConflictingOverlaySpelling(t *testing.T) {
	for range 1000 {
		got := merge(
			[]string{"Path=inherited"},
			map[string]string{"Path": "project", "PATH": "open-agents-pinned"},
			true,
		)
		var paths []string
		for _, entry := range got {
			key, _, _ := strings.Cut(entry, "=")
			if strings.EqualFold(key, "PATH") {
				paths = append(paths, entry)
			}
		}
		if !slices.Equal(paths, []string{"PATH=open-agents-pinned"}) {
			t.Fatalf("PATH entries = %v, want protected Open Agents PATH", paths)
		}
	}
}
