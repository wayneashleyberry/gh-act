package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wayneashleyberry/gh-act/pkg/api"
)

// ListOutdatedActions prints every action that has a newer version available
// and reports whether any were found.
func ListOutdatedActions(ctx context.Context) (bool, error) {
	client, err := api.NewClient()
	if err != nil {
		return false, fmt.Errorf("create github client: %w", err)
	}

	_, refs, err := collectActionRefs()
	if err != nil {
		return false, err
	}

	actions, err := resolveActions(ctx, refs, client)
	if err != nil {
		return false, err
	}

	found := false

	for _, action := range actions {
		outdated, err := action.IsOutdated()
		if err != nil {
			slog.Debug("problem checking if action is outdated", slog.String("action", action.ActionReference()), slog.String("error.message", err.Error()))

			continue
		}

		if !outdated {
			continue
		}

		found = true

		printOutdated(action)
	}

	return found, nil
}

// printOutdated prints a single outdated action in a human-readable form.
func printOutdated(action ParsedAction) {
	location := fmt.Sprintf("%s:%d:%d", action.FilePath, action.Node.Line, action.Node.Column)
	current := fmt.Sprintf("%s@%s", action.ActionReference(), action.RawVersionString)

	if action.VersionStyle == PinnedVersion {
		fmt.Printf(
			"%s: %s (%s) → %s@%s (%s)\n",
			location,
			current,
			action.CurrentVersionTag.GetName(),
			action.ActionReference(),
			action.LatestVersionTag.Commit.GetSHA(),
			action.LatestVersionTag.GetName(),
		)

		return
	}

	newVersionString, err := action.NewVersionString()
	if err != nil {
		slog.Debug("could not render new version string", slog.String("action", action.ActionReference()), slog.String("error.message", err.Error()))

		return
	}

	fmt.Printf(
		"%s: %s → %s@%s\n",
		location,
		current,
		action.ActionReference(),
		newVersionString,
	)
}
