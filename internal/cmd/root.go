package cmd

import (
	"context"
	"os"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/urfave/cli/v3"
	"go.uber.org/fx"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/client"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/metric"
)

// ExitFailure status code.
const ExitFailure = 1

func Execute() {
	// nolint: exhaustruct
	root := &cli.Command{
		Name:        "nats-blackbox-exporter",
		Description: "ping pong with nats broker to make sure it is up and running",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "configPath",
				Value: "./config.yaml",
				Usage: "Path to config file",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			fx.New(
				fx.Supply(c.String("configPath")),
				fx.Provide(config.Provide),
				fx.Provide(logger.Provide),
				fx.Provide(metric.Provide),
				fx.Provide(client.Provide),
				fx.Invoke(func() {
					_ = pterm.DefaultBigText.WithLetters(
						putils.LettersFromStringWithStyle("NATS", pterm.FgCyan.ToStyle()),
						putils.LettersFromStringWithStyle(" BlackBox", pterm.FgLightMagenta.ToStyle()),
						putils.LettersFromStringWithStyle(" Exporter", pterm.FgLightMagenta.ToStyle()),
					).Render()
				}),
			).Run()

			return nil
		},
	}

	if err := root.Run(context.Background(), os.Args); err != nil {
		os.Exit(ExitFailure)
	}
}
