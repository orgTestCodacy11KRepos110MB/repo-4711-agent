package github

import (
	"context"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/agent/component"
	metricsscraper "github.com/grafana/agent/component/metrics-scraper"
)

func init() {
	component.Register(component.Registration[Config]{
		Name: "integration.github",
		BuildComponent: func(l log.Logger, c Config) (component.Component[Config], error) {
			return NewComponent(l, c)
		},
	})
}

// Config represents the input state of the integration.github component.
type Config struct {
	Repositories []string `hcl:"repositories"`
}

// State represents the output state of the integration.github component.
type State struct {
	Targets []metricsscraper.TargetGroup `hcl:"targets"`
}

// Component is the integration.github component.
type Component struct {
	log log.Logger

	mut sync.RWMutex
	cfg Config
}

// NewComponent creates a new integration.github component.
func NewComponent(l log.Logger, c Config) (*Component, error) {
	res := &Component{log: l, cfg: c}
	if err := res.Update(c); err != nil {
		return nil, err
	}
	return res, nil
}

var _ component.Component[Config] = (*Component)(nil)

// Run implements Component.
func (c *Component) Run(ctx context.Context, onStateChange func()) error {
	level.Info(c.log).Log("msg", "component starting")
	defer level.Info(c.log).Log("msg", "component shutting down")

	<-ctx.Done()
	return nil
}

// Update implements UpdatableComponent.
func (c *Component) Update(cfg Config) error {
	c.mut.Lock()
	defer c.mut.Unlock()
	spew.Dump(cfg)
	c.cfg = cfg
	return nil
}

// CurrentState implements Component.
func (c *Component) CurrentState() interface{} {
	return State{}
}

// Config implements Component.
func (c *Component) Config() Config {
	c.mut.RLock()
	defer c.mut.RUnlock()
	return c.cfg
}