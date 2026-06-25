package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/urfave/cli/v3"
	"github.com/wayneashleyberry/gh-act/pkg/cmd"
)

// shortSHALen is how many characters of a commit SHA to display.
const shortSHALen = 7

// buildVersion derives the version shown by `act --version` from the build
// info stamped by the Go toolchain. For a precompiled release (a clean build
// at a git tag) it returns "<tag> (<short-sha>)". Otherwise it falls back to
// the short commit SHA (with a "-dirty" suffix for uncommitted changes), or
// "dev" when no build info is available.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	var revision, dirty string

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			if setting.Value == "true" {
				dirty = "-dirty"
			}
		}
	}

	short := revision
	if len(short) > shortSHALen {
		short = short[:shortSHALen]
	}

	// A clean tagged build (the release case) stamps the tag into
	// Main.Version. Pseudo-versions embed the commit SHA, so exclude those.
	tag := info.Main.Version
	isPseudo := short != "" && strings.Contains(tag, short)

	if tag != "" && tag != "(devel)" && !isPseudo {
		if short == "" {
			return tag
		}

		return fmt.Sprintf("%s (%s)", tag, short)
	}

	if short == "" {
		return "dev"
	}

	return short + dirty
}

func setDefaultLogger(level slog.Leveler) {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})

	slog.SetDefault(slog.New(handler))
}

func main() {
	if err := main2(); err != nil {
		fmt.Fprintln(os.Stderr, "act:", err)
		os.Exit(1)
	}
}

// main2 exists so that deferred cleanup (signal reset) runs before os.Exit.
func main2() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return run(ctx)
}

func run(ctx context.Context) error {
	setDefaultLogger(slog.LevelInfo)

	dryRunFlag := &cli.BoolFlag{
		Name:  "dry-run",
		Usage: "Print the changes that would be made without writing any files",
	}

	command := &cli.Command{
		Name:    "act",
		Usage:   "Update, manage and pin your GitHub Actions",
		Version: buildVersion(),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Print debug logs",
				Action: func(_ context.Context, _ *cli.Command, value bool) error {
					if value {
						setDefaultLogger(slog.LevelDebug)
					}

					return nil
				},
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "ls",
				Usage: "List used actions",
				Action: func(_ context.Context, _ *cli.Command) error {
					return cmd.ListActions()
				},
			},
			{
				Name:  "outdated",
				Usage: "Check for outdated actions",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "exit-code",
						Usage: "Exit with a non-zero status when outdated actions are found",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					found, err := cmd.ListOutdatedActions(ctx)
					if err != nil {
						return err
					}

					if c.Bool("exit-code") && found {
						return cli.Exit("", 1)
					}

					return nil
				},
			},
			{
				Name:  "update",
				Usage: "Update actions (supports branch references like @main when using --pin)",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "pin",
						Usage: "Pin actions after updating them (required for branch references like @main)",
					},
					dryRunFlag,
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return cmd.UpdateActions(ctx, c.Bool("pin"), c.Bool("dry-run"))
				},
			},
			{
				Name:  "pin",
				Usage: "Pin used actions",
				Flags: []cli.Flag{dryRunFlag},
				Action: func(ctx context.Context, c *cli.Command) error {
					return cmd.PinActions(ctx, c.Bool("dry-run"))
				},
			},
		},
	}

	return command.Run(ctx, os.Args)
}
