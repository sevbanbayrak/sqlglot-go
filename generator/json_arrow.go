package generator

import "github.com/sevbanbayrak/sqlglot-go/expressions"

// jsonExtractArgs returns the positional argument list for the *function* rendering of an
// exp.JSONExtract/JSONExtractScalar node: `this`, `expression`, then the variadic `expressions`
// tail (only JSONExtract keeps it, build_extract_json_with_path parser.py:110-122).
func jsonExtractArgs(e expressions.Expression) []any {
	return append([]any{e.Arg("this"), e.Arg("expression")}, listFromValue(e.Arg("expressions"))...)
}

// postgresJSONExtractSQL ports _json_extract_sql (generators/postgres.py:183-198).
func (g *Generator) postgresJSONExtractSQL(e expressions.Expression, name, op string) string {
	path := e.Expr()
	// Single non-literal segment: render as infix, not JSON_EXTRACT_PATH[_TEXT] (jsonb-unsafe).
	if (path == nil || (path.Kind() != expressions.KindJSONPath && path.Kind() != expressions.KindVariadic)) && len(listFromValue(e.Arg("expressions"))) == 0 {
		return g.binary(e, op)
	}

	if truthy(e.Arg("only_json_types")) {
		return g.jsonExtractSegments(e, name, false, op)
	}
	return g.jsonExtractSegments(e, name, true, "")
}

// jsonExtractSQL is the base default for exp.JSONExtract (function_fallback_sql).
func (g *Generator) jsonExtractSQL(e expressions.Expression) string {
	return g.funcCall("JSON_EXTRACT", jsonExtractArgs(e), "(", ")", true)
}

// jsonExtractScalarSQL is the base default for exp.JSONExtractScalar (function_fallback_sql).
func (g *Generator) jsonExtractScalarSQL(e expressions.Expression) string {
	return g.funcCall("JSON_EXTRACT_SCALAR", jsonExtractArgs(e), "(", ")", true)
}

// arrowJSONExtractSQL ports arrow_json_extract_sql (dialect.py:1250-1255): a string-literal
// `this` is wrapped in CAST(... AS JSON) when the dialect requires a JSON operand.
func (g *Generator) arrowJSONExtractSQL(e expressions.Expression) string {
	if this := e.This(); g.dialect.JSONTypeRequiredForExtraction && this != nil && this.Kind() == expressions.KindLiteral && this.IsString() {
		cast := expressions.Cast(expressions.Args{
			"this": this,
			"to":   expressions.DataType(expressions.Args{"this": expressions.DTypeJSON}),
		})
		e.Set("this", cast)
	}
	if e.Kind() == expressions.KindJSONExtract {
		return g.binary(e, "->")
	}
	return g.binary(e, "->>")
}

// jsonbExtractSQL ports base/mysql's default (functionFallbackSQL - JSONB_EXTRACT) and
// postgres's `lambda self, e: self.binary(e, "#>")` (generators/postgres.py:324).
func (g *Generator) jsonbExtractSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return g.binary(e, "#>")
	}
	return g.funcCall("JSONB_EXTRACT", []any{e.Arg("this"), e.Arg("expression")}, "(", ")", true)
}

// jsonbExtractScalarSQL ports base/mysql's default (functionFallbackSQL - JSONB_EXTRACT_SCALAR)
// and postgres's `lambda self, e: self.binary(e, "#>>")` (generators/postgres.py:325).
func (g *Generator) jsonbExtractScalarSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return g.binary(e, "#>>")
	}
	return g.funcCall("JSONB_EXTRACT_SCALAR", []any{e.Arg("this"), e.Arg("expression")}, "(", ")", true)
}

func init() {
	dispatch[expressions.KindJSONExtract] = (*Generator).jsonExtractSQL
	dispatch[expressions.KindJSONExtractScalar] = (*Generator).jsonExtractScalarSQL
	dispatch[expressions.KindJSONBExtract] = (*Generator).jsonbExtractSQL
	dispatch[expressions.KindJSONBExtractScalar] = (*Generator).jsonbExtractScalarSQL
}
