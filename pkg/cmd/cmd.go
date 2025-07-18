package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/wayneashleyberry/gh-act/pkg/api"
	"gopkg.in/yaml.v3"
)

var (
	// Configuration.
	workflowDirectory = ".github"

	// Patterns.
	sha1Regex = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
	// fully qualified semantic version (major.minor.patch).
	fullSemverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$`)
	// semantic version with major and minor components (major.minor).
	minorSemverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$`)
	// semantic version with only major component (major).
	majorSemverRegex = regexp.MustCompile(`^v?(\d+)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$`)
)

type VersionStyle string

var (
	PinnedVersion                     = VersionStyle("pinned")
	SemanticVersionFullyQualified     = VersionStyle("semver")
	SemanticVersionPartiallyQualified = VersionStyle("semver-partial")
	SemanticVersionMajorComponentOnly = VersionStyle("semver-major")
)

func parseActionsInFile(ctx context.Context, filepath string) ([]ParsedAction, error) {
	// Read the YAML file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return []ParsedAction{}, fmt.Errorf("unable to read file: %w", err)
	}

	var content map[string]yaml.Node

	err = yaml.Unmarshal(data, &content)
	if err != nil {
		return []ParsedAction{}, fmt.Errorf("unable to unmarshal YAML: %w", err)
	}

	parsedActions := []ParsedAction{}

	// Parse jobs.*.steps
	jobsNode, ok := content["jobs"]
	if ok {
		for i := 0; i < len(jobsNode.Content); i += 2 {
			jobContentNode := jobsNode.Content[i+1]
			parsedActions = append(parsedActions, parseSteps(ctx, filepath, jobContentNode, "steps")...)
		}
	}

	// Parse runs.steps
	runsNode, ok := content["runs"]
	if ok {
		parsedActions = append(parsedActions, parseSteps(ctx, filepath, &runsNode, "steps")...)
	}

	return parsedActions, nil
}

// Helper function to parse "steps" from a given node.
func parseSteps(ctx context.Context, filepath string, parentNode *yaml.Node, key string) []ParsedAction {
	parsedActions := []ParsedAction{}

	for j := 0; j < len(parentNode.Content); j += 2 {
		keyNode := parentNode.Content[j]
		if keyNode.Value == key {
			stepsNode := parentNode.Content[j+1]
			for _, stepNode := range stepsNode.Content {
				if stepNode.Kind == yaml.MappingNode {
					for k := 0; k < len(stepNode.Content); k += 2 {
						stepKeyNode := stepNode.Content[k]
						if stepKeyNode.Value == "uses" {
							usesNode := stepNode.Content[k+1]

							if strings.HasPrefix(usesNode.Value, ".") {
								slog.Debug("ignoring local action", slog.String("value", usesNode.Value))

								continue
							}

							if strings.Count(usesNode.Value, "/") > 1 {
								slog.Debug("ignoring nested action", slog.String("value", usesNode.Value))

								continue
							}

							parsed, err := parseAction(ctx, Action{
								FilePath: filepath,
								Node:     *usesNode,
							})
							if err != nil {
								slog.Debug("problem parsing action", slog.String("error.message", err.Error()))

								continue
							}

							parsedActions = append(parsedActions, parsed)
						}
					}
				}
			}
		}
	}

	return parsedActions
}

type Action struct {
	FilePath string
	Node     yaml.Node
}

type ParsedAction struct {
	FilePath          string
	Node              yaml.Node
	Owner, Repo       string
	RawVersionString  string
	CurrentVersionTag *api.Tag
	LatestVersionTag  *api.Tag
	PinVersionTag     *api.Tag
	VersionStyle      VersionStyle
}

func (p ParsedAction) NewVersionString() (string, error) {
	newVersion, err := semver.NewVersion(p.LatestVersionTag.GetName())
	if err != nil {
		return "", fmt.Errorf("new version from latest version tag: %w", err)
	}

	var newVersionString string

	switch p.VersionStyle {
	case SemanticVersionFullyQualified:
		newVersionString = newVersion.String()
	case SemanticVersionPartiallyQualified:
		newVersionString = fmt.Sprintf("v%d.%d", newVersion.Major(), newVersion.Minor())
	case SemanticVersionMajorComponentOnly:
		newVersionString = fmt.Sprintf("v%d", newVersion.Major())
	}

	return newVersionString, nil
}

