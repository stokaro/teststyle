// Package teststyle audits Go tests for declarative style and black-box-by-default package structure.
package teststyle

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

const (
	RuleNoIf                  = "teststyle-no-if"
	RuleNoSwitch              = "teststyle-no-switch"
	RuleNoGoto                = "teststyle-no-goto"
	RuleWhiteBoxFileName      = "teststyle-whitebox-filename"
	RuleWhiteBoxJustification = "teststyle-whitebox-justification"

	DefaultWhiteBoxJustificationPrefix = "// White-box testing required:"
)

var ErrBaselineMismatch = errors.New("teststyle baseline mismatch")

// Config controls enabled rules and optional baseline filtering.
type Config struct {
	DisabledRules               []string `json:"disabled_rules"`
	BaselinePath                string   `json:"baseline_path"`
	Root                        string   `json:"root"`
	WhiteBoxJustificationPrefix string   `json:"white_box_justification_prefix"`
	// SkipExamples exempts parameterless Example* functions from the
	// conditional rules. An example is documentation first: the `if err !=
	// nil` it shows is often exactly what a reader should copy, so a
	// repository can keep examples idiomatic while holding Test* and Fuzz*
	// functions declarative. White-box file rules are unaffected, since they
	// judge files rather than functions.
	SkipExamples bool `json:"skip_examples"`
}

// Baseline records known test-style violations. Cleanup PRs should reduce this
// file; ordinary feature PRs should keep it unchanged.
type Baseline struct {
	TestConditionals []ConditionalBaseline `json:"test_conditionals"`
	WhiteBoxFiles    []WhiteBoxBaseline    `json:"white_box_files"`
}

// ConditionalBaseline records prohibited conditional statements in one test
// function, grouped by statement kind.
type ConditionalBaseline struct {
	Path     string `json:"path"`
	Function string `json:"function"`
	Kind     string `json:"kind"`
	Count    int    `json:"count"`
}

// WhiteBoxBaseline records a same-package test file that still needs black-box
// conversion or an explicit white-box justification.
type WhiteBoxBaseline struct {
	Path    string `json:"path"`
	Package string `json:"package"`
	Reason  string `json:"reason"`
}

// Finding is a machine-readable test-style violation.
type Finding struct {
	RuleID   string
	Path     string
	Line     int
	Column   int
	Package  string
	Function string
	Kind     string
	Reason   string
	Message  string
	pos      token.Pos
}

// Analyzer is the default go/analysis entrypoint with all rules enabled and no baseline.
var Analyzer = mustAnalyzer(Config{})

// NewAnalyzer returns a configured go/analysis analyzer.
func NewAnalyzer(config Config) (*analysis.Analyzer, error) {
	config = config.normalized()
	var err error
	if config.Root != "" {
		config.Root, err = filepath.Abs(config.Root)
		if err != nil {
			return nil, fmt.Errorf("resolve root: %w", err)
		}
	}
	if config.BaselinePath != "" && !filepath.IsAbs(config.BaselinePath) && config.Root != "" {
		config.BaselinePath = filepath.Join(config.Root, config.BaselinePath)
	}
	var baseline Baseline
	if config.BaselinePath != "" {
		read, err := ReadBaseline(config.BaselinePath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		} else {
			baseline = read
		}
	}
	disabled := disabledRuleSet(config.DisabledRules)
	tracker := newBaselineTracker(baseline)

	return &analysis.Analyzer{
		Name: "teststyle",
		Doc:  "enforces declarative Go tests and black-box-by-default test packages",
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				filename := pass.Fset.Position(file.Package).Filename
				if !strings.HasSuffix(filename, "_test.go") {
					continue
				}
				path := relativePath(config.Root, filename)
				for _, finding := range scanFile(pass.Fset, filename, path, file, config, disabled) {
					if tracker.Allow(finding) {
						continue
					}
					pass.Report(analysis.Diagnostic{
						Pos:      finding.pos,
						Category: finding.RuleID,
						Message:  finding.Message,
					})
				}
			}
			return nil, nil
		},
	}, nil
}

// Scan scans root and returns the current repository test-style baseline.
func Scan(root string) (Baseline, error) {
	return ScanWithConfig(root, Config{})
}

