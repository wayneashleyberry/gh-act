package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsMarkdownFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "README.md", want: true},
		{name: "readme.MD", want: true},
		{name: "CONTRIBUTING.markdown", want: true},
		{name: "docs.MARKDOWN", want: true},
		{name: "workflow.yml", want: false},
		{name: "action.yaml", want: false},
		{name: "noextension", want: false},
		{name: ".md", want: true}, // dotfile with .md extension
		{name: "file.mdx", want: true},
		{name: "file.MDX", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isMarkdownFile(tt.name))
		})
	}
}

func TestExtractYAMLBlocks(t *testing.T) {
	tests := []struct {
		name           string
		markdown       string
		wantOffsets    []int
		wantContents   []string
		wantBlockCount int
	}{
		{
			name:           "single yaml block",
			markdown:       "# Title\n\nSome text.\n\n```yaml\njobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{6},
			wantContents:   []string{"jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4"},
		},
		{
			name:           "multiple blocks",
			markdown:       "```yaml\nfirst: block\n```\n\nSome prose.\n\n```yaml\nsecond: block\n```\n",
			wantBlockCount: 2,
			wantOffsets:    []int{2, 8},
			wantContents:   []string{"first: block", "second: block"},
		},
		{
			name:           "non-yaml fences are ignored",
			markdown:       "```bash\necho hello\n```\n\n```go\nfmt.Println()\n```\n\n```yml\nruns: {}\n```\n",
			wantBlockCount: 0,
		},
		{
			name:           "unclosed fence is discarded",
			markdown:       "```yaml\njobs: {}\n",
			wantBlockCount: 0,
		},
		{
			name:           "no code blocks",
			markdown:       "# Just prose\n\nNo fences here.\n",
			wantBlockCount: 0,
		},
		{
			name:           "case insensitive fence",
			markdown:       "```YAML\njobs: {}\n```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{2},
			wantContents:   []string{"jobs: {}"},
		},
		{
			name:           "fence with leading whitespace",
			markdown:       "  ```yaml\n  jobs: {}\n  ```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{2},
			wantContents:   []string{"  jobs: {}"},
		},
		{
			name:           "empty yaml block",
			markdown:       "```yaml\n```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{2},
			wantContents:   []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := extractYAMLBlocks([]byte(tt.markdown))
			require.Len(t, blocks, tt.wantBlockCount)

			for i, block := range blocks {
				require.Equal(t, tt.wantOffsets[i], block.lineOffset, "block %d lineOffset", i)
				require.Equal(t, tt.wantContents[i], string(block.content), "block %d content", i)
			}
		})
	}
}

func TestExtractYAMLBlocksCRLF(t *testing.T) {
	// CRLF line endings should be handled transparently.
	markdown := "```yaml\r\njobs:\r\n  build:\r\n    steps:\r\n      - uses: actions/checkout@v4\r\n```\r\n"
	blocks := extractYAMLBlocks([]byte(markdown))
	require.Len(t, blocks, 1)
	require.Equal(t, 2, blocks[0].lineOffset)
	// \r should be stripped from content lines.
	require.Equal(t, "jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4", string(blocks[0].content))
}

func TestFindActionRefsInMarkdownFile(t *testing.T) {
	dir := t.TempDir()

	// Markdown line layout (1-indexed):
	//  1: # README
	//  2: (blank)
	//  3: Some docs.
	//  4: (blank)
	//  5: ```yaml          <- fence opens; blockStart = 5+2 = 7... wait, i=4 (0-indexed) → blockStart = 4+2 = 6
	//  6: jobs:            <- YAML line 1
	//  7:   build:         <- YAML line 2
	//  8:     steps:       <- YAML line 3
	//  9:       - uses:    <- YAML line 4  →  file line = blockStart + YAML line - 1 = 6 + 4 - 1 = 9
	// 10: ```
	markdown := "# README\n\nSome docs.\n\n```yaml\njobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n```\n"

	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte(markdown), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	require.Equal(t, "actions/checkout@v4", actions[0].Node.Value)
	require.Equal(t, 9, actions[0].Node.Line)
}

