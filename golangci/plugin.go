// Package golangci exposes teststyle as a golangci-lint module plugin.
package golangci

import (
	"github.com/golangci/plugin-module-register/register"
	"github.com/stokaro/teststyle"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("teststyle", newPlugin)
}

// Plugin adapts teststyle to golangci-lint's module-plugin contract.
type Plugin struct {
	config teststyle.Config
}

func newPlugin(conf any) (register.LinterPlugin, error) {
	config, err := configFromAny(conf)
	if err != nil {
		return nil, err
	}
	return Plugin{config: config}, nil
}

// BuildAnalyzers returns the configured teststyle analyzer.
func (p Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	analyzer, err := teststyle.NewAnalyzer(p.config)
	if err != nil {
		return nil, err
	}
	return []*analysis.Analyzer{analyzer}, nil
}

// GetLoadMode reports that teststyle needs syntax-only package loading.
func (p Plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

// New returns configured analyzers for direct tests and non-golangci adapters.
func New(conf any) ([]*analysis.Analyzer, error) {
	plugin, err := newPlugin(conf)
	if err != nil {
		return nil, err
	}
	return plugin.BuildAnalyzers()
}

func configFromAny(conf any) (teststyle.Config, error) {
	if conf == nil {
		return teststyle.Config{}, nil
	}
	return register.DecodeSettings[teststyle.Config](conf)
}
