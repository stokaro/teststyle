// Package golangci exposes teststyle as a golangci-lint module plugin.
package golangci

import (
	"fmt"

	"github.com/stokaro/teststyle"
	"golang.org/x/tools/go/analysis"
)

// New is the entrypoint consumed by golangci-lint custom module plugins.
func New(conf any) ([]*analysis.Analyzer, error) {
	config, err := configFromAny(conf)
	if err != nil {
		return nil, err
	}
	analyzer, err := teststyle.NewAnalyzer(config)
	if err != nil {
		return nil, err
	}
	return []*analysis.Analyzer{analyzer}, nil
}

func configFromAny(conf any) (teststyle.Config, error) {
	if conf == nil {
		return teststyle.Config{}, nil
	}
	values, ok := conf.(map[string]any)
	if !ok {
		return teststyle.Config{}, fmt.Errorf("teststyle plugin config must be a map, got %T", conf)
	}
	config := teststyle.Config{}
	if value, ok := values["baseline_path"].(string); ok {
		config.BaselinePath = value
	}
	if value, ok := values["root"].(string); ok {
		config.Root = value
	}
	if value, ok := values["white_box_justification_prefix"].(string); ok {
		config.WhiteBoxJustificationPrefix = value
	}
	if raw, ok := values["disabled_rules"].([]any); ok {
		config.DisabledRules = make([]string, 0, len(raw))
		for _, item := range raw {
			value, ok := item.(string)
			if !ok {
				return teststyle.Config{}, fmt.Errorf("disabled_rules entries must be strings, got %T", item)
			}
			config.DisabledRules = append(config.DisabledRules, value)
		}
	}
	return config, nil
}
