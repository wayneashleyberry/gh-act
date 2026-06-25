// Package cmd implements the gh-act subcommands: discovering, resolving,
// reporting and rewriting the GitHub Actions referenced by a repository.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/wayneashleyberry/gh-act/pkg/api"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

// resolveConcurrency bounds how many actions are resolved against the GitHub
// API at once.
const resolveConcurrency = 8

// maxBranchNameLength caps how long a ref may be before we stop treating it as
// a plausible branch name.
const maxBranchNameLength = 100

var (
	// sha1Regex matches a full 40-character git SHA-1.
	sha1Regex = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
	// fullSemverRegex matches a fully qualified semantic version (major.minor.patch).
	fullSemverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$`)
	// minorSemverRegex matches a semantic version with major and minor components (major.minor).
	minorSemverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$`)
	// majorSemverRegex matches a semantic version with only a major component (major).
	majorSemverRegex = regexp.MustCompile(`^v?(\d+)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$`)
	// branchNameRegex matches common git branch naming conventions.
	branchNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)
	// pathSegmentRegex matches a single safe owner/repo/subpath segment.
	pathSegmentRegex = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// ErrCurrentVersionUnmatched is returned when a referenced version cannot be
// matched to any tag or release on GitHub.
var ErrCurrentVersionUnmatched = errors.New("could not match version tag to a tag or release on github (might be tagged to a commit)")

// VersionStyle describes how an action's version was expressed in YAML.
type VersionStyle string

const (
	PinnedVersion                     = VersionStyle("pinned")
	SemanticVersionFullyQualified     = VersionStyle("semver")
	SemanticVersionPartiallyQualified = VersionStyle("semver-partial")
	SemanticVersionMajorComponentOnly = VersionStyle("semver-major")
	BranchReference                   = VersionStyle("branch")
)

// ParsedAction is an Action that has been resolved against the GitHub API.
type ParsedAction struct {
	FilePath          string
	Node              yaml.Node
	Owner, Repo       string
	Subpath           string
	RawVersionString  string
	CurrentVersionTag *api.Tag
	LatestVersionTag  *api.Tag
	PinVersionTag     *api.Tag
	VersionStyle      VersionStyle
}

// NewVersionString renders the latest version in the same style the action was
// originally written (e.g. v4, v4.1 or v4.1.2).
func (p ParsedAction) NewVersionString() (string, error) {
	if p.LatestVersionTag == nil {
		return "", errors.New("latest version tag is nil")
	}

	newVersion, err := semver.NewVersion(p.LatestVersionTag.GetName())
	if err != nil {
		return "", fmt.Errorf("new version from latest version tag: %w", err)
	}

	switch p.VersionStyle {
	case BranchReference:
		return p.LatestVersionTag.GetName(), nil
	case SemanticVersionFullyQualified:
		return newVersion.String(), nil
	case SemanticVersionPartiallyQualified:
		return fmt.Sprintf("v%d.%d", newVersion.Major(), newVersion.Minor()), nil
	case SemanticVersionMajorComponentOnly:
		return fmt.Sprintf("v%d", newVersion.Major()), nil
	case PinnedVersion:
		return p.LatestVersionTag.GetName(), nil
	default:
		return "", fmt.Errorf("unsupported version style: %s", p.VersionStyle)
	}
}

// ActionReference returns the owner/repo(/subpath) reference without a version.
func (p ParsedAction) ActionReference() string {
	if p.Subpath != "" {
		return fmt.Sprintf("%s/%s/%s", p.Owner, p.Repo, p.Subpath)
	}

	return fmt.Sprintf("%s/%s", p.Owner, p.Repo)
}

// IsOutdated reports whether a newer version than the one referenced exists.
func (p ParsedAction) IsOutdated() (bool, error) {
	if p.CurrentVersionTag == nil {
		return false, fmt.Errorf("checking if parsed action is outdated, but current version tag is nil: %s", p.Node.Value)
	}

	if p.LatestVersionTag == nil {
		return false, fmt.Errorf("checking if parsed action is outdated, but latest version tag is nil: %s", p.Node.Value)
	}

	// Branch references should always be pinned, so treat them as outdated.
	if p.VersionStyle == BranchReference {
		return true, nil
	}

	constraint := "> " + p.RawVersionString
	if p.VersionStyle == PinnedVersion {
		constraint = "> " + p.CurrentVersionTag.GetName()
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, fmt.Errorf("new constraint: %w", err)
	}

	latest, err := semver.NewVersion(p.LatestVersionTag.GetName())
	if err != nil {
		return false, fmt.Errorf("new version: %w", err)
	}

	return c.Check(latest), nil
}

