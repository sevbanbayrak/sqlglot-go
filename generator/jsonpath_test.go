package generator_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	"github.com/sevbanbayrak/sqlglot-go/generator"
	"github.com/sevbanbayrak/sqlglot-go/jsonpath"
)

// jsonPathSQL ports `parse(selector).sql()` (tests/test_jsonpath.py) with the base dialect.
func jsonPathSQL(t *testing.T, selector string) string {
	t.Helper()
	path, err := jsonpath.Parse(selector, jsonpath.Options{})
	if err != nil {
		t.Fatalf("Parse(%q): %v", selector, err)
	}
	sql, err := generator.New(dialects.Base(), generator.Options{}).Generate(path)
	if err != nil {
		t.Fatalf("Generate(%q): %v", selector, err)
	}
	return sql
}

func TestJSONPathIdentity(t *testing.T) {
	for _, tc := range []struct{ selector, expected string }{
		{"$.select", "$.select"},
		{"$[(@.length-1)]", "$[(@.length-1)]"},
		{"$[((@.length-1))]", "$[((@.length-1))]"},
	} {
		if got := jsonPathSQL(t, tc.selector); got != "'"+tc.expected+"'" {
			t.Errorf("%s: got %s, want '%s'", tc.selector, got, tc.expected)
		}
	}
}

// sqlglot json path generator rewrites to a normal form
var ctsOverrides = map[string]string{
	"$.\u263a":            "$[\"\u263a\"]",
	"$['a',1]":            "$[\"a\",1]",
	"$[*,'a']":            "$[*,\"a\"]",
	"$..['a','d']":        "$..[\"a\",\"d\"]",
	"$[1, ?@.a=='b', 1:]": "$[1,?@.a=='b', 1:]",
	"$[\"a\"]":            "$.a",
	"$[\"c\"]":            "$.c",
	"$['a']":              "$.a",
	"$['c']":              "$.c",
	"$[' ']":              "$[\" \"]",
	"$['\\'']":            "$[\"'\"]",
	"$['\\\\']":           "$[\"\\\\\"]",
	"$['\\/']":            "$[\"\\/\"]",
	"$['\\b']":            "$[\"\\b\"]",
	"$['\\f']":            "$[\"\\f\"]",
	"$['\\n']":            "$[\"\\n\"]",
	"$['\\r']":            "$[\"\\r\"]",
	"$['\\t']":            "$[\"\\t\"]",
	"$['\\u263A']":        "$[\"\\u263A\"]",
	"$['\\u263a']":        "$[\"\\u263a\"]",
	"$['\\uD834\\uDD1E']": "$[\"\\uD834\\uDD1E\"]",
	"$['\\uD83D\\uDE00']": "$[\"\\uD83D\\uDE00\"]",
	"$['']":               "$[\"\"]",
	"$[? @.a]":            "$[?@.a]",
	"$[?\n@.a]":           "$[?@.a]",
	"$[?\t@.a]":           "$[?@.a]",
	"$[?\r@.a]":           "$[?@.a]",
	"$[? (@.a)]":          "$[?(@.a)]",
	"$[?\n(@.a)]":         "$[?(@.a)]",
	"$[?\t(@.a)]":         "$[?(@.a)]",
	"$[?\r(@.a)]":         "$[?(@.a)]",
	"$[ ?@.a]":            "$[?@.a]",
	"$[\n?@.a]":           "$[?@.a]",
	"$[\t?@.a]":           "$[?@.a]",
	"$[\r?@.a]":           "$[?@.a]",
	"$ ['a']":             "$.a",
	"$\n['a']":            "$.a",
	"$\t['a']":            "$.a",
	"$\r['a']":            "$.a",
	"$['a'] ['b']":        "$.a.b",
	"$['a'] \n['b']":      "$.a.b",
	"$['a'] \t['b']":      "$.a.b",
	"$['a'] \r['b']":      "$.a.b",
	"$ .a":                "$.a",
	"$\n.a":               "$.a",
	"$\t.a":               "$.a",
	"$\r.a":               "$.a",
	"$[ 'a']":             "$.a",
	"$[\n'a']":            "$.a",
	"$[\t'a']":            "$.a",
	"$[\r'a']":            "$.a",
	"$['a' ]":             "$.a",
	"$['a'\n]":            "$.a",
	"$['a'\t]":            "$.a",
	"$['a'\r]":            "$.a",
	"$['a' ,'b']":         "$[\"a\",\"b\"]",
	"$['a'\n,'b']":        "$[\"a\",\"b\"]",
	"$['a'\t,'b']":        "$[\"a\",\"b\"]",
	"$['a'\r,'b']":        "$[\"a\",\"b\"]",
	"$['a', 'b']":         "$[\"a\",\"b\"]",
	"$['a',\n'b']":        "$[\"a\",\"b\"]",
	"$['a',\t'b']":        "$[\"a\",\"b\"]",
	"$['a',\r'b']":        "$[\"a\",\"b\"]",
	"$[1 :5:2]":           "$[1:5:2]",
	"$[1\n:5:2]":          "$[1:5:2]",
	"$[1\t:5:2]":          "$[1:5:2]",
	"$[1\r:5:2]":          "$[1:5:2]",
	"$[1: 5:2]":           "$[1:5:2]",
	"$[1:\n5:2]":          "$[1:5:2]",
	"$[1:\t5:2]":          "$[1:5:2]",
	"$[1:\r5:2]":          "$[1:5:2]",
	"$[1:5 :2]":           "$[1:5:2]",
	"$[1:5\n:2]":          "$[1:5:2]",
	"$[1:5\t:2]":          "$[1:5:2]",
	"$[1:5\r:2]":          "$[1:5:2]",
	"$[1:5: 2]":           "$[1:5:2]",
	"$[1:5:\n2]":          "$[1:5:2]",
	"$[1:5:\t2]":          "$[1:5:2]",
	"$[1:5:\r2]":          "$[1:5:2]",
}

func TestJSONPathCTSFile(t *testing.T) {
	raw, err := os.ReadFile("../jsonpath/testdata/cts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Tests []struct {
			Name            string `json:"name"`
			Selector        string `json:"selector"`
			InvalidSelector bool   `json:"invalid_selector"`
		} `json:"tests"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}

	for _, test := range fixture.Tests {
		if test.InvalidSelector {
			// Upstream only requires that ParseError/TokenError is the failure mode.
			_, _ = jsonpath.Parse(test.Selector, jsonpath.Options{})
			continue
		}
		expected, ok := ctsOverrides[test.Selector]
		if !ok {
			expected = test.Selector
		}
		if got := jsonPathSQL(t, test.Selector); got != "'"+expected+"'" {
			t.Errorf("%q /* %s */: got %s, want '%s'", test.Selector, test.Name, got, expected)
		}
	}
}
