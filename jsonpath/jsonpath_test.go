package jsonpath_test

import (
	"testing"

	sqlerrors "github.com/sevbanbayrak/sqlglot-go/errors"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/jsonpath"
)

// Ports tests/test_jsonpath.py; the .sql() round-trips (test_identity, test_cts_file) live in
// generator/jsonpath_test.go because they need the generator.

func part(kind exp.Kind, args exp.Args) exp.Expression { return exp.New(kind, args) }

func parse(t *testing.T, path string) exp.Expression {
	t.Helper()
	e, err := jsonpath.Parse(path, jsonpath.Options{})
	if err != nil {
		t.Fatalf("Parse(%q): %v", path, err)
	}
	return e
}

func TestJSONPath(t *testing.T) {
	wildcard := func() exp.Expression { return part(exp.KindJSONPathWildcard, nil) }
	key := func(this any) exp.Expression { return part(exp.KindJSONPathKey, exp.Args{"this": this}) }
	expected := []exp.Expression{
		part(exp.KindJSONPathRoot, nil),
		key(wildcard()),
		key("a"),
		part(exp.KindJSONPathSubscript, exp.Args{"this": 0}),
		key("x"),
		part(exp.KindJSONPathUnion, exp.Args{"expressions": []any{wildcard(), "y", 1}}),
		key("z"),
		part(exp.KindJSONPathSelector, exp.Args{"this": part(exp.KindJSONPathFilter, exp.Args{"this": "(@.a == 'b'), 1:"})}),
		part(exp.KindJSONPathSubscript, exp.Args{"this": part(exp.KindJSONPathSlice, exp.Args{"start": 1, "end": 5})}),
		part(exp.KindJSONPathUnion, exp.Args{"expressions": []any{1, part(exp.KindJSONPathFilter, exp.Args{"this": "@.a"})}}),
		part(exp.KindJSONPathSelector, exp.Args{"this": part(exp.KindJSONPathScript, exp.Args{"this": "@.x)"})}),
	}
	want := part(exp.KindJSONPath, exp.Args{"expressions": expected})
	got := parse(t, "$.*.a[0]['x'][*, 'y', 1].z[?(@.a == 'b'), 1:][1:5][1,?@.a][(@.x)]")
	if !got.Equal(want) || got.ToS() != want.ToS() {
		t.Fatalf("got:\n%s\nwant:\n%s", got.ToS(), want.ToS())
	}
}

func TestUnclosedFilterRaisesParseError(t *testing.T) {
	// A filter/script that runs off the end of the path without a closing
	// bracket must raise ParseError, not IndexError.
	for _, selector := range []string{"$[?(@.x", "$[(@.x", "$[?(1"} {
		_, err := jsonpath.Parse(selector, jsonpath.Options{})
		if _, ok := err.(*sqlerrors.ParseError); !ok {
			t.Errorf("Parse(%q) error = %v, want *ParseError", selector, err)
		}
	}
}

func TestUnionPreservesFalseyMembers(t *testing.T) {
	union := func(items ...any) exp.Expression {
		return part(exp.KindJSONPathUnion, exp.Args{"expressions": items})
	}
	for _, tc := range []struct {
		selector string
		want     exp.Expression
	}{
		{"$[1,0]", union(1, 0)},
		{`$["a",""]`, union("a", "")},
		{`$["",1]`, union("", 1)},
		{"$[0]", part(exp.KindJSONPathSubscript, exp.Args{"this": 0})},
	} {
		parts := parse(t, tc.selector).Expressions()
		if got := parts[len(parts)-1]; !got.Equal(tc.want) || got.ToS() != tc.want.ToS() {
			t.Errorf("%s: got %s, want %s", tc.selector, got.ToS(), tc.want.ToS())
		}
	}
}

func TestOutputName(t *testing.T) {
	if got := parse(t, "$.a.b").OutputName(); got != "b" {
		t.Errorf("OutputName = %q, want b", got)
	}
	if got := parse(t, "$[0]").OutputName(); got != "" {
		t.Errorf("OutputName = %q, want empty", got)
	}
}
