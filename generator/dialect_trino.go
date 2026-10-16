package generator

import (
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
)

// generators/trino.py:22-53.
func init() {
	registerDialectDispatch("trino", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindCurrentVersion:            prestoRename("VERSION"),
		exp.KindFromISO8601TimestampNanos: prestoRename("FROM_ISO8601_TIMESTAMP_NANOS"),
		exp.KindGroupConcat:               func(g *Generator, e exp.Expression) string { return g.groupConcatStandardSQL(e, true) },
		exp.KindTrim:                      (*Generator).trimSQLStandard,
		exp.KindLocationProperty:          func(g *Generator, e exp.Expression) string { return "LOCATION=" + g.sqlKey(e, "this") },
		exp.KindJSONExtract:               (*Generator).trinoJSONExtractSQL,
		exp.KindArrayUniqueAgg:            func(g *Generator, e exp.Expression) string { return "ARRAY_AGG(DISTINCT " + g.sqlKey(e, "this") + ")" },
		exp.KindDeclare: func(g *Generator, e exp.Expression) string {
			replace := ""
			if truthy(e.Arg("replace")) {
				replace = "OR REPLACE "
			}
			return "DECLARE " + replace + g.expressions(exprsOptions{expression: e, flat: true})
		},
		// generator.py:6078-6090 with Trino's DECLARE_DEFAULT_ASSIGNMENT = "DEFAULT".
		exp.KindDeclareItem: func(g *Generator, e exp.Expression) string {
			sql := g.expressions(exprsOptions{expression: e, key: "this", flat: true})
			if kind := asExpression(e.Arg("kind")); kind != nil {
				if kind.Kind() == exp.KindSchema {
					sql += " TABLE"
				}
				sql += " " + g.gen(kind)
			}
			if dflt := g.sqlKey(e, "default"); dflt != "" {
				sql += " DEFAULT " + dflt
			}
			return sql
		},
		exp.KindStabilityProperty: func(_ *Generator, e exp.Expression) string {
			if e.Name() == "IMMUTABLE" {
				return "DETERMINISTIC"
			}
			return "NOT DETERMINISTIC"
		},
	})
	registerDialectPropertyLocations("trino", map[exp.Kind]propertyLocation{exp.KindLocationProperty: propertyLocationPostWith})
	// generators/trino.py:49-53.
	registerJSONPathParts("trino", exp.KindJSONPathKey, exp.KindJSONPathRoot, exp.KindJSONPathSubscript)
}

// groupConcatStandardSQL ports groupconcat_sql (dialects/dialect.py:2539-2587) with the LISTAGG
// defaults; onOverflow is the on_overflow parameter.
func (g *Generator) groupConcatStandardSQL(e exp.Expression, onOverflow bool) string {
	e = e.Copy()
	this := e.This()
	separator := g.sqlKey(e, "separator")
	if separator == "" {
		separator = "','"
	}
	if overflow := g.sqlKey(e, "on_overflow"); onOverflow && overflow != "" {
		separator += " ON OVERFLOW " + overflow
	}
	var limit exp.Expression
	if this != nil && this.Kind() == exp.KindLimit && this.This() != nil {
		limit = this
		this = this.This().Pop()
	}
	var order exp.Expression
	if this != nil {
		order = this.Find(exp.KindOrder)
	}
	if order != nil && order.This() != nil {
		this = order.This().Pop()
	}
	args := g.formatArgs([]any{this, separator}, ", ")
	if limit != nil {
		args += g.gen(limit)
	}
	sql := g.funcCall("LISTAGG", []any{args}, "(", ")", true)
	if order != nil {
		sql += " WITHIN GROUP (" + strings.TrimSpace(g.gen(order)) + ")"
	}
	return sql
}

// generators/trino.py:135-156.
func (g *Generator) trinoJSONExtractSQL(e exp.Expression) string {
	if !boolValue(e.Arg("json_query")) {
		args := []any{e.Arg("this"), e.Arg("expression")}
		for _, x := range e.Expressions() {
			args = append(args, x)
		}
		return g.funcCall("JSON_EXTRACT", args, "(", ")", true)
	}
	path := g.sqlKey(e, "expression")
	for _, key := range []string{"option", "quote", "on_condition"} {
		if value := g.sqlKey(e, key); value != "" {
			path += " " + value
		}
	}
	return g.funcCall("JSON_QUERY", []any{e.Arg("this"), path}, "(", ")", true)
}
