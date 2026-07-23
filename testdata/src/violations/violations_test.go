package violations // want "violations_test.go uses package violations; use package violations_test or rename justified white-box tests to \\*_internal_test.go"

import "testing"

func TestBad(t *testing.T) {
	if true { // want "TestBad contains prohibited if statement"
	}
	switch 1 { // want "TestBad contains prohibited switch statement"
	case 1:
	}
	goto done // want "TestBad contains prohibited goto statement"
done:
}
