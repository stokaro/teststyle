# teststyle

`teststyle` is a Go linter for declarative tests and black-box-by-default test
packages.

It started as Ptah's internal test-style auditor and is now a reusable module
with:

- a standalone CLI: `go tool teststyle`
- a `go/analysis` analyzer
- a `golangci-lint` module-plugin entrypoint
- JSON baseline support for incremental cleanup in existing repositories

## Rules

| Rule ID | Behavior |
| --- | --- |
| `teststyle-no-if` | Disallows `if` statements in `Test*`, `Fuzz*`, and `Example*` functions. |
| `teststyle-no-switch` | Disallows expression switches and type switches in test functions. |
| `teststyle-no-goto` | Disallows `goto` statements in test functions. |
| `teststyle-whitebox-filename` | Requires same-package test files to be named `*_internal_test.go`. |
| `teststyle-whitebox-justification` | Requires a white-box justification comment immediately after the package clause. |

`for` loops are allowed so table-driven tests can stay compact. Helper functions
may contain conditionals, but helpers should not hide assertion-selection logic.

## Bad And Good Examples

### `teststyle-no-if`

Bad:

```go
func TestParseConfig(t *testing.T) {
	got, err := ParseConfig("missing.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "app" {
		t.Fatalf("got %q", got.Name)
	}
}
```

Good:

```go
func TestParseConfig(t *testing.T) {
	got, err := ParseConfig("app.yaml")

	assertNoError(t, err)
	assertEqual(t, got.Name, "app")
}
```

### `teststyle-no-switch`

Bad:

```go
func TestRenderDialect(t *testing.T) {
	switch dialect {
	case "postgres":
		assertPostgres(t)
	default:
		assertGeneric(t)
	}
}
```

Good:

```go
func TestRenderDialect_Postgres(t *testing.T) {
	assertPostgres(t)
}

func TestRenderDialect_Generic(t *testing.T) {
	assertGeneric(t)
}
```

### `teststyle-no-goto`

Bad:

```go
func TestCleanup(t *testing.T) {
	goto cleanup

cleanup:
	assertClean(t)
}
```

Good:

```go
func TestCleanup(t *testing.T) {
	assertClean(t)
}
```

### `teststyle-whitebox-filename`

Bad:

```go
package config

func TestParseDefaults(t *testing.T) {}
```

Good:

```go
package config_test

func TestParseDefaults(t *testing.T) {}
```

White-box exception:

```go
package config

// White-box testing required: parseDefaults is an unexported state-machine
// helper whose edge cases cannot be isolated through the exported API.

func TestParseDefaults(t *testing.T) {}
```

The file must be named `*_internal_test.go`.

### `teststyle-whitebox-justification`

Bad:

```go
package config

import "testing"

func TestParseDefaults(t *testing.T) {}
```

Good:

```go
package config
// White-box testing required: parseDefaults is an unexported state-machine
// helper whose edge cases cannot be isolated through the exported API.

import "testing"

func TestParseDefaults(t *testing.T) {}
```

## Standalone Usage

Add the tool to your module:

```bash
go get -tool github.com/stokaro/teststyle/cmd/teststyle
```

Check a repository against an existing baseline:

```bash
go tool teststyle -baseline .teststyle-baseline.json -root .
```

Write a baseline during initial adoption:

```bash
go tool teststyle -write-baseline -baseline .teststyle-baseline.json -root .
```

Disable individual rules:

```bash
go tool teststyle -disable teststyle-no-if,teststyle-no-switch
```

## golangci-lint Module Plugin

Create a custom golangci-lint build config:

```yaml
version: v2.3.0
plugins:
  - module: github.com/stokaro/teststyle
    import: github.com/stokaro/teststyle/golangci
    version: v0.1.0
```

Build the custom binary:

```bash
golangci-lint custom
```

Enable the plugin in `.golangci.yml`:

```yaml
version: "2"

linters:
  default: none
  enable:
    - teststyle
  settings:
    custom:
      teststyle:
        type: module
        description: Declarative Go test style linter.
        settings:
          baseline_path: .teststyle-baseline.json
          root: .
```

The module-plugin path uses the same analyzer and rule IDs as the standalone
CLI. Baseline matching is count-aware, so a baseline entry for one `if` does not
hide a second newly introduced `if`.

Complete example configs are available in `examples/golangci/`.

## Baseline Format

```json
{
  "test_conditionals": [
    {
      "path": "parser/parser_test.go",
      "function": "TestParse",
      "kind": "if",
      "count": 1
    }
  ],
  "white_box_files": [
    {
      "path": "parser/parser_test.go",
      "package": "parser",
      "reason": "same-package test file is not named *_internal_test.go"
    }
  ]
}
```