// ScanWithConfig scans root with a custom rule configuration.
func ScanWithConfig(root string, config Config) (Baseline, error) {
	config = config.normalized()
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Baseline{}, fmt.Errorf("resolve root: %w", err)
	}
	config.Root = root
	disabled := disabledRuleSet(config.DisabledRules)

	fset := token.NewFileSet()
	conditionals := map[conditionalKey]int{}
	var whiteBoxFiles []WhiteBoxBaseline

	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skippedDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relativize %s: %w", path, err)
		}
		relativePath = filepath.ToSlash(relativePath)

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relativePath, err)
		}
		for _, finding := range scanFile(fset, path, relativePath, file, config, disabled) {
			addFinding(conditionals, &whiteBoxFiles, finding)
		}
		return nil
	}); err != nil {
		return Baseline{}, err
	}

	return Baseline{
		TestConditionals: conditionalBaseline(conditionals),
		WhiteBoxFiles:    sortedWhiteBoxBaseline(whiteBoxFiles),
	}, nil
}

// ReadBaseline reads a baseline JSON file.
func ReadBaseline(path string) (Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, fmt.Errorf("read baseline: %w", err)
	}
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, fmt.Errorf("parse baseline: %w", err)
	}
	baseline.Normalize()
	return baseline, nil
}

// WriteBaseline writes a deterministic baseline JSON file.
func WriteBaseline(path string, baseline Baseline) error {
	baseline.Normalize()
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

// Normalize sorts baseline entries deterministically.
func (b *Baseline) Normalize() {
	sort.Slice(b.TestConditionals, func(i, j int) bool {
		return compareConditional(b.TestConditionals[i], b.TestConditionals[j]) < 0
	})
	sort.Slice(b.WhiteBoxFiles, func(i, j int) bool {
		return compareWhiteBox(b.WhiteBoxFiles[i], b.WhiteBoxFiles[j]) < 0
	})
}

// HasFinding reports whether finding is recorded in the baseline.
func (b Baseline) HasFinding(finding Finding) bool {
	for _, value := range b.TestConditionals {
		if value.Path == finding.Path && value.Function == finding.Function && value.Kind == finding.Kind {
			return true
		}
	}
	for _, value := range b.WhiteBoxFiles {
		if value.Path == finding.Path && value.Package == finding.Package && value.Reason == finding.Reason {
			return true
		}
	}
	return false
}

// Diff returns a human-readable mismatch report. Empty string means equal.
func Diff(want, got Baseline) string {
	want = cloneBaseline(want)
	got = cloneBaseline(got)
	want.Normalize()
	got.Normalize()

	var builder strings.Builder
	writeConditionalDiff(&builder, want.TestConditionals, got.TestConditionals)
	writeWhiteBoxDiff(&builder, want.WhiteBoxFiles, got.WhiteBoxFiles)
	return builder.String()
}

func mustAnalyzer(config Config) *analysis.Analyzer {
	analyzer, err := NewAnalyzer(config)
	if err != nil {
		panic(err)
	}
	return analyzer
}

func (c Config) normalized() Config {
	if c.WhiteBoxJustificationPrefix == "" {
		c.WhiteBoxJustificationPrefix = DefaultWhiteBoxJustificationPrefix
	}
	return c
}

func disabledRuleSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

func ruleDisabled(disabled map[string]struct{}, ruleID string) bool {
	_, ok := disabled[ruleID]
	return ok
}

func scanFile(
	fset *token.FileSet,
	filename string,
	relativePath string,
	file *ast.File,
	config Config,
	disabled map[string]struct{},
) []Finding {
	var findings []Finding
	findings = append(findings, scanConditionals(fset, relativePath, file, config, disabled)...)
	findings = append(findings, scanWhiteBoxFile(filename, relativePath, file.Name.Name, file.Package, config, disabled)...)
	return findings
}

func scanConditionals(
	fset *token.FileSet,
	path string,
	file *ast.File,
	config Config,
	disabled map[string]struct{},
) []Finding {
	var findings []Finding
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !isTestFunction(fn) {
			continue
		}
		if config.SkipExamples && isExampleFunction(fn) {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch stmt := node.(type) {
			case *ast.IfStmt:
				findings = appendConditionalFinding(findings, fset, stmt.If, path, fn.Name.Name, "if", RuleNoIf)
			case *ast.SwitchStmt:
				findings = appendConditionalFinding(findings, fset, stmt.Switch, path, fn.Name.Name, "switch", RuleNoSwitch)
			case *ast.TypeSwitchStmt:
				findings = appendConditionalFinding(findings, fset, stmt.Switch, path, fn.Name.Name, "type switch", RuleNoSwitch)
			case *ast.BranchStmt:
				if stmt.Tok == token.GOTO {
					findings = appendConditionalFinding(findings, fset, stmt.TokPos, path, fn.Name.Name, "goto", RuleNoGoto)
				}
			}
			return true
		})
	}
	return filterDisabled(findings, disabled)
}

