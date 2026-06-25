package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseActionRefs(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected []string
	}{
		{
			name: "job steps",
			yaml: `
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
      - run: echo hi
      - uses: actions/setup-go@v5
`,
			expected: []string{"actions/checkout@v4", "actions/setup-go@v5"},
		},
		{
			name: "reusable workflow call at job level",
			yaml: `
jobs:
  call:
    uses: octo-org/this-repo/.github/workflows/release.yml@v1
`,
			expected: []string{"octo-org/this-repo/.github/workflows/release.yml@v1"},
		},
		{
			name: "composite action runs.steps include docker refs",
			yaml: `
runs:
  using: composite
  steps:
    - uses: actions/checkout@v4
    - uses: docker://alpine:3.20
`,
			expected: []string{"actions/checkout@v4", "docker://alpine:3.20"},
		},
		{
			name: "local actions are listed",
			yaml: `
jobs:
  build:
    steps:
      - uses: ./.github/actions/local
      - uses: actions/checkout@v4
`,
			expected: []string{"./.github/actions/local", "actions/checkout@v4"},
		},
		{
			name: "mixed reusable and steps in one job set",
			yaml: `
jobs:
  call:
    uses: octo-org/repo/.github/workflows/wf.yml@main
  build:
    steps:
      - uses: actions/checkout@v4
`,
			expected: []string{"octo-org/repo/.github/workflows/wf.yml@main", "actions/checkout@v4"},
		},
		{
			name:     "empty document",
			yaml:     "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := parseActionRefs([]byte(tt.yaml), "workflow.yml")
			require.NoError(t, err)

			values := make([]string, 0, len(refs))
			for _, ref := range refs {
				values = append(values, ref.Node.Value)
			}

			require.Equal(t, tt.expected, values)
		})
	}
}

func TestFindWorkflowFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	require.NoError(t, os.MkdirAll(filepath.Join(".github", "workflows"), 0o755))
	writeFile(t, filepath.Join(".github", "workflows", "ci.yml"), "jobs: {}\n")
	writeFile(t, filepath.Join(".github", "workflows", "release.yaml"), "jobs: {}\n")
	writeFile(t, filepath.Join(".github", "workflows", "notes.txt"), "ignored\n")
	// Nested workflow and other .github YAML must still be scanned (historical scope).
	writeFile(t, filepath.Join(".github", "workflows", "nested", "deep.yml"), "jobs: {}\n")
	writeFile(t, filepath.Join(".github", "dependabot.yml"), "version: 2\n")
	// Composite actions: at the root, under .github/actions, and elsewhere.
	writeFile(t, "action.yml", "runs: {}\n")
	writeFile(t, filepath.Join(".github", "actions", "setup", "action.yml"), "runs: {}\n")
	writeFile(t, filepath.Join("tools", "deep", "action.yaml"), "runs: {}\n")
	// These must be skipped.
	writeFile(t, filepath.Join(".git", "action.yml"), "runs: {}\n")
	writeFile(t, filepath.Join("node_modules", "pkg", "action.yml"), "runs: {}\n")
	writeFile(t, filepath.Join("vendor", "dep", "action.yaml"), "runs: {}\n")

	files, err := findWorkflowFiles()
	require.NoError(t, err)

	require.ElementsMatch(t, []string{
		filepath.Join(".github", "workflows", "ci.yml"),
		filepath.Join(".github", "workflows", "release.yaml"),
		filepath.Join(".github", "workflows", "nested", "deep.yml"),
		filepath.Join(".github", "dependabot.yml"),
		"action.yml",
		filepath.Join(".github", "actions", "setup", "action.yml"),
		filepath.Join("tools", "deep", "action.yaml"),
	}, files)
}

func TestFindWorkflowFilesMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	files, err := findWorkflowFiles()
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestIsPinnableRef(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "actions/checkout@v4", want: true},
		{value: "octo/repo/.github/workflows/wf.yml@v1", want: true},
		{value: "./.github/actions/local", want: false},
		{value: "../shared/action", want: false},
		{value: "docker://alpine:3.20", want: false},
		{value: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			require.Equal(t, tt.want, isPinnableRef(tt.value))
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
