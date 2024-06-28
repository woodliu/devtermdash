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
)

func init() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "storage.tsdb.path",
				Required:    true,
				Usage:       "Base path for metrics storage.",
				Destination: &storageDir,
			},
			&cli.StringFlag{
				Name:        "exporter.address",
				Required:    true,
				Usage:       "server metrics address. Format: [IP:PORT] or [URL:PORT]",
				Destination: &exporterAddr,
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