func (p ParsedAction) IsOutdated() (bool, error) {
	var constraint string

	if p.CurrentVersionTag == nil {
		return false, fmt.Errorf("checking if parsed action is outdated, but current version tag is nil: %s", p.Node.Value)
	}

	if p.LatestVersionTag == nil {
		return false, fmt.Errorf("checking if parsed action is outdated, but latest version tag is nil: %s", p.Node.Value)
	}

	if p.VersionStyle == PinnedVersion {
		constraint = "> " + p.CurrentVersionTag.GetName()
	} else {
		constraint = "> " + p.RawVersionString
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

var ErrCurrentVersionUnmatched = errors.New("could not match version tag to a tag or release on github (might be tagged to a commit)")

func matchVersionToTag(rawVersion string, style VersionStyle, tags []api.Tag) (*api.Tag, error) {
	if style == PinnedVersion {
		pinnedMatches := []api.Tag{}

		for _, tag := range tags {
			if rawVersion == tag.Commit.GetSHA() {
				pinnedMatches = append(pinnedMatches, tag)
			}
		}

		if len(pinnedMatches) == 0 {
			return nil, errors.New("pinned to a commit hash with no associated tag")
		}

		if len(pinnedMatches) == 1 {
			return &pinnedMatches[0], nil
		}

		// Some projects, like actions/github-script, point multiple tags to
		// the same commit. For example, v7 and v7.0.1 will have the same
		// sha. In these kinds of scenarios, the more specific tag is chosen.
		var specificTagName string

		var specificTag api.Tag

		slog.Debug("matched pinned commit hash to more than one tag", slog.String("version", rawVersion))

		for _, tag := range pinnedMatches {
			if len(tag.GetName()) > len(specificTagName) {
				specificTagName = tag.GetName()
				specificTag = tag
			}
		}

		return &specificTag, nil
	}

	currentVersion, err := semver.NewVersion(rawVersion)
	if err != nil {
		return nil, fmt.Errorf("parse raw version string: %s: %w", rawVersion, err)
	}

	for _, tag := range tags {
		tagVersion, err := semver.NewVersion(tag.GetName())
		if err != nil {
			slog.Debug(
				"could not parse tag name as semantic version",
				slog.String("node.value", rawVersion),
				slog.String("tag.name", tag.GetName()),
				slog.String("error.message", err.Error()),
			)

			continue
		}

		if currentVersion.Equal(tagVersion) {
			return &tag, nil
		}
	}

	return nil, errors.New("no match")
}

func parseAction(ctx context.Context, action Action) (ParsedAction, error) {
	parts := strings.Split(action.Node.Value, "@")
	if len(parts) != 2 {
		return ParsedAction{}, fmt.Errorf("too few parts: %s", action.Node.Value)
	}

	subParts := strings.Split(parts[0], "/")
	if len(subParts) > 2 {
		return ParsedAction{}, fmt.Errorf("too few parts: %s", action.Node.Value)
	}

	parsed := ParsedAction{
		FilePath:         action.FilePath,
		Node:             action.Node,
		RawVersionString: parts[1],
		Owner:            subParts[0],
		Repo:             subParts[1],
	}

	style, err := detectVersionStyle(parsed.RawVersionString)
	if err != nil {
		return ParsedAction{}, fmt.Errorf("could not determine version style: %w", err)
	}

	parsed.VersionStyle = style

	tags, err := api.FetchAllTags(ctx, parsed.Owner, parsed.Repo)
	if err != nil {
		return parsed, fmt.Errorf("fetch all tags: %w", err)
	}

	// If there are no tags on a repo then we can't do much.
	if len(tags) == 0 {
		return ParsedAction{}, fmt.Errorf("action has no tags: %s/%s", parsed.Owner, parsed.Repo)
	}

	matchedTag, err := matchVersionToTag(parsed.RawVersionString, parsed.VersionStyle, tags)
	if err != nil {
		return parsed, fmt.Errorf("problem matching version to a tag: %w", err)
	}

	if matchedTag == nil {
		slog.Debug("match version to tag didn't return an error or a tag", slog.String("action", action.Node.Value))
	}

	parsed.CurrentVersionTag = matchedTag

	currentVersion, err := semver.NewVersion(matchedTag.GetName())
	if err != nil {
		slog.Debug(
			"details on problematic matched tag",
			slog.String("name", matchedTag.Name),
			slog.String("commit.sha", matchedTag.Commit.Sha),
			slog.String("commit.url", matchedTag.Commit.URL),
		)

		return parsed, fmt.Errorf("parse semantic version: %s: %w", matchedTag.GetName(), err)
	}

	var latestVersion *semver.Version

	var latestTag *api.Tag

	pinVersionTag := parsed.CurrentVersionTag

	nextVersion := currentVersion

	if parsed.VersionStyle == SemanticVersionFullyQualified {
		incPatch := currentVersion.IncPatch()
		nextVersion = &incPatch
	}

	if parsed.VersionStyle == SemanticVersionPartiallyQualified {
		incMinor := currentVersion.IncMinor()
		nextVersion = &incMinor
	}

	if parsed.VersionStyle == SemanticVersionMajorComponentOnly {
		incMajor := currentVersion.IncMajor()
		nextVersion = &incMajor
	}

	pinConstraintString := ">=" + currentVersion.String() + " <" + nextVersion.String()

	slog.Debug("constraint string: " + pinConstraintString)

	pinConstraint, err := semver.NewConstraint(pinConstraintString)
	if err != nil {
		return parsed, fmt.Errorf("new constraint: %w", err)
	}

	pinVersion := currentVersion

	for _, tag := range tags {
		tagVersion, err := semver.NewVersion(tag.GetName())
		if err != nil {
			slog.Debug(
				"could not parse tag name as semantic version",
				slog.String("node.value", action.Node.Value),
				slog.String("tag.name", tag.GetName()),
				slog.String("error.message", err.Error()),
			)

		// Exclude pre-release versions (alpha, beta, rc, etc.)
		if tagVersion.Prerelease() != "" {
			continue
		}

		if latestVersion == nil {
			latestVersion = tagVersion
			latestTag = &tag
		}

		if tagVersion.GreaterThan(latestVersion) {
			latestVersion = tagVersion
			latestTag = &tag
		}

		if tagVersion.GreaterThan(pinVersion) && pinConstraint.Check(tagVersion) {
			pinVersion = tagVersion
			pinVersionTag = &tag
		}
	}

	parsed.LatestVersionTag = latestTag
	parsed.PinVersionTag = pinVersionTag

	return parsed, nil
}

func findYAMFiles() ([]string, error) {
	var yamlFiles []string

	err := filepath.Walk(workflowDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if the file has a .yaml or .yml extension
		if !info.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
			yamlFiles = append(yamlFiles, path)
		}

		return nil
	})
	if err != nil {
		return []string{}, fmt.Errorf("error walking the path %q: %w", workflowDirectory, err)
	}

	return yamlFiles, nil
}

func parseYAMLFile(filePath string) ([]Action, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	type Step struct {
		Uses yaml.Node `yaml:"uses"`
	}

	type Job struct {
		Steps []Step `yaml:"steps"`
	}

	type Workflow struct {
		Jobs map[string]Job `yaml:"jobs"`
	}

	// Unmarshal the YAML content into a Workflow struct
	var workflow Workflow

	err = yaml.Unmarshal(data, &workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
	}

	var actions []Action

	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Uses.Value == "" {
				continue
			}

			actions = append(actions, Action{
				FilePath: filePath,
				Node:     step.Uses,
			})
		}
	}

	return actions, nil
}

func detectVersionStyle(input string) (VersionStyle, error) {
	// Check if the string is a valid SHA1
	if sha1Regex.MatchString(input) {
		return PinnedVersion, nil
	}

	// Check if the string is a fully qualified semantic version (major.minor.patch)
	if fullSemverRegex.MatchString(input) {
		return SemanticVersionFullyQualified, nil
	}

	// Check if the string is a semantic version with major and minor components (major.minor)
	if minorSemverRegex.MatchString(input) {
		return SemanticVersionPartiallyQualified, nil
	}

	// Check if the string is a semantic version with only major component (major)
	if majorSemverRegex.MatchString(input) {
		return SemanticVersionMajorComponentOnly, nil
	}

	return "", fmt.Errorf("unknown string type: %s", input)
}