// parseActionsInString parses and resolves every action reference in a single
// YAML document.
func parseActionsInString(ctx context.Context, yamlContent, filePath string, client api.GitHubAPI) ([]ParsedAction, error) {
	refs, err := parseActionRefs([]byte(yamlContent), filePath)
	if err != nil {
		return nil, err
	}

	return resolveActions(ctx, refs, client)
}

// resolveActions resolves every reference concurrently, preserving input order.
// References that cannot be resolved are logged at debug level and skipped so a
// single unusual action never aborts the whole run. Context cancellation is
// propagated.
func resolveActions(ctx context.Context, refs []Action, client api.GitHubAPI) ([]ParsedAction, error) {
	resolved := make([]*ParsedAction, len(refs))

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(resolveConcurrency)

	for i, ref := range refs {
		if !isPinnableRef(ref.Node.Value) {
			slog.Debug("ignoring non-pinnable action", slog.String("value", ref.Node.Value))

			continue
		}

		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("resolve actions: %w", err)
			}

			parsed, err := resolveAction(ctx, ref, client)
			if err != nil {
				if ctx.Err() != nil {
					return fmt.Errorf("resolve %s: %w", ref.Node.Value, err)
				}

				slog.Debug("problem parsing action", slog.String("action", ref.Node.Value), slog.String("error.message", err.Error()))

				return nil
			}

			resolved[i] = &parsed

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	actions := make([]ParsedAction, 0, len(resolved))

	for _, action := range resolved {
		if action != nil {
			actions = append(actions, *action)
		}
	}

	return actions, nil
}

// resolveAction parses an action reference and resolves its current, latest and
// pin target tags against the GitHub API.
func resolveAction(ctx context.Context, action Action, client api.GitHubAPI) (ParsedAction, error) {
	parts := strings.Split(action.Node.Value, "@")
	if len(parts) != 2 {
		return ParsedAction{}, fmt.Errorf("expected exactly one '@' in reference: %s", action.Node.Value)
	}

	subParts := strings.Split(parts[0], "/")
	if len(subParts) < 2 {
		return ParsedAction{}, fmt.Errorf("expected owner/repo in reference: %s", action.Node.Value)
	}

	parsed := ParsedAction{
		FilePath:         action.FilePath,
		Node:             action.Node,
		RawVersionString: parts[1],
		Owner:            subParts[0],
		Repo:             subParts[1],
	}

	if len(subParts) > 2 {
		parsed.Subpath = strings.Join(subParts[2:], "/")
	}

	if err := validateReference(parsed); err != nil {
		return ParsedAction{}, err
	}

	style, err := detectVersionStyle(parsed.RawVersionString)
	if err != nil {
		return ParsedAction{}, fmt.Errorf("could not determine version style: %w", err)
	}

	parsed.VersionStyle = style

	if style == BranchReference {
		return resolveBranchReference(ctx, parsed, client)
	}

	tags, err := client.FetchAllTags(ctx, parsed.Owner, parsed.Repo)
	if err != nil {
		return ParsedAction{}, fmt.Errorf("fetch all tags: %w", err)
	}

	if len(tags) == 0 {
		return ParsedAction{}, fmt.Errorf("action has no tags: %s/%s", parsed.Owner, parsed.Repo)
	}

	matchedTag, err := matchVersionToTag(parsed.RawVersionString, parsed.VersionStyle, tags, parsed.ActionReference())
	if err != nil {
		return ParsedAction{}, fmt.Errorf("problem matching version to a tag: %w", err)
	}

	parsed.CurrentVersionTag = matchedTag

	currentVersion, err := semver.NewVersion(matchedTag.GetName())
	if err != nil {
		return ParsedAction{}, fmt.Errorf("parse semantic version: %s: %w", matchedTag.GetName(), err)
	}

	latestTag, pinTag, err := selectLatestAndPin(parsed, currentVersion, tags)
	if err != nil {
		return ParsedAction{}, err
	}

	parsed.LatestVersionTag = latestTag
	parsed.PinVersionTag = pinTag

	return parsed, nil
}

