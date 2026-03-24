---
name: go-unit-test
description: Write Go unit tests
allowed-tools: Bash, Glob, Grep, Read, Edit, Write
---

# Writing Go unit tests

Wherever practical, make tests table-driven and use `reflect.DeepEqual` to compare whole expected values rather than individual fields.

## Table-driven tests

Use table-driven tests wherever possible. One table of test cases per test function.
Each test case struct should have fields for the inputs and the expected outputs.
A test case can have multiple input fields and multiple expected output fields.
Prefer `expect` as the prefix for expected-value fields (e.g. `expect`, `expectErr`).

```go
func TestFoo(t *testing.T) {
	tests := []struct {
		name   string
		input  InputType
		expect OutputType
	}{
		{
			name:   "description of case",
			input:  InputType{...},
			expect: OutputType{...},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Foo(tc.input)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}
```

## Comparing outputs

Do not compare struct fields individually. Put the complete expected value in the test table and compare the whole thing with `reflect.DeepEqual`.

Bad:
```go
if got.X != 1 { t.Errorf(...) }
if got.Y != 2 { t.Errorf(...) }
```

Good:
```go
if !reflect.DeepEqual(got, tc.expect) {
	t.Errorf("got  %+v\nwant %+v", got, tc.expect)
}
```

## Error handling in tests

Use `t.Fatalf` for errors that prevent further testing (setup failures, parse errors).
Use `t.Errorf` for assertion failures (DeepEqual comparisons).

For tests that expect errors, add an `expectErr bool` field to the table:
```go
if tc.expectErr {
	if err == nil {
		t.Fatalf("expected error")
	}
	return
}
if err != nil {
	t.Fatalf("unexpected error: %v", err)
}
```

## No third-party test libraries

Use only the standard `testing` package and `reflect.DeepEqual`. Do not use testify or other assertion libraries.
