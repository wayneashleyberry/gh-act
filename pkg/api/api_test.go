package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// MockGitHubAPI implements the GitHubAPI interface for testing.
type MockGitHubAPI struct {
	tags map[string][]Tag // keyed by "owner/repo"
	err  error
}

// NewMockGitHubAPI creates a new mock API client.
func NewMockGitHubAPI() *MockGitHubAPI {
	return &MockGitHubAPI{
		tags: make(map[string][]Tag),
	}
}

// SetTags sets the mock tags for a specific repository.
func (m *MockGitHubAPI) SetTags(owner, repo string, tags []Tag) {
	key := owner + "/" + repo
	m.tags[key] = tags
}

// SetError sets an error to be returned by FetchAllTags.
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

func TestRealGitHubAPI(_ *testing.T) {
	// This test just verifies the real API implements the interface correctly
	api := NewRealGitHubAPI()

	var _ GitHubAPI = api // compile-time interface check
}
