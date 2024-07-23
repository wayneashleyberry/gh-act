package gh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/go-github/v50/github"
	"github.com/patrickmn/go-cache"
	"golang.org/x/oauth2"
)

var c *cache.Cache

func init() {
	c = cache.New(5*time.Minute, 10*time.Minute)
}

func NewClient(ctx context.Context) (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return github.NewClient(nil), errors.New("GITHUB_TOKEN environment variable is not set")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc), nil
}

func FetchAllTags(ctx context.Context, client *github.Client, owner, repo string) ([]*github.RepositoryTag, error) {
	cacheKey := fmt.Sprintf("github.tags.all.%s.%s", owner, repo)

	cachedTags, found := c.Get(cacheKey)
	if found {
		slog.Debug(fmt.Sprintf("using cache for %s/%s", owner, repo))

		return cachedTags.([]*github.RepositoryTag), nil
	}

	var allTags []*github.RepositoryTag

	opt := &github.ListOptions{PerPage: 100}

	for {
		tags, resp, err := client.Repositories.ListTags(ctx, owner, repo, opt)
		if err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}

		allTags = append(allTags, tags...)

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	c.Set(cacheKey, allTags, cache.DefaultExpiration)

	return allTags, nil
}
