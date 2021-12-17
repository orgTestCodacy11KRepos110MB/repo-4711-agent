// Package metricsutils provides utilities for creating metrics integrations.
package metricsutils

import (
	"time"

	"github.com/grafana/agent/pkg/integrations/v2"
	"github.com/prometheus/prometheus/pkg/relabel"
)

// CommonConfig is a set of common options shared by all integrations. It should be
// utilised by an integration's config by inlining the common options:
//
//   type IntegrationConfig struct {
//     Common config.CommonConfig `yaml:",inline"`
//   }
type CommonConfig struct {
	InstanceKey          *string           `yaml:"instance,omitempty"`
	ScrapeIntegration    *bool             `yaml:"scrape_integration,omitempty"`
	ScrapeInterval       time.Duration     `yaml:"scrape_interval,omitempty"`
	ScrapeTimeout        time.Duration     `yaml:"scrape_timeout,omitempty"`
	RelabelConfigs       []*relabel.Config `yaml:"relabel_configs,omitempty"`
	MetricRelabelConfigs []*relabel.Config `yaml:"metric_relabel_configs,omitempty"`
}

// MetricsConfig is an extension of integrations.Config that also embeds
// CommonConfig.
type MetricsConfig interface {
	integrations.Config
	MetricsConfig() CommonConfig
}