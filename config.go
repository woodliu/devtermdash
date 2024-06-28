package main

import (
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"time"
)

const (
	globSampleLimit = 1500
	globTargetLimit = 30
	scrapeInterval  = model.Duration(5 * time.Second)
	jobName         = "prometheus"
)

var Conf *config.Config

func init() {
	staticConfig := &discovery.StaticConfig{
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel: model.LabelValue(exporterAddr),
				},
			},
			Source: "0",
		},
	}

	sd := discovery.Configs{
		staticConfig,
	}

	Conf = &config.Config{
		GlobalConfig: config.GlobalConfig{
			ScrapeInterval:     scrapeInterval,
			ScrapeTimeout:      config.DefaultGlobalConfig.ScrapeTimeout,
			EvaluationInterval: model.Duration(30 * time.Second),

			SampleLimit:     globSampleLimit,
			TargetLimit:     globTargetLimit,
			ScrapeProtocols: config.DefaultGlobalConfig.ScrapeProtocols,
		},

		Runtime: config.DefaultRuntimeConfig,

		ScrapeConfigs: []*config.ScrapeConfig{
			{
				JobName: jobName,

				HonorLabels:       true,
				HonorTimestamps:   true,
				ScrapeInterval:    model.Duration(15 * time.Second),
				ScrapeTimeout:     config.DefaultGlobalConfig.ScrapeTimeout,
				EnableCompression: true,

				MetricsPath: config.DefaultScrapeConfig.MetricsPath,
				Scheme:      config.DefaultScrapeConfig.Scheme,

				HTTPClientConfig: common_config.HTTPClientConfig{
					FollowRedirects: true,
					EnableHTTP2:     true,
				},

				ServiceDiscoveryConfigs: sd,
			},
		},

		StorageConfig: config.StorageConfig{
			TSDBConfig: &config.TSDBConfig{
				OutOfOrderTimeWindow:     30 * time.Minute.Milliseconds(),
				OutOfOrderTimeWindowFlag: model.Duration(30 * time.Minute),
			},
			ExemplarsConfig: &config.DefaultExemplarsConfig,
		},
	}
}
