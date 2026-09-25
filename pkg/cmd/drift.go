package cmd

import (
	"fmt"
	"log/slog"
	"strings"
)

// driftEntry is a single located reference to an action, before grouping.
type driftEntry struct {
	FilePath   string
	Line       int
	Column     int
	RawValue   string
	RawVersion string
	Style      VersionStyle
}

// driftGroup is every reference to the same action (owner/repo/subpath,
// matched case-insensitively) found across the repository.
type driftGroup struct {
	ActionRef string
	Entries   []driftEntry
}

// hasDrift reports whether a group references more than one distinct raw
// version or pin style.
func (g driftGroup) hasDrift() bool {
	versions := make(map[string]bool)
	styles := make(map[VersionStyle]bool)

	for _, e := range g.Entries {
		versions[e.RawVersion] = true
		styles[e.Style] = true
	}

	return len(versions) > 1 || len(styles) > 1
}

// ListActionDrift prints every action referenced with more than one distinct
// version or pin style across the repository (e.g. one workflow pinning to a
// commit SHA while another uses a tag, or two workflows on different tags of
// the same action). It performs no network calls: refs are compared as
// written, so a SHA and a tag that happen to point at the same commit are
// still reported as drift. Returns whether any drift was found.
func ListActionDrift(opts CollectOptions) (bool, error) {
	_, refs, err := collectActionRefs(opts)
	if err != nil {
		return false, fmt.Errorf("find actions: %w", err)
	}

	groups := groupDriftEntries(refs)

	found := false

	for _, group := range groups {
		if !group.hasDrift() {
			continue
		}

		found = true

		printDriftGroup(group)
	}

	return found, nil
}

// groupDriftEntries parses every pinnable, non-nopin action reference and
// groups them by owner/repo(/subpath), preserving first-seen order.
func groupDriftEntries(refs []Action) []driftGroup {
	index := make(map[string]int)

	var groups []driftGroup

	for _, ref := range refs {
		value := ref.Node.Value

		if !isPinnableRef(value) || hasNopinDirective(ref.Node.LineComment) {
			continue
		}

		owner, repo, subpath, rawVersion, err := splitActionValue(value)
		if err != nil {
			slog.Debug("problem parsing action for drift check", slog.String("action", value), slog.String("error.message", err.Error()))

			continue
		}

		style, err := detectVersionStyle(rawVersion)
		if err != nil {
			slog.Debug("could not determine version style for drift check", slog.String("action", value), slog.String("error.message", err.Error()))

			continue
		}

		actionRef := owner + "/" + repo
		if subpath != "" {
			actionRef += "/" + subpath
		}

		key := strings.ToLower(actionRef)

		entry := driftEntry{
			FilePath:   ref.FilePath,
			Line:       ref.Node.Line,
			Column:     ref.Node.Column,
			RawValue:   value,
			RawVersion: rawVersion,
			Style:      style,
		}

		i, ok := index[key]
		if !ok {
			index[key] = len(groups)
			groups = append(groups, driftGroup{ActionRef: actionRef})
			i = len(groups) - 1
		}

		groups[i].Entries = append(groups[i].Entries, entry)
	}

	return groups
}

// printDriftGroup prints a single drifting group in a human-readable form.
func printDriftGroup(group driftGroup) {
	fmt.Printf("%s is used inconsistently:\n", group.ActionRef)

	for _, e := range group.Entries {
		fmt.Printf("  %s:%d:%d: %s (%s)\n", e.FilePath, e.Line, e.Column, e.RawValue, e.Style)
	}
}
