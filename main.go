package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/wayneashleyberry/gh-act/pkg/cmd"
)

func setDefaultLogger(level slog.Leveler) {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(handler)

	slog.SetDefault(logger)
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(_ context.Context) error {
	setDefaultLogger(slog.LevelInfo)

	app := &cli.App{
		Name:  "act",
		Usage: "Update, manage and pin your GitHub Actions",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Value: false,
				Usage: "Print debug logs",
				Action: func(_ *cli.Context, v bool) error {
					if v {
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
				Action: func(_ *cli.Context) error {
					return cmd.ListActions()
				},
			},
			{
				Name:  "outdated",
				Usage: "Check for outdated actions",
				Action: func(ctx *cli.Context) error {
					return cmd.ListOutdatedActions(ctx.Context)
				},
			},
			{
				Name:  "update",
				Usage: "Update actions (supports branch references like @main when using --pin)",
				Action: func(ctx *cli.Context) error {
					return cmd.UpdateActions(ctx.Context, ctx.Bool("pin"))
				},
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "pin",
						Value: false,
						Usage: "Pin actions after updating them (required for branch references like @main)",
					},
				},
			},
			{
				Name:  "pin",
				Usage: "Pin used actions",
				Action: func(ctx *cli.Context) error {
					return cmd.PinActions(ctx.Context)
				},
			},
		},
	}

	return app.Run(os.Args)
}