// selectLatestAndPin walks the tag list once and returns the absolute latest
// non-prerelease version and the highest version that satisfies the pin
// constraint for the action's version style.
func selectLatestAndPin(parsed ParsedAction, currentVersion *semver.Version, tags []api.Tag) (*api.Tag, *api.Tag, error) {
	nextVersion := nextVersionForStyle(parsed.VersionStyle, currentVersion)

	pinConstraint, err := semver.NewConstraint(">=" + currentVersion.String() + " <" + nextVersion.String())
	if err != nil {
		return nil, nil, fmt.Errorf("new constraint: %w", err)
	}

	var (
		latestVersion *semver.Version
		latestTag     *api.Tag
		pinVersion    = currentVersion
		pinTag        = parsed.CurrentVersionTag
	)

	for i := range tags {
		tag := &tags[i]

		tagVersion, err := semver.NewVersion(tag.GetName())
		if err != nil {
			slog.Debug(
				"could not parse tag name as semantic version",
				slog.String("action", parsed.ActionReference()),
				slog.String("tag.name", tag.GetName()),
				slog.String("error.message", err.Error()),
			)

			continue
		}

		if tagVersion.Prerelease() != "" {
			continue
		}

		if latestVersion == nil || tagVersion.GreaterThan(latestVersion) {
			latestVersion = tagVersion
			latestTag = tag
		}

		if tagVersion.GreaterThan(pinVersion) && pinConstraint.Check(tagVersion) {
			pinVersion = tagVersion
			pinTag = tag
		}
	}

	if latestTag == nil {
		return nil, nil, fmt.Errorf("no non-prerelease semantic version tags found for %s", parsed.ActionReference())
	}

	return latestTag, pinTag, nil
}

// nextVersionForStyle returns the exclusive upper bound used to keep a pin
// within the same precision the action was written at.
func nextVersionForStyle(style VersionStyle, current *semver.Version) *semver.Version {
	switch style {
	case SemanticVersionFullyQualified:
		next := current.IncPatch()

		return &next
	case SemanticVersionPartiallyQualified:
		next := current.IncMinor()

		return &next
	case SemanticVersionMajorComponentOnly:
		next := current.IncMajor()

		return &next
	case PinnedVersion, BranchReference:
		return current
	default:
		return current
	}
}

// resolveBranchReference handles actions referenced by a branch. Only the
// repository's default branch is allowed, and it is resolved to the latest
// tagged version.
func resolveBranchReference(ctx context.Context, parsed ParsedAction, client api.GitHubAPI) (ParsedAction, error) {
	repo, err := client.FetchRepository(ctx, parsed.Owner, parsed.Repo)
	if err != nil {
		return ParsedAction{}, fmt.Errorf("fetch repository info: %w", err)
	}

	if parsed.RawVersionString != repo.DefaultBranch {
		return ParsedAction{}, fmt.Errorf("branch reference %q is not the default branch (%q); only default branch references are supported", parsed.RawVersionString, repo.DefaultBranch)
	}

	slog.Debug("detected default branch reference", slog.String("action", parsed.ActionReference()), slog.String("branch", parsed.RawVersionString))

	tags, err := client.FetchAllTags(ctx, parsed.Owner, parsed.Repo)
	if err != nil {
		return ParsedAction{}, fmt.Errorf("fetch all tags: %w", err)
	}

	if len(tags) == 0 {
		return ParsedAction{}, fmt.Errorf("action has no tags: %s/%s (cannot resolve branch reference to a version)", parsed.Owner, parsed.Repo)
	}

	var (
		latestVersion *semver.Version
		latestTag     *api.Tag
	)

	for i := range tags {
		tag := &tags[i]

		tagVersion, err := semver.NewVersion(tag.GetName())
		if err != nil {
			continue
		}

		if tagVersion.Prerelease() != "" {
			continue
		}

		if latestVersion == nil || tagVersion.GreaterThan(latestVersion) {
			latestVersion = tagVersion
			latestTag = tag
		}
	}

	if latestTag == nil {
		return ParsedAction{}, fmt.Errorf("no valid semantic version tags found for %s/%s", parsed.Owner, parsed.Repo)
	}

	parsed.CurrentVersionTag = latestTag
	parsed.LatestVersionTag = latestTag
	parsed.PinVersionTag = latestTag

	return parsed, nil
}

