package main

import (
	"context"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/scrape"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	manager, err := startScrape()
	if err != nil {
		zapLogger.Panicln(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := createDashboard(ctx); err != nil {
		zapLogger.Panicf("create dashboard failed, %v", err)
	}

	c := make(chan os.Signal, 10)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-c
	zapLogger.Infoln("system shutting down")
	cancel()
	manager.Stop()
	localStorage.db.Close()
	os.Exit(0)
}

func startScrape() (*scrape.Manager, error) {
	manager, err := newScrapeManager(storageDir, promlog.New(&promlog.Config{}), localStorage)
	if err != nil {
		return nil, err
	}

	if err := manager.ApplyConfig(Conf); err != nil {
		zapLogger.Panicln("Failed to apply configuration", err)
	}

	ts := make(chan map[string][]*targetgroup.Group)
	go manager.Run(ts)
	ts <- map[string][]*targetgroup.Group{
		jobName: {
			{
				Targets: []model.LabelSet{
					{
						"__address__": model.LabelValue(exporterAddr),
					},
				},
			},
		},
	}
	return manager, nil
}

func newScrapeManager(storageDir string, logger log.Logger, localStorage *readyStorage) (*scrape.Manager, error) {
	opts := scrape.Options{
		DiscoveryReloadInterval: model.Duration(time.Second * 5),
	}

	if err := setLocalStorage(storageDir, logger, localStorage); err != nil {
		return nil, err
	}
	return scrape.NewManager(&opts, log.With(promlog.New(&promlog.Config{}), "component", "scrape manager"), localStorage, prometheus.DefaultRegisterer)
}
