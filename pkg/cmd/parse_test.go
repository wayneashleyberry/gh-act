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

func TestCollectActionRefsWithMarkdown(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// A standard workflow YAML file with one action reference.
	writeFile(t, filepath.Join(".github", "workflows", "ci.yml"), `
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
`)

	// A markdown file whose fenced YAML block contains a second reference.
	writeFile(t, "README.md", "# Example\n\n```yaml\njobs:\n  fmt:\n    uses: gdcorp-actions/setup-oxfmt/.github/workflows/oxfmt-check.yaml@v1.0.0\n```\n")

	// With markdown disabled only the YAML ref should be found.
	_, refs, err := collectActionRefs(CollectOptions{IncludeMarkdown: false})
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, "actions/checkout@v4", refs[0].Node.Value)

	// With markdown enabled both refs should be found.
	files, refs, err := collectActionRefs(CollectOptions{IncludeMarkdown: true})
	require.NoError(t, err)

	values := make([]string, 0, len(refs))
	for _, r := range refs {
		values = append(values, r.Node.Value)
	}

	require.Contains(t, values, "actions/checkout@v4")
	require.Contains(t, values, "gdcorp-actions/setup-oxfmt/.github/workflows/oxfmt-check.yaml@v1.0.0")

	// The markdown file must appear in the returned file list.
	require.Contains(t, files, "README.md")
}

func TestCollectActionRefsWithFilter(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeFile(t, filepath.Join(".github", "workflows", "ci.yml"), `
jobs:
  call:
    uses: octo-org/repo/.github/workflows/release.yml@v1
  build:
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - uses: golangci/golangci-lint-action@v6
`)

	tests := []struct {
		name     string
		filters  []string
		expected []string
	}{
		{
			name:    "no filter returns everything",
			filters: nil,
			expected: []string{
				"octo-org/repo/.github/workflows/release.yml@v1",
				"actions/checkout@v4",
				"actions/setup-go@v5",
				"golangci/golangci-lint-action@v6",
			},
		},
		{
			name:     "exact match",
			filters:  []string{"actions/setup-go"},
			expected: []string{"actions/setup-go@v5"},
		},
		{
			name:     "owner/repo pattern matches subpath reference",
			filters:  []string{"octo-org/repo"},
			expected: []string{"octo-org/repo/.github/workflows/release.yml@v1"},
		},
		{
			name:     "pattern with extra slashes targets the subpath specifically",
			filters:  []string{"octo-org/repo/.github/workflows/*"},
			expected: []string{"octo-org/repo/.github/workflows/release.yml@v1"},
		},
		{
			name:     "pattern with extra slashes excludes non-matching subpath",
			filters:  []string{"octo-org/repo/.github/workflows/other.yml"},
			expected: []string{},
		},
		{
			name:     "glob match",
			filters:  []string{"actions/*"},
			expected: []string{"actions/checkout@v4", "actions/setup-go@v5"},
		},
		{
			name:     "multiple filters",
			filters:  []string{"actions/setup-go", "golangci/golangci-lint-action"},
			expected: []string{"actions/setup-go@v5", "golangci/golangci-lint-action@v6"},
		},
		{
			name:     "case-insensitive",
			filters:  []string{"ACTIONS/SETUP-GO"},
			expected: []string{"actions/setup-go@v5"},
		},
		{
			name:     "no match returns nothing",
			filters:  []string{"octo/nonexistent"},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, refs, err := collectActionRefs(CollectOptions{Filters: tt.filters})
			require.NoError(t, err)

			values := make([]string, 0, len(refs))
			for _, ref := range refs {
				values = append(values, ref.Node.Value)
			}

			require.Equal(t, tt.expected, values)
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
