package main

import (
	"fmt"
	"time"

	repoter "github.com/gurleensethi/load-send/internal/reporter"
	"github.com/gurleensethi/load-send/internal/starlark/modules/loadsend"
	lifecyclescript "github.com/gurleensethi/load-send/internal/starlark/script"
	"github.com/gurleensethi/load-send/pkg/starlark/modules"
	"github.com/urfave/cli/v2"
	"go.starlark.net/starlarkstruct"
)

const (
	loadScriptNotFoundMsg = "load script not provided\nusage: load-send <path_to_script>"
)

func NewApp() *cli.App {
	return &cli.App{
		Name:    "load-send",
		Version: "v0.0.4",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "virual-users",
				Aliases:     []string{"vu"},
				Value:       5,
				DefaultText: "5",
				Usage:       "number of virtual users",
			},
			&cli.IntFlag{
				Name:        "duration",
				Aliases:     []string{"d"},
				Value:       30,
				DefaultText: "30",
				Usage:       "duration to run (in seconds)",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "verbose mode",
			},
		},
		Action: func(ctx *cli.Context) error {
			duration := ctx.Int("duration")
			vu := ctx.Int("virual-users")

			httpReporter := repoter.NewHttp()

			err := httpReporter.Start(ctx.Context)
			if err != nil {
				return err
			}

			s := lifecyclescript.New(map[string]*starlarkstruct.Module{
				"loadsend": loadsend.New(loadsend.Reporters{
					HttpRepoter: httpReporter,
				}),
				"os": modules.OS,
			})

			err = s.Run(ctx.Context, ctx.Args().Get(0), &lifecyclescript.RunOptions{
				VU:       vu,
				Duration: time.Duration(duration) * time.Second,
			})
			if err != nil {
				return err
			}

			err = httpReporter.Stop()
			if err != nil {
				return err
			}

			report := httpReporter.GetReport()

			for _, line := range report.DisplayLines {
				fmt.Println(line)
			}

			return nil
		},
	}
}
