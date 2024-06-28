package main

import (
	"context"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/promql"
	"time"
)

const (
	queryMaxSamples  = 50000000
	queryTimeout     = time.Minute * 2
	queryConcurrency = 20
	lookbackDelta    = time.Minute * 5
)

type querier struct {
	queryEngine *promql.Engine
	storage     *readyStorage
}

func newQuerier(localStoragePath string, storage *readyStorage) *querier {
	return &querier{
		queryEngine: newQueryEngine(localStoragePath),
		storage:     storage,
	}
}

func (q *querier) query(ctx context.Context, qs string, start time.Time, step time.Duration) (promql.Matrix, error) {
	end := time.Now()
	if qs == "" {
		return nil, nil
	}
	qry, err := q.queryEngine.NewRangeQuery(context.Background(), q.storage, nil, qs, start, end, step)
	if err != nil {
		return nil, err
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, res.Err
	}
	return res.Matrix()
}

func newQueryEngine(localStoragePath string) *promql.Engine {
	logger := promlog.New(&promlog.Config{})
	opts := promql.EngineOpts{
		Logger:             log.With(logger, "component", "query engine"),
		Reg:                prometheus.DefaultRegisterer,
		MaxSamples:         queryMaxSamples,
		Timeout:            queryTimeout,
		ActiveQueryTracker: promql.NewActiveQueryTracker(localStoragePath, queryConcurrency, log.With(logger, "component", "activeQueryTracker")),
		LookbackDelta:      lookbackDelta,
		NoStepSubqueryIntervalFn: func(rangeMillis int64) int64 {
			return int64(config.DefaultGlobalConfig.EvaluationInterval)
		},
		// EnableAtModifier and EnableNegativeOffset have to be
		// always on for regular PromQL as of Prometheus v2.33.
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		EnablePerStepStats:   false,
	}

	return promql.NewEngine(opts)
}
