package main

// White-box testing required: run contains the command's exit-code contract and
// is intentionally unexported; exercising it directly avoids subprocess noise.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun_MissingBaselineCleanRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sample_test.go"), `package sample_test

import "testing"

func TestSample(t *testing.T) {
	t.Helper()
}
`)

	code := run([]string{"-root", dir})

	assertEqual(t, code, 0)
}

func TestRun_MissingBaselineReportsViolations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sample_test.go"), `package sample_test

import "testing"

func TestSample(t *testing.T) {
	if true {
		t.Fatal("bad")
	}
}
`)

	code := run([]string{"-root", dir})

	assertEqual(t, code, 1)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0o600)
	assertNoError(t, err)
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
