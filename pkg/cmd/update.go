package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

func UpdateActions(ctx context.Context, pin bool) error {
	files, err := findYAMFiles()
	if err != nil {
		return fmt.Errorf("find yaml files: %w", err)
	}

	for _, filepath := range files {
		updates, err := parseActionsInFile(ctx, filepath)
		if err != nil {
			return fmt.Errorf("get updates for file: %w", err)
		}

		if pin {
			for n, update := range updates {
				update.VersionStyle = PinnedVersion
				updates[n] = update
			}
		}

		err = updateFile(filepath, updates)
		if err != nil {
			return fmt.Errorf("update file: %w", err)
		}
	}

	return nil
}

func updateFile(filepath string, updates []ParsedAction) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("read file: %s: %w", filepath, err)
	}

	lines := strings.Split(string(data), "\n")

	for _, update := range updates {
		i := update.Node.Line - 1

		line := lines[i]

		before, _, found := strings.Cut(line, "uses:")
		if !found {
			return errors.New("line separator not found")
		}

		if update.VersionStyle == PinnedVersion {
			line = before + fmt.Sprintf("uses: %s@%s # %s", update.ActionReference(), update.LatestVersionTag.Commit.GetSHA(), update.LatestVersionTag.GetName())
		} else {
			newVersionString, err := update.NewVersionString()
			if err != nil {
				return fmt.Errorf("new version from latest version tag: %w", err)
			}

			line = before + fmt.Sprintf("uses: %s@%s", update.ActionReference(), newVersionString)
		}

		lines[i] = line
	}

	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("create file: %s: %w", filepath, err)
	}

	_, err = f.WriteString(strings.Join(lines, "\n"))
	if err != nil {
		return fmt.Errorf("write string to file: %s: %w", filepath, err)
	}

	return nil
}
