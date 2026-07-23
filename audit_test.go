package teststyle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stokaro/teststyle"
)

func TestScanReportsConditionalsAndWhiteBoxViolations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sample/blackbox_test.go", `package sample_test

import "testing"

func TestDeclarative(t *testing.T) {}
`)
	writeFile(t, dir, "sample/mixed_test.go", `package sample

import "testing"

func TestBad(t *testing.T) {
	if true {}
	switch 1 { case 1: }
	goto done
done:
}
`)
	writeFile(t, dir, "sample/valid_internal_test.go", `package sample
// White-box testing required: verifies unexported parser state that is not observable through the exported API.

import "testing"

func TestInternal(t *testing.T) {}
`)
	writeFile(t, dir, "sample/missing_internal_test.go", `package sample

import "testing"

func TestMissingComment(t *testing.T) {}
`)

	got, err := teststyle.Scan(dir)

	assertNoError(t, err)
	assertDeepEqual(t, got, teststyle.Baseline{
		TestConditionals: []teststyle.ConditionalBaseline{
			{Path: "sample/mixed_test.go", Function: "TestBad", Kind: "goto", Count: 1},
			{Path: "sample/mixed_test.go", Function: "TestBad", Kind: "if", Count: 1},
			{Path: "sample/mixed_test.go", Function: "TestBad", Kind: "switch", Count: 1},
		},
		WhiteBoxFiles: []teststyle.WhiteBoxBaseline{
			{Path: "sample/missing_internal_test.go", Package: "sample", Reason: "missing white-box justification comment after package clause"},
			{Path: "sample/mixed_test.go", Package: "sample", Reason: "same-package test file is not named *_internal_test.go"},
		},
	})
}

func TestScanWithConfigDisablesRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sample/mixed_test.go", `package sample

import "testing"

func TestBad(t *testing.T) {
	if true {}
	switch 1 { case 1: }
	goto done
done:
}
`)

	got, err := teststyle.ScanWithConfig(dir, teststyle.Config{
		DisabledRules: []string{
			teststyle.RuleNoIf,
			teststyle.RuleWhiteBoxFileName,
		},
	})

	assertNoError(t, err)
	assertDeepEqual(t, got, teststyle.Baseline{
		TestConditionals: []teststyle.ConditionalBaseline{
			{Path: "sample/mixed_test.go", Function: "TestBad", Kind: "goto", Count: 1},
			{Path: "sample/mixed_test.go", Function: "TestBad", Kind: "switch", Count: 1},
		},
	})
}

func TestBaselineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	baseline := teststyle.Baseline{
		TestConditionals: []teststyle.ConditionalBaseline{
			{Path: "a_test.go", Function: "TestA", Kind: "if", Count: 1},
		},
		WhiteBoxFiles: []teststyle.WhiteBoxBaseline{
			{Path: "b_test.go", Package: "b", Reason: "same-package test file is not named *_internal_test.go"},
		},
	}

	err := teststyle.WriteBaseline(path, baseline)
	got, readErr := teststyle.ReadBaseline(path)

	assertNoError(t, err)
	assertNoError(t, readErr)
	assertDeepEqual(t, got, baseline)
	assertEqual(t, teststyle.Diff(baseline, got), "")
}

func TestDiffReportsNewAndStaleEntries(t *testing.T) {
	want := teststyle.Baseline{
		TestConditionals: []teststyle.ConditionalBaseline{
			{Path: "old_test.go", Function: "TestOld", Kind: "if", Count: 1},
		},
	}
	got := teststyle.Baseline{
		TestConditionals: []teststyle.ConditionalBaseline{
			{Path: "new_test.go", Function: "TestNew", Kind: "switch", Count: 1},
		},
	}

	diff := teststyle.Diff(want, got)

	assertContains(t, diff, "test conditional baseline is stale or too high")
	assertContains(t, diff, "old_test.go")
	assertContains(t, diff, "new test conditional violations")
	assertContains(t, diff, "new_test.go")
}

func TestDiffDoesNotMutateInputs(t *testing.T) {
	want := teststyle.Baseline{
		TestConditionals: []teststyle.ConditionalBaseline{
			{Path: "z_test.go", Function: "TestZ", Kind: "if", Count: 1},
			{Path: "a_test.go", Function: "TestA", Kind: "switch", Count: 1},
		},
	}
	got := teststyle.Baseline{
		WhiteBoxFiles: []teststyle.WhiteBoxBaseline{
			{Path: "z_internal_test.go", Package: "sample", Reason: "missing white-box justification comment after package clause"},
			{Path: "a_test.go", Package: "sample", Reason: "same-package test file is not named *_internal_test.go"},
		},
	}
	wantBefore := want
	gotBefore := got

	_ = teststyle.Diff(want, got)

	assertDeepEqual(t, want, wantBefore)
	assertDeepEqual(t, got, gotBefore)
}

func writeFile(t *testing.T, root string, relativePath string, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	assertNoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	assertNoError(t, os.WriteFile(path, []byte(data), 0o600))
}
