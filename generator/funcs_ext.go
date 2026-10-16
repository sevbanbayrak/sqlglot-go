package generator

import (
	"fmt"

	"github.com/sevbanbayrak/sqlglot-go/expressions"
)

func init() {
	dispatch[expressions.KindPad] = (*Generator).padSQL
	dispatch[expressions.KindNvl2] = (*Generator).nvl2SQL
	dispatch[expressions.KindArrayAny] = (*Generator).arrayAnySQL
	dispatch[expressions.KindConcatWs] = (*Generator).concatWsSQL
	dispatch[expressions.KindMedian] = (*Generator).medianSQL
	dispatch[expressions.KindTsOrDsToDate] = (*Generator).tsOrDsToDateSQL
	// generator.py:4821 kwarg_sql
	dispatch[expressions.KindKwarg] = func(g *Generator, e expressions.Expression) string { return g.binary(e, "=>") }
	// generator.py:4816-4819 joinhint_sql
	dispatch[expressions.KindJoinHint] = func(g *Generator, e expressions.Expression) string {
		return g.sqlKey(e, "this") + "(" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
	}
	// generator.py:4056-4059 fromtimezone_sql
	dispatch[expressions.KindFromTimeZone] = func(g *Generator, e expressions.Expression) string {
		return g.sqlKey(e, "this") + " AT TIME ZONE " + g.sqlKey(e, "zone") + " AT TIME ZONE 'UTC'"
	}
}

// unsupportedArgs ports the unsupported_args decorator (generator.py): warn for each named arg
// the target dialect cannot express.
func (g *Generator) unsupportedArgs(e expressions.Expression, keys ...string) {
	for _, key := range keys {
		if e.Arg(key) != nil {
			g.unsupported(fmt.Sprintf("Argument '%s' is not supported for expression '%s' when targeting %s.", key, expressions.ClassName(e.Kind()), g.dialect.Name))
		}
	}
}

// padSQL ports pad_sql (generator.py:5572-5579).
func (g *Generator) padSQL(e expressions.Expression) string {
	prefix := "R"
	if boolValue(e.Arg("is_left")) {
		prefix = "L"
	}
	var fill any = e.Arg("fill_pattern")
	if fill == nil && g.dialect.PadFillPatternIsRequired {
		fill = "' '"
	}
	return g.funcCall(prefix+"PAD", []any{e.This(), e.Expr(), fill}, "(", ")", true)
}

// nvl2SQL ports nvl2_sql (generator.py:5023-5036).
func (g *Generator) nvl2SQL(e expressions.Expression) string {
	if g.dialect.Nvl2Supported {
		return g.functionFallbackSQL(e)
	}
	isNull := expressions.Is(expressions.Args{"this": e.This(), "expression": expressions.Null()})
	args := expressions.Args{"ifs": []expressions.Expression{
		expressions.If(expressions.Args{"this": expressions.Not(expressions.Args{"this": isNull}), "true": e.Arg("true")}),
	}}
	if elseCond := e.Arg("false"); elseCond != nil {
		args["default"] = elseCond
	}
	return g.gen(expressions.Case(args))
}

// arrayAnySQL ports arrayany_sql (generator.py:5252-5266).
func (g *Generator) arrayAnySQL(e expressions.Expression) string {
	if g.dialect.CanImplementArrayAny {
		filtered := expressions.New(expressions.KindArrayFilter, expressions.Args{"this": e.This(), "expression": e.Expr()})
		filteredNotEmpty := expressions.NEQ(expressions.Args{"this": expressions.New(expressions.KindArraySize, expressions.Args{"this": filtered}), "expression": expressions.LiteralNumber(0)})
		originalIsEmpty := expressions.EQ(expressions.Args{"this": expressions.New(expressions.KindArraySize, expressions.Args{"this": e.This().Copy()}), "expression": expressions.LiteralNumber(0)})
		return g.gen(expressions.Paren(expressions.Args{"this": expressions.Or(expressions.Args{"this": originalIsEmpty, "expression": filteredNotEmpty})}))
	}
	if g.dialect.Name != "" {
		g.unsupported("ARRAY_ANY is unsupported")
	}
	return g.functionFallbackSQL(e)
}

// concatWsSQL ports concatws_sql (generator.py:3726-3741).
func (g *Generator) concatWsSQL(e expressions.Expression) string {
	if g.dialect.ConcatWsCoalesce && !boolValue(e.Arg("coalesce")) {
		var conds []expressions.Expression
		for _, arg := range e.Expressions() {
			conds = append(conds, expressions.Is(expressions.Args{"this": arg, "expression": expressions.Null()}))
		}
		e.Set("coalesce", true)
		return g.gen(expressions.Case(expressions.Args{
			"ifs":     []expressions.Expression{expressions.If(expressions.Args{"this": expressions.Or_(conds...), "true": expressions.Null()})},
			"default": e,
		}))
	}
	exprs := e.Expressions()
	var first any
	if len(exprs) > 0 {
		first = exprs[0]
	}
	return g.funcCall("CONCAT_WS", append([]any{first}, g.convertConcatArgs(e)...), "(", ")", true)
}

func init() {
	// varmap_sql (generator.py): MAP(keys, values) in the base dialect.
	dispatch[expressions.KindVarMap] = func(g *Generator, e expressions.Expression) string {
		return g.funcCall("MAP", []any{e.Arg("keys"), e.Arg("values")}, "(", ")", true)
	}
}

// medianSQL ports median_sql (generator.py:5862-5868).
func (g *Generator) medianSQL(e expressions.Expression) string {
	if !g.dialect.SupportsMedian {
		return g.gen(expressions.New(expressions.KindPercentileCont, expressions.Args{"this": e.This(), "expression": expressions.LiteralNumber(0.5)}))
	}
	return g.functionFallbackSQL(e)
}

// formatTime is Generator.format_time (generator.py:4712-4722) over the dialect's FormatTime.
func (g *Generator) formatTime(e expressions.Expression) expressions.Expression {
	format := asExpression(e.Arg("format"))
	if format != nil && format.IsString() && g.dialect.FormatTime != nil {
		return expressions.LiteralString(g.dialect.FormatTime(format.Name()))
	}
	return format
}

// tsOrDsToDateSQL ports tsordstodate_sql (generator.py:5202-5220); the this.is_type(DATE) test
// is reduced to an explicit DATE cast, since annotate_types never runs here.
func (g *Generator) tsOrDsToDateSQL(e expressions.Expression) string {
	this := e.This()
	safe := e.Arg("safe")
	if format := g.formatTime(e); format != nil && format.IsString() && format.Name() != g.dialect.TimeFormat && format.Name() != g.dialect.DateFormat {
		strToTime := expressions.New(expressions.KindStrToTime, expressions.Args{"this": this, "format": e.Arg("format"), "safe": safe})
		return g.gen(expressions.New(expressions.KindCast, expressions.Args{"this": strToTime, "to": expressions.DTypeDate.IntoExpr(nil)}))
	}
	if this.Kind() == expressions.KindTsOrDsToDate || (this.Kind() == expressions.KindCast && expressions.IsType(asExpression(this.Arg("to")), expressions.DTypeDate)) {
		return g.gen(this)
	}
	kind := expressions.KindCast
	if boolValue(safe) {
		kind = expressions.KindTryCast
	}
	return g.gen(expressions.New(kind, expressions.Args{"this": this, "to": expressions.DTypeDate.IntoExpr(nil)}))
}
