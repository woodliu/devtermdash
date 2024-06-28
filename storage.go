package main

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/agent"
	prom_runtime "github.com/prometheus/prometheus/util/runtime"
	"math"
	"sync"
	"time"
)

var localStorage = &readyStorage{stats: tsdb.NewDBStats()}

const (
	RetentionDuration      = 2 * time.Hour
	RetentionDurationInt64 = int64(RetentionDuration / time.Millisecond)
)

func setLocalStorage(storageDir string, logger log.Logger, localStorage *readyStorage) error {
	tsdbOpts := tsdb.DefaultOptions()
	tsdbOpts.RetentionDuration = RetentionDurationInt64
	db, err := openDBWithMetrics(storageDir, logger, prometheus.DefaultRegisterer, tsdbOpts, localStorage.getStats())
	if err != nil {
		return fmt.Errorf("opening storage failed: %w", err)
	}

	switch fsType := prom_runtime.Statfs(storageDir); fsType {
	case "NFS_SUPER_MAGIC":
		level.Warn(logger).Log("fs_type", fsType, "msg", "This filesystem is not supported and may lead to data corruption and data loss. Please carefully read https://prometheus.io/docs/prometheus/latest/storage/ to learn more about supported filesystems.")
	default:
		level.Info(logger).Log("fs_type", fsType)
	}

	level.Info(logger).Log("msg", "TSDB started")

	startTimeMargin := int64(2 * time.Duration(tsdbOpts.MinBlockDuration).Seconds() * 1000)
	localStorage.Set(db, startTimeMargin)
	return nil
}

func openDBWithMetrics(dir string, logger log.Logger, reg prometheus.Registerer, opts *tsdb.Options, stats *tsdb.DBStats) (*tsdb.DB, error) {
	db, err := tsdb.Open(
		dir,
		log.With(logger, "component", "tsdb"),
		reg,
		opts,
		stats,
	)
	if err != nil {
		return nil, err
	}

	reg.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_lowest_timestamp_seconds",
			Help: "Lowest timestamp value stored in the database.",
		}, func() float64 {
			bb := db.Blocks()
			if len(bb) == 0 {
				return float64(db.Head().MinTime() / 1000)
			}
			return float64(db.Blocks()[0].Meta().MinTime / 1000)
		}), prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_min_time_seconds",
			Help: "Minimum time bound of the head block.",
		}, func() float64 { return float64(db.Head().MinTime() / 1000) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_max_time_seconds",
			Help: "Maximum timestamp of the head block.",
		}, func() float64 { return float64(db.Head().MaxTime() / 1000) }),
	)

	return db, nil
}

// Copy from prometheus source code https://github.com/prometheus/prometheus/blob/main/cmd/prometheus/main.go#L1456
// readyStorage implements the Storage interface while allowing to set the actual
// storage at a later point in time.
type readyStorage struct {
	mtx             sync.RWMutex
	db              storage.Storage
	startTimeMargin int64
	stats           *tsdb.DBStats
}

func (s *readyStorage) ApplyConfig(conf *config.Config) error {
	db := s.get()
	if db, ok := db.(*tsdb.DB); ok {
		return db.ApplyConfig(conf)
	}
	return nil
}

// Set the storage.
func (s *readyStorage) Set(db storage.Storage, startTimeMargin int64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.db = db
	s.startTimeMargin = startTimeMargin
}

func (s *readyStorage) get() storage.Storage {
	s.mtx.RLock()
	x := s.db
	s.mtx.RUnlock()
	return x
}

func (s *readyStorage) getStats() *tsdb.DBStats {
	s.mtx.RLock()
	x := s.stats
	s.mtx.RUnlock()
	return x
}

// StartTime implements the Storage interface.
func (s *readyStorage) StartTime() (int64, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			var startTime int64
			if len(db.Blocks()) > 0 {
				startTime = db.Blocks()[0].Meta().MinTime
			} else {
				startTime = time.Now().Unix() * 1000
			}
			// Add a safety margin as it may take a few minutes for everything to spin up.
			return startTime + s.startTimeMargin, nil
		case *agent.DB:
			return db.StartTime()
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}

	return math.MaxInt64, tsdb.ErrNotReady
}

// Querier implements the Storage interface.
func (s *readyStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	if x := s.get(); x != nil {
		return x.Querier(mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

// ChunkQuerier implements the Storage interface.
func (s *readyStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	if x := s.get(); x != nil {
		return x.ChunkQuerier(mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

func (s *readyStorage) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.ExemplarQuerier(ctx)
		case *agent.DB:
			return nil, agent.ErrUnsupported
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return nil, tsdb.ErrNotReady
}

// Appender implements the Storage interface.
func (s *readyStorage) Appender(ctx context.Context) storage.Appender {
	if x := s.get(); x != nil {
		return x.Appender(ctx)
	}
	return notReadyAppender{}
}

type notReadyAppender struct{}

func (n notReadyAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendCTZeroSample(ref storage.SeriesRef, l labels.Labels, t, ct int64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) Commit() error { return tsdb.ErrNotReady }

func (n notReadyAppender) Rollback() error { return tsdb.ErrNotReady }
