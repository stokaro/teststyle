package baselineextra // want "baselineextra_test.go uses package baselineextra; use package baselineextra_test or rename justified white-box tests to \\*_internal_test.go"

import "testing"

func TestBad(t *testing.T) {
	if true {
	}
	if false { // want "TestBad contains prohibited if statement"
	}
}
