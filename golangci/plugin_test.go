package golangci_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stokaro/teststyle"
	"github.com/stokaro/teststyle/golangci"
)

func TestNewAcceptsModulePluginConfig(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	assertNoError(t, os.WriteFile(baselinePath, []byte(`{"test_conditionals":[],"white_box_files":[]}`), 0o600))

	analyzers, err := golangci.New(map[string]any{
		"baseline_path":                  baselinePath,
		"root":                           ".",
		"white_box_justification_prefix": "// White-box testing required:",
		"disabled_rules": []any{
			teststyle.RuleNoIf,
			teststyle.RuleNoSwitch,
		},
	})

	assertNoError(t, err)
	assertEqual(t, len(analyzers), 1)
	assertEqual(t, analyzers[0].Name, "teststyle")
}

func TestNewAcceptsStringSliceDisabledRules(t *testing.T) {
	analyzers, err := golangci.New(map[string]any{
		"disabled_rules": []string{
			teststyle.RuleNoGoto,
		},
	})

	assertNoError(t, err)
	assertEqual(t, len(analyzers), 1)
	assertEqual(t, analyzers[0].Name, "teststyle")
}

func TestNewRejectsInvalidConfigShape(t *testing.T) {
	_, err := golangci.New("invalid")

	assertErrorContains(t, err, "cannot unmarshal string")
}

func TestNewRejectsInvalidDisabledRules(t *testing.T) {
	_, err := golangci.New(map[string]any{
		"disabled_rules": []any{42},
	})

	assertErrorContains(t, err, "cannot unmarshal number into Go struct field Config.disabled_rules")
}

func TestNewRejectsUnknownConfigKeys(t *testing.T) {
	_, err := golangci.New(map[string]any{
		"baseline": ".teststyle-baseline.json",
	})

	assertErrorContains(t, err, `unknown field "baseline"`)
}

func TestModulePluginRegistration(t *testing.T) {
	newPlugin, err := register.GetPlugin("teststyle")
	assertNoError(t, err)

	plugin, err := newPlugin(map[string]any{
		"disabled_rules": []any{teststyle.RuleNoGoto},
	})
	assertNoError(t, err)

	analyzers, err := plugin.BuildAnalyzers()
	assertNoError(t, err)

	assertEqual(t, plugin.GetLoadMode(), register.LoadModeSyntax)
	assertEqual(t, len(analyzers), 1)
	assertEqual(t, analyzers[0].Name, "teststyle")
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEqual[T comparable](t *testing.T, got T, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %q, want substring %q", err.Error(), want)
	}
}
