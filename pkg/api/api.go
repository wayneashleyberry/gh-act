// Package api provides a thin, cached client over the GitHub REST API for the
// tags and repository metadata that gh-act needs to resolve action versions.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/cli/go-gh/v2/pkg/api"
	"golang.org/x/sync/singleflight"
)

// perPage is the maximum page size GitHub allows for list endpoints.
const perPage = 100

// Repository represents basic repository information from the GitHub API.
type Repository struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// Commit is the git commit a tag points at.
type Commit struct {
	Sha string `json:"sha"`
	URL string `json:"url"`
}

// Tag is a single git tag as returned by the GitHub tags endpoint.
type Tag struct {
	Name       string `json:"name"`
	ZipballURL string `json:"zipball_url"`
	TarballURL string `json:"tarball_url"`
	Commit     Commit `json:"commit"`
	NodeID     string `json:"node_id"`
}

// GetName returns the tag name, or an empty string if the tag is nil.
func (t *Tag) GetName() string {
	if t != nil {
		return t.Name
	}

	return ""
}

// GetSHA returns the commit SHA, or an empty string if the commit is nil.
func (c *Commit) GetSHA() string {
	if c != nil {
		return c.Sha
	}

	return ""
}

// GitHubAPI defines the GitHub operations gh-act depends on. It is satisfied by
// Client and can be replaced with a fake in tests.
type GitHubAPI interface {
	FetchAllTags(ctx context.Context, owner, repo string) ([]Tag, error)
	FetchRepository(ctx context.Context, owner, repo string) (*Repository, error)
}

// Client is a concurrency-safe GitHub API client. Identical requests issued
// during a single run are de-duplicated via singleflight and memoised in
// process, so resolving the same action across many workflow files only hits
// the network once.
type Client struct {
	rest *api.RESTClient

	group singleflight.Group

	mu        sync.Mutex
	tagCache  map[string][]Tag
	repoCache map[string]*Repository
}

// NewClient creates a Client backed by the user's existing gh authentication.
func NewClient() (*Client, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("create REST client: %w", err)
	}

	return &Client{
		rest:      rest,
		tagCache:  make(map[string][]Tag),
		repoCache: make(map[string]*Repository),
	}, nil
}

// FetchAllTags returns every tag for owner/repo, following pagination. Results
// are cached and de-duplicated for the lifetime of the Client.
func (c *Client) FetchAllTags(ctx context.Context, owner, repo string) ([]Tag, error) {
	key := owner + "/" + repo

	c.mu.Lock()
	cached, ok := c.tagCache[key]
	c.mu.Unlock()

	if ok {
		return cached, nil
	}

	result, err, _ := c.group.Do("tags:"+key, func() (any, error) {
		tags, err := c.fetchAllTags(ctx, owner, repo)
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		c.tagCache[key] = tags
		c.mu.Unlock()

		return tags, nil
	})
	if err != nil {
		return nil, err
	}

	tags, ok := result.([]Tag)
	if !ok {
		return nil, fmt.Errorf("unexpected cache type for %s", key)
	}

	return tags, nil
}

// FetchRepository returns metadata for owner/repo, cached for the lifetime of
// the Client.
func (c *Client) FetchRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	key := owner + "/" + repo

	c.mu.Lock()
	cached, ok := c.repoCache[key]
	c.mu.Unlock()

	if ok {
		return cached, nil
	}

	result, err, _ := c.group.Do("repo:"+key, func() (any, error) {
		repository, err := c.fetchRepository(ctx, owner, repo)
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		c.repoCache[key] = repository
		c.mu.Unlock()

		return repository, nil
	})
	if err != nil {
		return nil, err
	}

	repository, ok := result.(*Repository)
	if !ok {
		return nil, fmt.Errorf("unexpected cache type for %s", key)
	}

	return repository, nil
}

func (c *Client) fetchAllTags(ctx context.Context, owner, repo string) ([]Tag, error) {
	var tags []Tag

	path := fmt.Sprintf("repos/%s/%s/tags?per_page=%d", owner, repo, perPage)

	for path != "" {
		resp, err := c.rest.RequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("fetch tags for %s/%s: %w", owner, repo, err)
		}

		var page []Tag

		err = json.NewDecoder(resp.Body).Decode(&page)

		closeErr := resp.Body.Close()

		if err != nil {
			return nil, fmt.Errorf("decode tags for %s/%s: %w", owner, repo, err)
		}

		if closeErr != nil {
			return nil, fmt.Errorf("close response body for %s/%s: %w", owner, repo, closeErr)
		}

		tags = append(tags, page...)
		path = nextPagePath(resp.Header.Get("Link"))
	}

	return tags, nil
}

func (c *Client) fetchRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	path := fmt.Sprintf("repos/%s/%s", owner, repo)

	var repository Repository

	err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &repository)
	if err != nil {
		return nil, fmt.Errorf("fetch repository info for %s/%s: %w", owner, repo, err)
	}

	return &repository, nil
}

// nextPagePath extracts the rel="next" URL from a GitHub Link header. It
// returns an empty string when there is no further page.
func nextPagePath(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	for _, link := range strings.Split(linkHeader, ",") {
		sections := strings.Split(link, ";")
		if len(sections) < 2 {
			continue
		}

		url := strings.TrimSpace(sections[0])
		url = strings.TrimPrefix(url, "<")
		url = strings.TrimSuffix(url, ">")

		for _, section := range sections[1:] {
			if strings.TrimSpace(section) == `rel="next"` {
				return url
			}
		}
	}

	return ""
}