func TestFindActionRefsInMarkdownFileMultipleBlocks(t *testing.T) {
	dir := t.TempDir()

	// Markdown line layout (1-indexed):
	//  1: # Title
	//  2: (blank)
	//  3: ```yaml        ← i=2 (0-indexed) → blockStart=4
	//  4: jobs:
	//  5:   a:
	//  6:     steps:
	//  7:       - uses: actions/checkout@v4   ← YAML line 4 → file line 4+4-1 = 7
	//  8: ```
	//  9: (blank)
	// 10: More prose.
	// 11: (blank)
	// 12: ```yaml        ← i=11 (0-indexed) → blockStart=13
	// 13: jobs:
	// 14:   b:
	// 15:     steps:
	// 16:       - uses: actions/setup-go@v5   ← YAML line 4 → file line 13+4-1 = 16
	// 17: ```
	markdown := "# Title\n\n```yaml\njobs:\n  a:\n    steps:\n      - uses: actions/checkout@v4\n```\n\nMore prose.\n\n```yaml\njobs:\n  b:\n    steps:\n      - uses: actions/setup-go@v5\n```\n"

	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte(markdown), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Len(t, actions, 2)

	require.Equal(t, "actions/checkout@v4", actions[0].Node.Value)
	require.Equal(t, 7, actions[0].Node.Line)

	require.Equal(t, "actions/setup-go@v5", actions[1].Node.Value)
	require.Equal(t, 16, actions[1].Node.Line)
}

func TestFindActionRefsInMarkdownFileNoBlocks(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte("# Just prose\n\nNo YAML here.\n"), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Empty(t, actions)
}

func TestFindActionRefsInMarkdownFileMalformedYAML(t *testing.T) {
	dir := t.TempDir()

	// Malformed YAML inside a fence should not cause an error — it's skipped.
	markdown := "```yaml\n: : : invalid\n```\n\n```yaml\njobs:\n  b:\n    steps:\n      - uses: actions/checkout@v4\n```\n"
	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte(markdown), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	require.Equal(t, "actions/checkout@v4", actions[0].Node.Value)
}

func TestFindMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeFile(t, "README.md", "# root\n")
	writeFile(t, "CONTRIBUTING.markdown", "# contrib\n")
	writeFile(t, filepath.Join("docs", "guide.md"), "# guide\n")
	writeFile(t, filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"), "# pr\n")
	writeFile(t, "page.mdx", "# mdx page\n")
	writeFile(t, filepath.Join("docs", "component.mdx"), "# mdx component\n")
	// Non-markdown files must be excluded.
	writeFile(t, "workflow.yml", "jobs: {}\n")
	writeFile(t, filepath.Join("docs", "notes.txt"), "ignored\n")
	// Skipped directories must not appear.
	writeFile(t, filepath.Join(".git", "COMMIT_EDITMSG"), "ignored\n")
	writeFile(t, filepath.Join("node_modules", "pkg", "README.md"), "ignored\n")
	writeFile(t, filepath.Join("vendor", "dep", "README.md"), "ignored\n")

	files, err := findMarkdownFiles()
	require.NoError(t, err)

	require.ElementsMatch(t, []string{
		"README.md",
		"CONTRIBUTING.markdown",
		filepath.Join("docs", "guide.md"),
		filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"),
		"page.mdx",
		filepath.Join("docs", "component.mdx"),
	}, files)
}

func TestFindMarkdownFilesSkipsNestedGitCheckouts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeFile(t, "README.md", "# root\n")

	// A linked git worktree checked out anywhere in the repo must not be
	// scanned as part of it.
	writeFile(t, filepath.Join(".worktrees", "other-branch", ".git"), "gitdir: /elsewhere\n")
	writeFile(t, filepath.Join(".worktrees", "other-branch", "NOTES.md"), "ignored\n")

	files, err := findMarkdownFiles()
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"README.md"}, files)
}
