package cmd

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wayneashleyberry/gh-act/pkg/api"
	"gopkg.in/yaml.v3"
)

// MockGitHubAPI implements the GitHubAPI interface for testing.
type MockGitHubAPI struct {
	tags       []api.Tag
	repository *api.Repository // single repository for testing
	err        error
}

// NewMockGitHubAPI creates a new mock API client.
func NewMockGitHubAPI(tags []api.Tag, err error) *MockGitHubAPI {
	return &MockGitHubAPI{
		tags:       tags,
		repository: nil,
		err:        err,
	}
}

// SetRepository sets the mock repository for testing.
func (m *MockGitHubAPI) SetRepository(repo *api.Repository) {
	m.repository = repo
}

func (m *MockGitHubAPI) FetchAllTags(_ context.Context, _, _ string) ([]api.Tag, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.tags, nil
}

func (m *MockGitHubAPI) FetchRepository(_ context.Context, owner, repo string) (*api.Repository, error) {
	if m.err != nil {
		return nil, m.err
	}

	// Use the explicitly set repository if available
	if m.repository != nil {
		return m.repository, nil
	}

	// Return a default repository with main as default branch
	return &api.Repository{
		Name:          repo,
		FullName:      owner + "/" + repo,
		DefaultBranch: "main",
		Private:       false,
	}, nil
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
      - uses: aws-actions/configure-aws-credentials/setup@v1.0.0
      - run: echo "Testing subpath action"
`,
			expectedLen:  1,
			expectedName: "aws-actions/configure-aws-credentials/setup@v1.0.0",
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
			name:     "branch name main",
			input:    "main",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "branch name master",
			input:    "master",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "branch name develop",
			input:    "develop",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "feature branch",
			input:    "feature/branch-name",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "release branch",
			input:    "release/v1.2.3",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "hyphenated branch name",
			input:    "invalid-version",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "string with spaces",
			input:   "branch name",
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
			actionValue:     "aws-actions/configure-aws-credentials/setup@v1.0.0",
			expectedOwner:   "aws-actions",
			expectedRepo:    "configure-aws-credentials",
			expectedSubpath: "setup",
			expectedRef:     "aws-actions/configure-aws-credentials/setup",
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

func TestBranchReferenceDetection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected VersionStyle
		wantErr  bool
	}{
		{
			name:     "main branch",
			input:    "main",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "master branch",
			input:    "master",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "develop branch",
			input:    "develop",
			expected: BranchReference,
			wantErr:  false,
		},
		{
			name:     "feature branch",
			input:    "feature/new-feature",
			expected: BranchReference,
			wantErr:  false,
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

func TestBranchReferenceNewVersionString(t *testing.T) {
	tag := &api.Tag{
		Name: "v2.1.0",
		Commit: api.Commit{
			Sha: "abc123def456abc123def456abc123def456ab12",
			URL: "https://api.github.com/repos/test/repo/git/commits/abc123",
		},
	}

	parsedAction := ParsedAction{
		VersionStyle:      BranchReference,
		CurrentVersionTag: tag,
		LatestVersionTag:  tag,
		PinVersionTag:     tag,
	}

	newVersionString, err := parsedAction.NewVersionString()
	require.NoError(t, err)
	require.Equal(t, "v2.1.0", newVersionString)
}

func TestParseBranchReference(t *testing.T) {
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
		{
			Name: "v2.0.0",
			Commit: api.Commit{
				Sha: "def456ghi789def456ghi789def456ghi789de45",
				URL: "https://api.github.com/repos/test/repo/git/commits/def456",
			},
		},
	}

	tests := []struct {
		name                 string
		actionValue          string
		repositoryBranch     string
		expectedLatestTag    string
		expectedVersionStyle VersionStyle
		expectError          bool
		errorSubstring       string
	}{
		{
			name:                 "main branch reference (default branch)",
			actionValue:          "test/repo@main",
			repositoryBranch:     "main",
			expectedLatestTag:    "v2.0.0", // latest tag
			expectedVersionStyle: BranchReference,
			expectError:          false,
		},
		{
			name:             "non-default branch reference",
			actionValue:      "test/repo@develop",
			repositoryBranch: "main", // develop is not the default branch
			expectError:      true,
			errorSubstring:   "not the default branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := NewMockGitHubAPI(mockTags, nil)

			// Set up mock repository with the specified default branch
			mockRepo := &api.Repository{
				Name:          "repo",
				FullName:      "test/repo",
				DefaultBranch: tt.repositoryBranch,
				Private:       false,
			}
			mockAPI.SetRepository(mockRepo)

			action := Action{
				FilePath: "test.yml",
				Node:     yaml.Node{Value: tt.actionValue},
			}

			result, err := parseAction(ctx, action, mockAPI)

			if tt.expectError {
				require.Error(t, err)

				if tt.errorSubstring != "" {
					require.Contains(t, err.Error(), tt.errorSubstring)
				}

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedVersionStyle, result.VersionStyle)
			require.Equal(t, tt.expectedLatestTag, result.LatestVersionTag.GetName())

			// Branch references should always be considered outdated (for pinning)
			outdated, err := result.IsOutdated()
			require.NoError(t, err)
			require.True(t, outdated)
		})
	}
}

func TestBranchReferenceEdgeCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		tags        []api.Tag
		apiError    error
		actionValue string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "repository with no tags",
			tags:        []api.Tag{}, // no tags
			actionValue: "test/repo@main",
			expectError: true,
			errorMsg:    "cannot resolve branch reference to a version",
		},
		{
			name:        "API error",
			tags:        nil,
			apiError:    errors.New("API connection failed"),
			actionValue: "test/repo@main",
			expectError: true,
			errorMsg:    "fetch repository info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := NewMockGitHubAPI(tt.tags, tt.apiError)

			action := Action{
				FilePath: "test.yml",
				Node:     yaml.Node{Value: tt.actionValue},
			}

			_, err := parseAction(ctx, action, mockAPI)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateFileCommentHandling(t *testing.T) {
	ctx := context.Background()

	// Create a temporary test file with one action that has a comment and one without
	testContent := `name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2 # v2.0.0
      - uses: actions/setup-node@v3
      - run: echo "test"
`

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-workflow-*.yml")
	require.NoError(t, err)

	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(testContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Mock tags for testing - simulate updating from v2 to v4 for checkout
	checkoutTags := []api.Tag{
		{
			Name: "v2.0.0",
			Commit: api.Commit{
				Sha: "abc123def456abc123def456abc123def456ab12",
				URL: "https://api.github.com/repos/actions/checkout/git/commits/abc123",
			},
		},
		{
			Name: "v4.0.0",
			Commit: api.Commit{
				Sha: "def456ghi789def456ghi789def456ghi789de45",
				URL: "https://api.github.com/repos/actions/checkout/git/commits/def456",
			},
		},
	}

	// Test checkout action (has existing comment)
	mockAPI := NewMockGitHubAPI(checkoutTags, nil)
	updates, err := parseActionsInString(ctx, testContent, tmpFile.Name(), mockAPI)
	require.NoError(t, err)

	// Filter to just checkout action
	var checkoutUpdate ParsedAction

	for _, update := range updates {
		if update.Repo == "checkout" {
			checkoutUpdate = update

			break
		}
	}

	require.NotEmpty(t, checkoutUpdate.Node.Value)

	// Update the file
	err = updateFile(tmpFile.Name(), []ParsedAction{checkoutUpdate})
	require.NoError(t, err)

	// Read the updated content
	updatedContent, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)

	updatedStr := string(updatedContent)

	// Verify that the comment was updated to reflect the new version
	require.Contains(t, updatedStr, "uses: actions/checkout@v4 # v4.0.0")
	require.NotContains(t, updatedStr, "# v2.0.0") // old comment should be gone

	// Test that actions without comments also get comments added
	setupNodeTags := []api.Tag{
		{
			Name: "v3.0.0",
			Commit: api.Commit{
				Sha: "ghi789jkl012ghi789jkl012ghi789jkl012gh78",
				URL: "https://api.github.com/repos/actions/setup-node/git/commits/ghi789",
			},
		},
		{
			Name: "v4.1.0",
			Commit: api.Commit{
				Sha: "jkl012mno345jkl012mno345jkl012mno345jk01",
				URL: "https://api.github.com/repos/actions/setup-node/git/commits/jkl012",
			},
		},
	}

	// Create a new test file for setup-node
	testContent2 := `name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-node@v3
      - run: echo "test"
`

	tmpFile2, err := os.CreateTemp("", "test-workflow2-*.yml")
	require.NoError(t, err)

	defer os.Remove(tmpFile2.Name())

	_, err = tmpFile2.WriteString(testContent2)
	require.NoError(t, err)
	tmpFile2.Close()

	mockAPI2 := NewMockGitHubAPI(setupNodeTags, nil)
	updates2, err := parseActionsInString(ctx, testContent2, tmpFile2.Name(), mockAPI2)
	require.NoError(t, err)
	require.Len(t, updates2, 1)

	// Update the file
	err = updateFile(tmpFile2.Name(), updates2)
	require.NoError(t, err)

	// Read the updated content
	updatedContent2, err := os.ReadFile(tmpFile2.Name())
	require.NoError(t, err)

	updatedStr2 := string(updatedContent2)

	// Verify that a comment was added even though there wasn't one before
	require.Contains(t, updatedStr2, "uses: actions/setup-node@v4 # v4.1.0")
}
