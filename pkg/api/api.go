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