func appendConditionalFinding(
	findings []Finding,
	fset *token.FileSet,
	pos token.Pos,
	path string,
	function string,
	kind string,
	ruleID string,
) []Finding {
	position := fset.Position(pos)
	return append(findings, Finding{
		RuleID:   ruleID,
		Path:     path,
		Line:     position.Line,
		Column:   position.Column,
		Function: function,
		Kind:     kind,
		Message:  fmt.Sprintf("%s contains prohibited %s statement", function, kind),
		pos:      pos,
	})
}

func addFinding(conditionals map[conditionalKey]int, whiteBoxFiles *[]WhiteBoxBaseline, finding Finding) {
	switch finding.RuleID {
	case RuleNoIf, RuleNoSwitch, RuleNoGoto:
		conditionals[conditionalKey{path: finding.Path, function: finding.Function, kind: finding.Kind}]++
	case RuleWhiteBoxFileName, RuleWhiteBoxJustification:
		*whiteBoxFiles = append(*whiteBoxFiles, WhiteBoxBaseline{
			Path:    finding.Path,
			Package: finding.Package,
			Reason:  finding.Reason,
		})
	}
}

type conditionalKey struct {
	path     string
	function string
	kind     string
}

func skippedDirectory(name string) bool {
	return name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata"
}

func isTestFunction(fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		return false
	}
	switch {
	case isTestName(fn.Name.Name, "Test"):
		return hasSingleTestingPointerParam(fn, "T")
	case isTestName(fn.Name.Name, "Fuzz"):
		return hasSingleTestingPointerParam(fn, "F")
	case isTestName(fn.Name.Name, "Example"):
		return hasNoParams(fn)
	default:
		return false
	}
}

// isExampleFunction reports whether fn is a testable example: a parameterless
// function whose name the testing package would treat as Example output. It is
// the subset of isTestFunction that Config.SkipExamples exempts.
func isExampleFunction(fn *ast.FuncDecl) bool {
	return fn.Recv == nil && isTestName(fn.Name.Name, "Example") && hasNoParams(fn)
}

func isTestName(name string, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	for _, ch := range name[len(prefix):] {
		return !unicode.IsLower(ch)
	}
	return true
}

func hasSingleTestingPointerParam(fn *ast.FuncDecl, typeName string) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != typeName {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "testing"
}

func hasNoParams(fn *ast.FuncDecl) bool {
	return fn.Type.Params == nil || len(fn.Type.Params.List) == 0
}

func scanWhiteBoxFile(
	path string,
	relativePath string,
	packageName string,
	packagePos token.Pos,
	config Config,
	disabled map[string]struct{},
) []Finding {
	if strings.HasSuffix(packageName, "_test") {
		return nil
	}
	if !strings.HasSuffix(relativePath, "_internal_test.go") {
		return filterDisabled([]Finding{{
			RuleID:  RuleWhiteBoxFileName,
			Path:    relativePath,
			Package: packageName,
			Reason:  "same-package test file is not named *_internal_test.go",
			Message: fmt.Sprintf("%s uses package %s; use package %s_test or rename justified white-box tests to *_internal_test.go",
				relativePath, packageName, packageName),
			pos: packagePos,
		}}, disabled)
	}
	if hasWhiteBoxJustification(path, config.WhiteBoxJustificationPrefix) {
		return nil
	}
	return filterDisabled([]Finding{{
		RuleID:  RuleWhiteBoxJustification,
		Path:    relativePath,
		Package: packageName,
		Reason:  "missing white-box justification comment after package clause",
		Message: fmt.Sprintf("%s is a white-box test and needs a justification comment immediately after the package clause",
			relativePath),
		pos: packagePos,
	}}, disabled)
}

type baselineTracker struct {
	mu           sync.Mutex
	conditionals map[string]int
	whiteBox     map[string]int
}

func newBaselineTracker(baseline Baseline) *baselineTracker {
	conditionals := make(map[string]int, len(baseline.TestConditionals))
	for _, value := range baseline.TestConditionals {
		conditionals[conditionalFindingKey(value.Path, value.Function, value.Kind)] += value.Count
	}
	whiteBox := make(map[string]int, len(baseline.WhiteBoxFiles))
	for _, value := range baseline.WhiteBoxFiles {
		whiteBox[whiteBoxFindingKey(value.Path, value.Package, value.Reason)]++
	}
	return &baselineTracker{conditionals: conditionals, whiteBox: whiteBox}
}

