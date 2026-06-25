package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// MockGitHubAPI implements the GitHubAPI interface for testing.
type MockGitHubAPI struct {
	tags         map[string][]Tag       // keyed by "owner/repo"
	repositories map[string]*Repository // keyed by "owner/repo"
	err          error
}

// NewMockGitHubAPI creates a new mock API client.
func NewMockGitHubAPI() *MockGitHubAPI {
	return &MockGitHubAPI{
		tags:         make(map[string][]Tag),
		repositories: make(map[string]*Repository),
	}
}

// SetTags sets the mock tags for a specific repository.
func (m *MockGitHubAPI) SetTags(owner, repo string, tags []Tag) {
	key := owner + "/" + repo
	m.tags[key] = tags
}

// SetRepository sets the mock repository info for a specific repository.
func (m *MockGitHubAPI) SetRepository(owner, repo string, repository *Repository) {
	key := owner + "/" + repo
	m.repositories[key] = repository
}

// SetError sets an error to be returned by FetchAllTags and FetchRepository.
func (m *MockGitHubAPI) SetError(err error) {
	m.err = err
}

func (m *MockGitHubAPI) FetchAllTags(_ context.Context, owner, repo string) ([]Tag, error) {
	if m.err != nil {
		return nil, m.err
	}

	key := owner + "/" + repo
	if tags, exists := m.tags[key]; exists {
		return tags, nil
	}

	return []Tag{}, nil
}

func (m *MockGitHubAPI) FetchRepository(_ context.Context, owner, repo string) (*Repository, error) {
	if m.err != nil {
		return nil, m.err
	}

	key := owner + "/" + repo
	if repository, exists := m.repositories[key]; exists {
		return repository, nil
	}

	// Return a default repository with main as default branch if not explicitly set
	return &Repository{
		Name:          repo,
		FullName:      owner + "/" + repo,
		DefaultBranch: "main",
		Private:       false,
	}, nil
}

func TestMockGitHubAPI(t *testing.T) {
	ctx := context.Background()

	mock := NewMockGitHubAPI()

	// Test empty response
	tags, err := mock.FetchAllTags(ctx, "owner", "repo")
	require.NoError(t, err)
	require.Empty(t, tags)

	// Test with mock data
	expectedTags := []Tag{
		{Name: "v1.0.0", Commit: Commit{Sha: "abc123", URL: "https://example.com"}},
		{Name: "v1.1.0", Commit: Commit{Sha: "def456", URL: "https://example.com"}},
	}

	mock.SetTags("owner", "repo", expectedTags)

	tags, err = mock.FetchAllTags(ctx, "owner", "repo")
	require.NoError(t, err)
	require.Len(t, tags, 2)

	// Test error case
	expectedError := errors.New("API error")
	mock.SetError(expectedError)

	_, err = mock.FetchAllTags(ctx, "owner", "repo")
	require.ErrorIs(t, err, expectedError)
}

func TestClientImplementsInterface(_ *testing.T) {
	// Verify the real client implements the interface without requiring
	// authentication to construct one.
	var _ GitHubAPI = (*Client)(nil)
}

func TestNextPagePath(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "empty header",
			header: "",
			want:   "",
		},
		{
			name:   "next and last",
			header: `<https://api.github.com/repos/o/r/tags?page=2>; rel="next", <https://api.github.com/repos/o/r/tags?page=5>; rel="last"`,
			want:   "https://api.github.com/repos/o/r/tags?page=2",
		},
		{
			name:   "only prev and first (no next)",
			header: `<https://api.github.com/repos/o/r/tags?page=1>; rel="prev", <https://api.github.com/repos/o/r/tags?page=1>; rel="first"`,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, nextPagePath(tt.header))
		})
	}
}

func TestRepositoryMockFunctionality(t *testing.T) {
	ctx := context.Background()

	mock := NewMockGitHubAPI()

	// Test default repository response
	repo, err := mock.FetchRepository(ctx, "owner", "repo")
	require.NoError(t, err)
	require.Equal(t, "repo", repo.Name)
	require.Equal(t, "owner/repo", repo.FullName)
	require.Equal(t, "main", repo.DefaultBranch)
	require.False(t, repo.Private)

	// Test with custom repository
	customRepo := &Repository{
		Name:          "custom",
		FullName:      "owner/custom",
		DefaultBranch: "develop",
		Private:       true,
	}
	mock.SetRepository("owner", "custom", customRepo)

	repo, err = mock.FetchRepository(ctx, "owner", "custom")
	require.NoError(t, err)
	require.Equal(t, "custom", repo.Name)
	require.Equal(t, "owner/custom", repo.FullName)
	require.Equal(t, "develop", repo.DefaultBranch)
	require.True(t, repo.Private)

	// Test error case
	expectedError := errors.New("Repository API error")
	mock.SetError(expectedError)

	_, err = mock.FetchRepository(ctx, "owner", "repo")
	require.ErrorIs(t, err, expectedError)
}
