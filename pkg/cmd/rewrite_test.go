package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wayneashleyberry/gh-act/pkg/api"
	"gopkg.in/yaml.v3"
)

func tag(name, sha string) *api.Tag {
	return &api.Tag{
		Name:   name,
		Commit: api.Commit{Sha: sha},
	}
}

// actionAt builds an actions/checkout ParsedAction (written as a major-only
// reference like @v4) whose Node points at a specific line, matching how the
// YAML parser records positions.
func actionAt(line int) ParsedAction {
	return ParsedAction{
		Owner:        "actions",
		Repo:         "checkout",
		VersionStyle: SemanticVersionMajorComponentOnly,
		Node:         yaml.Node{Line: line},
	}
}

func TestRewriteFilePreservesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wf.yml")

	content := "jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v2\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	action := actionAt(4)
	action.LatestVersionTag = tag("v4.0.0", "deadbeef")

	require.NoError(t, rewriteFile(path, []ParsedAction{action}, modeUpdate, false))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t,
		"jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4 # v4.0.0\n",
		string(got),
	)
}

func TestRewriteFilePreservesCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wf.yml")

	content := "jobs:\r\n  build:\r\n    steps:\r\n      - uses: actions/checkout@v2\r\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	action := actionAt(4)
	action.LatestVersionTag = tag("v4.0.0", "deadbeef")

	require.NoError(t, rewriteFile(path, []ParsedAction{action}, modeUpdate, false))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t,
		"jobs:\r\n  build:\r\n    steps:\r\n      - uses: actions/checkout@v4 # v4.0.0\r\n",
		string(got),
	)
}

func TestRewriteFilePinMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wf.yml")

	content := "steps:\n  - uses: actions/checkout@v4\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	action := actionAt(2)
	action.PinVersionTag = tag("v4.2.0", "abc123sha")

	require.NoError(t, rewriteFile(path, []ParsedAction{action}, modePin, false))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "steps:\n  - uses: actions/checkout@abc123sha # v4.2.0\n", string(got))
}

// TestRewriteFileSkipsNilTag verifies the previously panicking / corrupting
// path: when no latest tag is resolved, the action is skipped rather than
// written as an empty "@" reference.
func TestRewriteFileSkipsNilTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wf.yml")

	content := "steps:\n  - uses: actions/checkout@v4\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	action := actionAt(2)
	// LatestVersionTag deliberately left nil.

	require.NotPanics(t, func() {
		require.NoError(t, rewriteFile(path, []ParsedAction{action}, modeUpdatePin, false))
	})

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(got)) // unchanged, not corrupted
}

func TestRewriteFileDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wf.yml")

	content := "steps:\n  - uses: actions/checkout@v2\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	action := actionAt(2)
	action.LatestVersionTag = tag("v4.0.0", "deadbeef")

	require.NoError(t, rewriteFile(path, []ParsedAction{action}, modeUpdate, true))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(got)) // dry-run leaves the file untouched
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")

	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	require.NoError(t, atomicWriteFile(path, []byte("new content")))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "new content", string(got))

	// No temp files should be left behind.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}
