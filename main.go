package main

import (
	"context"
	"time"
)

func main() {
	manager, err := startScrape()
	if err != nil {
		zapLogger.Panicln(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := createDashboard(ctx, cancel); err != nil {
		zapLogger.Panicf("create dashboard failed, %v", err)
	}

	<-ctx.Done()
	time.Sleep(time.Second)
	zapLogger.Infoln("system shutting down")
	manager.Stop()
	localStorage.db.Close()
}
