package main

import (
	"github.com/urfave/cli/v2"
	"log"
	"os"
	"sort"
)

var (
	storageDir   string
	exporterAddr string
	resetDb      bool
)

func init() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "storage.tsdb.path",
				Required:    true,
				Usage:       "Base path for metrics storage.",
				Destination: &storageDir,
				Action: func(context *cli.Context, dir string) error {
					if _, err := os.Stat(dir); os.IsNotExist(err) {
						if err := os.Mkdir(dir, 0o755); err != nil {
							return err
						}
						return nil
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "exporter.address",
				Required:    true,
				Usage:       "server metrics address. Format: [IP:PORT] or [URL:PORT]",
				Destination: &exporterAddr,
			},
			&cli.BoolFlag{
				Name:        "reset",
				Required:    false,
				Value:       false,
				Usage:       "reset db",
				Destination: &resetDb,
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