func (b *baselineTracker) Allow(finding Finding) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch finding.RuleID {
	case RuleNoIf, RuleNoSwitch, RuleNoGoto:
		return decrementIfPresent(b.conditionals, conditionalFindingKey(finding.Path, finding.Function, finding.Kind))
	case RuleWhiteBoxFileName, RuleWhiteBoxJustification:
		return decrementIfPresent(b.whiteBox, whiteBoxFindingKey(finding.Path, finding.Package, finding.Reason))
	default:
		return false
	}
}

func decrementIfPresent(values map[string]int, key string) bool {
	count := values[key]
	if count == 0 {
		return false
	}
	values[key] = count - 1
	return true
}

func conditionalFindingKey(path, function, kind string) string {
	return fmt.Sprintf("%s\t%s\t%s", path, function, kind)
}

func whiteBoxFindingKey(path, packageName, reason string) string {
	return fmt.Sprintf("%s\t%s\t%s", path, packageName, reason)
}

func hasWhiteBoxJustification(path string, prefix string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	seenPackage := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !seenPackage {
			seenPackage = strings.HasPrefix(line, "package ")
			continue
		}
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, prefix)
	}
	return false
}

func filterDisabled(findings []Finding, disabled map[string]struct{}) []Finding {
	if len(disabled) == 0 {
		return findings
	}
	result := findings[:0]
	for _, finding := range findings {
		if ruleDisabled(disabled, finding.RuleID) {
			continue
		}
		result = append(result, finding)
	}
	return result
}

func conditionalBaseline(conditionals map[conditionalKey]int) []ConditionalBaseline {
	result := make([]ConditionalBaseline, 0, len(conditionals))
	for key, count := range conditionals {
		result = append(result, ConditionalBaseline{
			Path:     key.path,
			Function: key.function,
			Kind:     key.kind,
			Count:    count,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return compareConditional(result[i], result[j]) < 0
	})
	return result
}

func sortedWhiteBoxBaseline(values []WhiteBoxBaseline) []WhiteBoxBaseline {
	sort.Slice(values, func(i, j int) bool {
		return compareWhiteBox(values[i], values[j]) < 0
	})
	return values
}

func compareConditional(a, b ConditionalBaseline) int {
	return cmp.Or(
		cmp.Compare(a.Path, b.Path),
		cmp.Compare(a.Function, b.Function),
		cmp.Compare(a.Kind, b.Kind),
	)
}

func compareWhiteBox(a, b WhiteBoxBaseline) int {
	return cmp.Or(
		cmp.Compare(a.Path, b.Path),
		cmp.Compare(a.Package, b.Package),
		cmp.Compare(a.Reason, b.Reason),
	)
}

func writeConditionalDiff(builder *strings.Builder, want, got []ConditionalBaseline) {
	wantSet := conditionalSet(want)
	gotSet := conditionalSet(got)
	writeMissing(builder, "test conditional baseline is stale or too high", setDiff(wantSet, gotSet))
	writeMissing(builder, "new test conditional violations", setDiff(gotSet, wantSet))
}

func writeWhiteBoxDiff(builder *strings.Builder, want, got []WhiteBoxBaseline) {
	wantSet := whiteBoxSet(want)
	gotSet := whiteBoxSet(got)
	writeMissing(builder, "white-box baseline is stale or too high", setDiff(wantSet, gotSet))
	writeMissing(builder, "new white-box test violations", setDiff(gotSet, wantSet))
}

func conditionalSet(values []ConditionalBaseline) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[fmt.Sprintf("%s\t%s\t%s\t%d", value.Path, value.Function, value.Kind, value.Count)] = struct{}{}
	}
	return result
}

func whiteBoxSet(values []WhiteBoxBaseline) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[fmt.Sprintf("%s\t%s\t%s", value.Path, value.Package, value.Reason)] = struct{}{}
	}
	return result
}

func setDiff(a, b map[string]struct{}) []string {
	var result []string
	for value := range a {
		if _, ok := b[value]; !ok {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return result
}

func writeMissing(builder *strings.Builder, heading string, values []string) {
	if len(values) == 0 {
		return
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(heading)
	builder.WriteString(":\n")
	for _, value := range values {
		builder.WriteString("  - ")
		builder.WriteString(value)
		builder.WriteByte('\n')
	}
}

func cloneBaseline(baseline Baseline) Baseline {
	return Baseline{
		TestConditionals: slices.Clone(baseline.TestConditionals),
		WhiteBoxFiles:    slices.Clone(baseline.WhiteBoxFiles),
	}
}

func relativePath(root string, filename string) string {
	if root == "" {
		return filepath.ToSlash(filename)
	}
	path, err := filepath.Rel(root, filename)
	if err != nil {
		return filepath.ToSlash(filename)
	}
	return filepath.ToSlash(path)
}
