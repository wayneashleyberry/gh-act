package cmd

import (
	"context"
	"fmt"
	"log/slog"
)

func ListOutdatedActions(ctx context.Context) error {
	files, err := findYAMFiles()
	if err != nil {
		return fmt.Errorf("find yaml files: %w", err)
	}

	for _, filepath := range files {
		actions, err := parseActionsInFile(ctx, filepath)
		if err != nil {
			return fmt.Errorf("get updates for file: %s: %w", filepath, err)
		}

		for _, action := range actions {
			outdated, err := action.IsOutdated()
			if err != nil {
				slog.Debug("problem checking if action is outdated", slog.String("action", action.ActionReference()), slog.String("error.message", err.Error()))

				continue
			}

			if !outdated {
				continue
			}

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
			}

			if action.VersionStyle != PinnedVersion {
				newVersionString, err := action.NewVersionString()
				if err != nil {
					return fmt.Errorf("new version from latest version tag: %w", err)
				}

				fmt.Printf(
					"%s: %s → %s@%s\n",
					location,
					current,
					action.ActionReference(),
					newVersionString,
				)
			}
		}
	}

	return nil
}
