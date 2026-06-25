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
			name: "composite action runs.steps",
			yaml: `
runs:
  using: composite
  steps:
    - uses: actions/checkout@v4
    - uses: docker://alpine:3.20
`,
			expected: []string{"actions/checkout@v4"},
		},
		{
			name: "local actions are ignored",
			yaml: `
jobs:
  build:
    steps:
      - uses: ./.github/actions/local
      - uses: actions/checkout@v4
`,
			expected: []string{"actions/checkout@v4"},
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
	writeFile(t, "action.yml", "runs: {}\n")

	files, err := findWorkflowFiles()
	require.NoError(t, err)

	require.ElementsMatch(t, []string{
		filepath.Join(".github", "workflows", "ci.yml"),
		filepath.Join(".github", "workflows", "release.yaml"),
		"action.yml",
	}, files)
}

func TestFindWorkflowFilesMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	files, err := findWorkflowFiles()
	require.NoError(t, err)
	require.Empty(t, files)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
