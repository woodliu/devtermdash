package main

import (
	"context"
	"os"
	"path"
	"time"
)

func main() {
	if resetDb {
		dir, err := os.ReadDir(storageDir)
		if err != nil {
			zapLogger.Panicln(err)
		}
		for _, d := range dir {
			os.RemoveAll(path.Join([]string{storageDir, d.Name()}...))
		}
	}

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
