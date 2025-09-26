package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wayneashleyberry/gh-act/pkg/api"
	"gopkg.in/yaml.v3"
)

// MockGitHubAPI implements the GitHubAPI interface for testing.
type MockGitHubAPI struct {
	tags []api.Tag
	err  error
}

// NewMockGitHubAPI creates a new mock API client.
func NewMockGitHubAPI(tags []api.Tag, err error) *MockGitHubAPI {
	return &MockGitHubAPI{
		tags: tags,
		err:  err,
	}
}

func (m *MockGitHubAPI) FetchAllTags(_ context.Context, _, _ string) ([]api.Tag, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.tags, nil
}

func TestParseActionsInString(t *testing.T) {
	ctx := context.Background()

	// Mock tags for testing
	mockTags := []api.Tag{
		{
			Name: "v1.0.0",
			Commit: api.Commit{
				Sha: "abc123def456",
				URL: "https://api.github.com/repos/actions/checkout/git/commits/abc123def456",
			},
		},
		{
			Name: "v1.1.0",
			Commit: api.Commit{
				Sha: "def456ghi789",
				URL: "https://api.github.com/repos/actions/checkout/git/commits/def456ghi789",
			},
		},
	}

	mockAPI := NewMockGitHubAPI(mockTags, nil)

	tests := []struct {
		name         string
		yamlContent  string
		expectedLen  int
		expectedName string
	}{
		{
			name: "workflow with single action",
			yamlContent: `
name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v1.0.0
      - run: echo "Hello World"
`,
			expectedLen:  1,
			expectedName: "actions/checkout@v1.0.0",
		},
		{
			name: "workflow with multiple actions",
			yamlContent: `
name: Test Workflow  
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v1.0.0
      - uses: actions/setup-node@v1.1.0
      - run: npm test
`,
			expectedLen: 2,
		},
		{
			name: "empty workflow",
			yamlContent: `
name: Empty Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "No actions used"
`,
			expectedLen: 0,
		},
		{
			name: "workflow with local action (should be ignored)",
			yamlContent: `
name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./local-action
      - uses: actions/checkout@v1.0.0
`,
			expectedLen: 1,
		},
		{
			name: "workflow with subpath action",
			yamlContent: `
name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: gdcorp-actions/apps-change-orders/create@v1.0.0
      - run: echo "Testing subpath action"
`,
			expectedLen:  1,
			expectedName: "gdcorp-actions/apps-change-orders/create@v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseActionsInString(ctx, tt.yamlContent, "test-workflow.yml", mockAPI)
			require.NoError(t, err)
			require.Len(t, result, tt.expectedLen)

			if tt.expectedName != "" && len(result) > 0 {
				require.Equal(t, tt.expectedName, result[0].Node.Value)
			}
		})
	}
}

func TestDetectVersionStyle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected VersionStyle
		wantErr  bool
	}{
		{
			name:     "SHA1 hash",
			input:    "abc123def456abc123def456abc123def456ab12",
			expected: PinnedVersion,
			wantErr:  false,
		},
		{
			name:     "full semver",
			input:    "v1.2.3",
			expected: SemanticVersionFullyQualified,
			wantErr:  false,
		},
		{
			name:     "major.minor semver",
			input:    "v1.2",
			expected: SemanticVersionPartiallyQualified,
			wantErr:  false,
		},
		{
			name:     "major only semver",
			input:    "v1",
			expected: SemanticVersionMajorComponentOnly,
			wantErr:  false,
		},
		{
			name:    "invalid version",
			input:   "invalid-version",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := detectVersionStyle(tt.input)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseActionWithSubpath(t *testing.T) {
	ctx := context.Background()

	// Mock tags for testing
	mockTags := []api.Tag{
		{
			Name: "v1.0.0",
			Commit: api.Commit{
				Sha: "abc123def456abc123def456abc123def456ab12",
				URL: "https://api.github.com/repos/test/repo/git/commits/abc123",
			},
		},
	}

	mockAPI := NewMockGitHubAPI(mockTags, nil)

	tests := []struct {
		name            string
		actionValue     string
		expectedOwner   string
		expectedRepo    string
		expectedSubpath string
		expectedRef     string
	}{
		{
			name:            "action with subpath",
			actionValue:     "gdcorp-actions/apps-change-orders/create@v1.0.0",
			expectedOwner:   "gdcorp-actions",
			expectedRepo:    "apps-change-orders",
			expectedSubpath: "create",
			expectedRef:     "gdcorp-actions/apps-change-orders/create",
		},
		{
			name:            "action with deep subpath",
			actionValue:     "owner/repo/path/to/action@v1.0.0",
			expectedOwner:   "owner",
			expectedRepo:    "repo",
			expectedSubpath: "path/to/action",
			expectedRef:     "owner/repo/path/to/action",
		},
		{
			name:            "regular action without subpath",
			actionValue:     "actions/checkout@v1.0.0",
			expectedOwner:   "actions",
			expectedRepo:    "checkout",
			expectedSubpath: "",
			expectedRef:     "actions/checkout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := Action{
				FilePath: "test.yml",
				Node:     yaml.Node{Value: tt.actionValue},
			}

			result, err := parseAction(ctx, action, mockAPI)
			require.NoError(t, err)

			require.Equal(t, tt.expectedOwner, result.Owner)
			require.Equal(t, tt.expectedRepo, result.Repo)
			require.Equal(t, tt.expectedSubpath, result.Subpath)
			require.Equal(t, tt.expectedRef, result.ActionReference())
		})
	}
}
