package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/patrickmn/go-cache"
)

var c *cache.Cache

func init() {
	c = cache.New(5*time.Minute, 10*time.Minute)
}

// Repository represents basic repository information from GitHub API.
type Repository struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// GitHubAPI defines the interface for GitHub API operations.
type GitHubAPI interface {
	FetchAllTags(ctx context.Context, owner, repo string) ([]Tag, error)
	FetchRepository(ctx context.Context, owner, repo string) (*Repository, error)
}

// RealGitHubAPI implements GitHubAPI using the actual GitHub API.
type RealGitHubAPI struct{}

// NewRealGitHubAPI creates a new instance of RealGitHubAPI.
func NewRealGitHubAPI() *RealGitHubAPI {
	return &RealGitHubAPI{}
}

type Commit struct {
	Sha string `json:"sha"`
	URL string `json:"url"`
}

type Tag struct {
	Name       string `json:"name"`
	ZipballURL string `json:"zipball_url"`
	TarballURL string `json:"tarball_url"`
	Commit     Commit `json:"commit"`
	NodeID     string `json:"node_id"`
}

func (t *Tag) GetName() string {
	if t != nil {
		return t.Name
	}

	return ""
}

func (c *Commit) GetSHA() string {
	if c != nil {
		return c.Sha
	}

	return ""
}

func (r *RealGitHubAPI) FetchAllTags(ctx context.Context, owner, repo string) ([]Tag, error) {
	return FetchAllTags(ctx, owner, repo)
}

func (r *RealGitHubAPI) FetchRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	return FetchRepository(ctx, owner, repo)
}

func FetchAllTags(ctx context.Context, owner, repo string) ([]Tag, error) {
	cacheKey := fmt.Sprintf("github.tags.all.%s.%s", owner, repo)

	cachedTags, found := c.Get(cacheKey)
	if found {
		slog.Debug(fmt.Sprintf("using cache for %s/%s", owner, repo))

		return cachedTags.([]Tag), nil
	}

	path := fmt.Sprintf("/repos/%s/%s/tags", owner, repo)

	b, _, err := gh.ExecContext(ctx, "api", path, "--paginate")
	if err != nil {
		return []Tag{}, err
	}

	tags := []Tag{}

	err = json.Unmarshal(b.Bytes(), &tags)
	if err != nil {
		return []Tag{}, err
	}

	c.Set(cacheKey, tags, cache.DefaultExpiration)

	return tags, nil
}

func FetchRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	cacheKey := fmt.Sprintf("github.repository.%s.%s", owner, repo)

	cachedRepo, found := c.Get(cacheKey)
	if found {
		slog.Debug(fmt.Sprintf("using cache for repository %s/%s", owner, repo))

		return cachedRepo.(*Repository), nil
	}

	path := fmt.Sprintf("/repos/%s/%s", owner, repo)

	b, _, err := gh.ExecContext(ctx, "api", path)
	if err != nil {
		return nil, fmt.Errorf("fetch repository info: %w", err)
	}

	var repository Repository

	err = json.Unmarshal(b.Bytes(), &repository)
	if err != nil {
		return nil, fmt.Errorf("unmarshal repository info: %w", err)
	}

	c.Set(cacheKey, &repository, cache.DefaultExpiration)

	return &repository, nil
}