// matchVersionToTag resolves the raw version reference to the specific tag it
// points at.
func matchVersionToTag(rawVersion string, style VersionStyle, tags []api.Tag, actionRef string) (*api.Tag, error) {
	if style == BranchReference {
		return nil, errors.New("branch references must be handled by resolveBranchReference")
	}

	if style == PinnedVersion {
		return matchPinnedTag(rawVersion, tags, actionRef)
	}

	currentVersion, err := semver.NewVersion(rawVersion)
	if err != nil {
		return nil, fmt.Errorf("parse raw version string: %s: %w", rawVersion, err)
	}

	for i := range tags {
		tagVersion, err := semver.NewVersion(tags[i].GetName())
		if err != nil {
			continue
		}

		if currentVersion.Equal(tagVersion) {
			return &tags[i], nil
		}
	}

	return nil, ErrCurrentVersionUnmatched
}

// matchPinnedTag resolves a pinned SHA to its tag. When several tags point at
// the same commit (e.g. v7 and v7.0.1) the most specific is chosen: the highest
// semantic version, falling back to the longest tag name for non-semver tags.
func matchPinnedTag(sha string, tags []api.Tag, actionRef string) (*api.Tag, error) {
	var matches []*api.Tag

	for i := range tags {
		if sha == tags[i].Commit.GetSHA() {
			matches = append(matches, &tags[i])
		}
	}

	switch len(matches) {
	case 0:
		return nil, errors.New("pinned to a commit hash with no associated tag")
	case 1:
		return matches[0], nil
	}

	slog.Debug("matched pinned commit hash to more than one tag", slog.String("action", actionRef), slog.String("version", sha))

	var (
		best        *api.Tag
		bestVersion *semver.Version
	)

	for _, tag := range matches {
		version, err := semver.NewVersion(tag.GetName())
		if err != nil {
			continue
		}

		if bestVersion == nil || version.GreaterThan(bestVersion) {
			bestVersion = version
			best = tag
		}
	}

	if best != nil {
		return best, nil
	}

	// No tag parsed as semver; fall back to the longest (most specific) name.
	best = matches[0]
	for _, tag := range matches[1:] {
		if len(tag.GetName()) > len(best.GetName()) {
			best = tag
		}
	}

	return best, nil
}

// validateReference guards against malformed owner/repo/subpath segments before
// they are interpolated into an API path.
func validateReference(p ParsedAction) error {
	if !isSafePathSegment(p.Owner) {
		return fmt.Errorf("invalid owner %q in reference %s", p.Owner, p.Node.Value)
	}

	if !isSafePathSegment(p.Repo) {
		return fmt.Errorf("invalid repo %q in reference %s", p.Repo, p.Node.Value)
	}

	if p.Subpath == "" {
		return nil
	}

	for _, segment := range strings.Split(p.Subpath, "/") {
		if !isSafePathSegment(segment) {
			return fmt.Errorf("invalid subpath segment %q in reference %s", segment, p.Node.Value)
		}
	}

	return nil
}

func isSafePathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}

	return pathSegmentRegex.MatchString(segment)
}

// detectVersionStyle classifies a raw version reference.
func detectVersionStyle(input string) (VersionStyle, error) {
	switch {
	case sha1Regex.MatchString(input):
		return PinnedVersion, nil
	case fullSemverRegex.MatchString(input):
		return SemanticVersionFullyQualified, nil
	case minorSemverRegex.MatchString(input):
		return SemanticVersionPartiallyQualified, nil
	case majorSemverRegex.MatchString(input):
		return SemanticVersionMajorComponentOnly, nil
	case len(input) <= maxBranchNameLength && branchNameRegex.MatchString(input):
		return BranchReference, nil
	default:
		return "", fmt.Errorf("unknown version reference: %s", input)
	}
}
