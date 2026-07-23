package teststyle_test

import (
	"path/filepath"
	"testing"

	"github.com/stokaro/teststyle"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzerReportsViolations(t *testing.T) {
	testdata := analysistest.TestData()
	analyzer, err := teststyle.NewAnalyzer(teststyle.Config{
		Root: filepath.Join(testdata, "src"),
	})

	assertNoError(t, err)
	analysistest.Run(t, testdata, analyzer, "violations")
}

func TestAnalyzerBaselineDoesNotHideNewViolations(t *testing.T) {
	testdata := analysistest.TestData()
	analyzer, err := teststyle.NewAnalyzer(teststyle.Config{
		Root:         filepath.Join(testdata, "src"),
		BaselinePath: filepath.Join(testdata, "baseline.json"),
	})

	assertNoError(t, err)
	analysistest.Run(t, testdata, analyzer, "baselineextra")
}

func TestAnalyzerMissingBaselineUsesEmptyBaseline(t *testing.T) {
	testdata := analysistest.TestData()
	analyzer, err := teststyle.NewAnalyzer(teststyle.Config{
		Root:         filepath.Join(testdata, "src"),
		BaselinePath: filepath.Join(testdata, "missing-baseline.json"),
	})

	assertNoError(t, err)
	analysistest.Run(t, testdata, analyzer, "violations")
}
