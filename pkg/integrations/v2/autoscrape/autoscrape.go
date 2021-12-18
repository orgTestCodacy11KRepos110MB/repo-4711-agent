// Package autoscrape implements a scraper for integrations.
package autoscrape

import (
	"context"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/agent/pkg/metrics"
	"github.com/grafana/agent/pkg/metrics/instance"
	"github.com/prometheus/common/model"
	prom_config "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/pkg/relabel"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
)

// DefaultGlobal holds default values for Global.
var DefaultGlobal Global = Global{
	Enable:          true,
	MetricsInstance: "default",
}

// Global holds default settings for metrics integrations that support
// autoscraping. Integrations may override their settings.
type Global struct {
	Enable          bool           `yaml:"enable,omitempty"`           // Whether self-scraping should be enabled.
	MetricsInstance string         `yaml:"metrics_instance,omitempty"` // Metrics instance name to send metrics to.
	ScrapeInterval  model.Duration `yaml:"scrape_interval,omitempty"`  // Self-scraping frequency.
	ScrapeTimeout   model.Duration `yaml:"scrape_timeout,omitempty"`   // Self-scraping timeout.
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (g *Global) UnmarshalYAML(f func(interface{}) error) error {
	*g = DefaultGlobal
	type global Global
	return f((*global)(g))
}

// Config configure autoscrape for an individual integration. Override defaults.
type Config struct {
	Enable          *bool          `yaml:"enable,omitempty"`           // Whether self-scraping should be enabled.
	MetricsInstance string         `yaml:"metrics_instance,omitempty"` // Metrics instance name to send metrics to.
	ScrapeInterval  model.Duration `yaml:"scrape_interval,omitempty"`  // Self-scraping frequency.
	ScrapeTimeout   model.Duration `yaml:"scrape_timeout,omitempty"`   // Self-scraping timeout.

	RelabelConfigs       []*relabel.Config `yaml:"relabel_configs,omitempty"`        // Relabel the autoscrape job
	MetricRelabelConfigs []*relabel.Config `yaml:"metric_relabel_configs,omitempty"` // Relabel individual autoscrape metrics
}

// ScrapeConfig bind a Prometheus scrape config with an instance to send
// scraped metrics to.
type ScrapeConfig struct {
	Instance string
	Config   prom_config.ScrapeConfig
}

// Scraper is a metrics autoscraper.
type Scraper struct {
	ctx    context.Context
	cancel context.CancelFunc

	log log.Logger
	im  instance.Manager

	scrapersMut sync.RWMutex
	scrapers    map[string]*instanceScraper
}

// NewScraper creates a new autoscraper. Scraper will run until Stop is called.
// Instances to send scraped metrics to will be looked up via im.
func NewScraper(l log.Logger, im instance.Manager) *Scraper {
	l = log.With(l, "component", "autoscraper")

	ctx, cancel := context.WithCancel(context.Background())

	s := &Scraper{
		ctx:    ctx,
		cancel: cancel,

		log:      l,
		im:       im,
		scrapers: map[string]*instanceScraper{},
	}
	return s
}

// ApplyConfig will apply the given jobs. An error will be returned for any
// jobs that failed to be applied.
func (s *Scraper) ApplyConfig(jobs []*ScrapeConfig) error {
	s.scrapersMut.Lock()
	defer s.scrapersMut.Unlock()

	var firstError error
	saveError := func(e error) {
		if firstError == nil {
			firstError = e
		}
	}

	// Shard our jobs by target instance.
	shardedJobs := map[string][]*prom_config.ScrapeConfig{}
	for _, j := range jobs {
		_, err := s.im.GetInstance(j.Instance)
		if err != nil {
			level.Error(s.log).Log("msg", "cannot autoscrape integration", "name", j.Config.JobName, "err", err)
			saveError(err)
			continue
		}

		shardedJobs[j.Instance] = append(shardedJobs[j.Instance], &j.Config)
	}

	// Then pass the jobs to instanceScraper, creating them if we need to.
	for instance, jobs := range shardedJobs {
		is, ok := s.scrapers[instance]
		if !ok {
			is = newInstanceScraper(s.ctx, s.log, s.im, instance)
			s.scrapers[instance] = is
		}
		if err := is.ApplyConfig(jobs); err != nil {
			// Not logging here; is.ApplyConfig already logged the errors.
			saveError(err)
		}
	}

	// Garbage collect: if if there's a key in s.scrapers that wasn't in
	// shardedJobs, stop that unused scraper.
	for instance, is := range s.scrapers {
		_, current := shardedJobs[instance]
		if !current {
			is.Stop()
			delete(s.scrapers, instance)
		}
	}

	return firstError
}

// TargetsActive returns the set of active scrape targets for all target
// instances.
func (s *Scraper) TargetsActive() map[string]metrics.TargetSet {
	s.scrapersMut.RLock()
	defer s.scrapersMut.RUnlock()

	allTargets := make(map[string]metrics.TargetSet, len(s.scrapers))
	for instance, is := range s.scrapers {
		allTargets[instance] = is.sm.TargetsActive()
	}
	return allTargets
}

// Stop stops the Scraper.
func (s *Scraper) Stop() {
	s.scrapersMut.Lock()
	defer s.scrapersMut.Unlock()

	for instance, is := range s.scrapers {
		is.Stop()
		delete(s.scrapers, instance)
	}

	s.cancel()
}

// instanceScraper is a Scraper which always sends to the same instance.
type instanceScraper struct {
	cancel context.CancelFunc
	log    log.Logger

	sd *discovery.Manager
	sm *scrape.Manager
}

// newInstanceScraper returns a new instanceScraper. Must be stopped by calling
// Stop.
func newInstanceScraper(
	ctx context.Context,
	l log.Logger,
	im instance.Manager,
	instanceName string,
) *instanceScraper {
	ctx, cancel := context.WithCancel(ctx)

	l = log.With(l, "target_instance", instanceName)

	sd := discovery.NewManager(ctx, l, discovery.Name("autoscraper/"+instanceName))
	sm := scrape.NewManager(&scrape.Options{}, l, &agentAppender{
		inst: instanceName,
		im:   im,
	})

	go func() { _ = sd.Run() }()
	go func() { _ = sm.Run(sd.SyncCh()) }()

	return &instanceScraper{
		cancel: cancel,
		log:    l,

		sd: sd,
		sm: sm,
	}
}

type agentAppender struct {
	inst string
	im   instance.Manager
}

func (aa *agentAppender) Appender(ctx context.Context) storage.Appender {
	mi, err := aa.im.GetInstance(aa.inst)
	if err != nil {
		return &failedAppender{instanceName: aa.inst}
	}
	return mi.Appender(ctx)
}

func (is *instanceScraper) ApplyConfig(jobs []*prom_config.ScrapeConfig) error {
	var firstError error
	saveError := func(e error) {
		if firstError == nil && e != nil {
			firstError = e
		}
	}

	var (
		scrapeConfigs = make([]*prom_config.ScrapeConfig, 0, len(jobs))
		sdConfigs     = make(map[string]discovery.Configs, len(jobs))
	)
	for _, job := range jobs {
		sdConfigs[job.JobName] = job.ServiceDiscoveryConfigs
		scrapeConfigs = append(scrapeConfigs, job)
	}
	if err := is.sd.ApplyConfig(sdConfigs); err != nil {
		level.Error(is.log).Log("msg", "error when applying SD to autoscraper", "err", err)
		saveError(err)
	}
	if err := is.sm.ApplyConfig(&prom_config.Config{ScrapeConfigs: scrapeConfigs}); err != nil {
		level.Error(is.log).Log("msg", "error when applying jobs to scraper", "err", err)
		saveError(err)
	}

	return firstError
}

func (is *instanceScraper) Stop() {
	is.cancel()
	is.sm.Stop()
}