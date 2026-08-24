package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"
	"github.com/wayneashleyberry/gh-act/pkg/cmd"
)

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

	noMDFlag := &cli.BoolFlag{
		Name:  "no-md",
		Usage: "Skip scanning fenced YAML code blocks in markdown files",
	}

	onlyFlag := &cli.StringSliceFlag{
		Name:  "only",
		Usage: "Only include actions matching these owner/repo glob patterns, repeatable (e.g. --only actions/* --only golangci/golangci-lint-action)",
	}

	collectOpts := func(c *cli.Command) cmd.CollectOptions {
		return cmd.CollectOptions{
			IncludeMarkdown: !c.Bool("no-md"),
			Filters:         c.StringSlice("only"),
		}
	}

	command := &cli.Command{
		Name:  "act",
		Usage: "Update, manage and pin your GitHub Actions",
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
				Flags: []cli.Flag{noMDFlag, onlyFlag},
				Action: func(_ context.Context, c *cli.Command) error {
					return cmd.ListActions(collectOpts(c))
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
					noMDFlag,
					onlyFlag,
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					found, err := cmd.ListOutdatedActions(ctx, collectOpts(c))
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
					noMDFlag,
					onlyFlag,
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return cmd.UpdateActions(ctx, c.Bool("pin"), c.Bool("dry-run"), collectOpts(c))
				},
			},
			{
				Name:  "pin",
				Usage: "Pin used actions",
				Flags: []cli.Flag{dryRunFlag, noMDFlag, onlyFlag},
				Action: func(ctx context.Context, c *cli.Command) error {
					return cmd.PinActions(ctx, c.Bool("dry-run"), collectOpts(c))
				},
			},
		},
	}

	return command.Run(ctx, os.Args)
}
