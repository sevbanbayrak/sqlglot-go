package generator

import (
	"strconv"
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
)

// dialectJSONPathParts ports SUPPORTED_JSON_PATH_PARTS (generator.py:517): the parts a dialect
// keeps in TRANSFORMS (dialect.py:302-305 pops the rest). Lookups walk dialectParents; a chain
// with no entry supports every part.
var dialectJSONPathParts = map[string]map[exp.Kind]bool{}

func registerJSONPathParts(name string, kinds ...exp.Kind) {
	dialectJSONPathParts[name] = map[exp.Kind]bool{}
	for _, kind := range kinds {
		dialectJSONPathParts[name][kind] = true
	}
}

func (g *Generator) jsonPathPartSupported(kind exp.Kind) bool {
	for key := g.overrideKey(); key != ""; key = dialectParents[key] {
		if supported, ok := dialectJSONPathParts[key]; ok {
			return supported[kind]
		}
	}
	return true
}

// jsonPathSQL ports jsonpath_sql (generator.py:3795-3801).
func (g *Generator) jsonPathSQL(e exp.Expression) string {
	path := strings.TrimLeft(g.expressions(exprsOptions{expression: e, emptySep: true, flat: true}), ".")
	if g.dialect.QuoteJSONPath {
		path = g.dialect.QuoteStart + path + g.dialect.QuoteEnd
	}
	return path
}

// jsonPathPart ports json_path_part (generator.py:3803-3822).
func (g *Generator) jsonPathPart(v any) string {
	switch tv := v.(type) {
	case exp.Expression:
		transform := g.lookupDispatch(tv.Kind())
		if transform == nil {
			g.unsupported("Unsupported JSONPathPart type " + exp.ClassName(tv.Kind()))
			return ""
		}
		return transform(g, tv)
	case int:
		return strconv.Itoa(tv)
	}

	expression, _ := v.(string)
	// The generator always quotes keys using brackets here; only BigQuery toggles
	// _quote_json_path_key_using_brackets (generators/bigquery.py:238-243).
	if g.dialect.JSONPathSingleQuoteEscape {
		// Upstream discards the escaped value (generator.py:3816-3817); kept as is.
		return `\'` + expression + `\'`
	}
	return `"` + strings.ReplaceAll(expression, `"`, `\"`) + `"`
}

// jsonPathKeySQL ports _jsonpathkey_sql (generator.py:5350-5374).
func (g *Generator) jsonPathKeySQL(e exp.Expression) string {
	if wildcard := e.This(); wildcard != nil && wildcard.Kind() == exp.KindJSONPathWildcard {
		if this := g.jsonPathPart(wildcard); this != "" {
			return "." + this
		}
		return ""
	}

	quoted := truthy(e.Arg("quoted"))
	key, _ := e.Arg("this").(string)
	if !(quoted && g.dialect.JSONPathKeyQuotedForcesBrackets) && g.dialect.SafeJSONPathKeyRE.MatchString(key) {
		return "." + key
	}

	key = g.jsonPathPart(key)

	if quoted && g.dialect.QuoteJSONPath {
		// The whole path is rendered as a single quoted string literal, so the bracketed key
		// (which may itself contain backslash-escaped quotes, e.g. ["x \"y\"z"]) must be
		// escaped again for the outer string literal (-> ["x \\"y\\"z"]).
		key = g.escapeStr(key)
	}

	if g.dialect.JSONPathBracketedKeySupported {
		return "[" + key + "]"
	}
	return "." + key
}

// jsonPathSubscriptSQL ports _jsonpathsubscript_sql (generator.py:5376-5378).
func (g *Generator) jsonPathSubscriptSQL(e exp.Expression) string {
	if this := g.jsonPathPart(e.Arg("this")); this != "" {
		return "[" + this + "]"
	}
	return ""
}

// jsonExtractSegments ports json_extract_segments (dialect.py:2250-2277).
func (g *Generator) jsonExtractSegments(e exp.Expression, name string, quotedIndex bool, op string) string {
	path := e.Expr()
	if path == nil || path.Kind() != exp.KindJSONPath {
		return g.funcCall(name, jsonExtractArgs(e), "(", ")", true)
	}

	var segments []string
	for _, segment := range path.Expressions() {
		sql := g.gen(segment)
		if sql == "" {
			continue
		}
		if exp.IsJSONPathPart(segment.Kind()) && (quotedIndex || segment.Kind() != exp.KindJSONPathSubscript) {
			if truthy(segment.Arg("quoted")) {
				sql = g.escapeStr(sql)
			}
			sql = g.dialect.QuoteStart + sql + g.dialect.QuoteEnd
		}
		segments = append(segments, sql)
	}

	if op != "" {
		return strings.Join(append([]string{g.sqlKey(e, "this")}, segments...), " "+op+" ")
	}
	args := []any{e.Arg("this")}
	for _, segment := range segments {
		args = append(args, segment)
	}
	return g.funcCall(name, args, "(", ")", true)
}

// jsonPathKeyOnlyName ports json_path_key_only_name (dialect.py:2280-2284).
func (g *Generator) jsonPathKeyOnlyName(e exp.Expression) string {
	if this := e.This(); this != nil && this.Kind() == exp.KindJSONPathWildcard {
		g.unsupported("Unsupported wildcard in JSONPathKey expression")
	}
	return e.Name()
}

// JSON_PATH_PART_TRANSFORMS (jsonpath.py:219-238).
func init() {
	dispatch[exp.KindJSONPath] = (*Generator).jsonPathSQL
	dispatch[exp.KindJSONPathFilter] = func(_ *Generator, e exp.Expression) string { return "?" + e.Text("this") }
	dispatch[exp.KindJSONPathKey] = (*Generator).jsonPathKeySQL
	dispatch[exp.KindJSONPathRecursive] = func(g *Generator, e exp.Expression) string { return ".." + g.gen(e.Arg("this")) }
	dispatch[exp.KindJSONPathRoot] = func(*Generator, exp.Expression) string { return "$" }
	dispatch[exp.KindJSONPathScript] = func(_ *Generator, e exp.Expression) string { return "(" + e.Text("this") }
	dispatch[exp.KindJSONPathSelector] = func(g *Generator, e exp.Expression) string {
		return "[" + g.jsonPathPart(e.Arg("this")) + "]"
	}
	dispatch[exp.KindJSONPathSlice] = func(g *Generator, e exp.Expression) string {
		var parts []string
		for _, key := range []string{"start", "end", "step"} {
			p := e.Arg(key)
			if p == nil {
				continue
			}
			if p == any(false) {
				parts = append(parts, "")
			} else {
				parts = append(parts, g.jsonPathPart(p))
			}
		}
		return strings.Join(parts, ":")
	}
	dispatch[exp.KindJSONPathSubscript] = (*Generator).jsonPathSubscriptSQL
	dispatch[exp.KindJSONPathUnion] = func(g *Generator, e exp.Expression) string {
		var parts []string
		for _, p := range listFromValue(e.Arg("expressions")) {
			parts = append(parts, g.jsonPathPart(p))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	dispatch[exp.KindJSONPathWildcard] = func(*Generator, exp.Expression) string { return "*" }
}
