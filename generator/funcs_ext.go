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
	// generator.py:4959-4972 querytransform_sql
	dispatch[expressions.KindQueryTransform] = func(g *Generator, e expressions.Expression) string {
		args := make([]any, 0, len(e.Expressions()))
		for _, x := range e.Expressions() {
			args = append(args, x)
		}
		sql := g.funcCall("TRANSFORM", args, "(", ")", true)
		for _, part := range []struct{ key, prefix string }{{"row_format_before", " "}, {"record_writer", " RECORDWRITER "}} {
			if s := g.sqlKey(e, part.key); s != "" {
				sql += part.prefix + s
			}
		}
		sql += " USING " + g.sqlKey(e, "command_script")
		for _, part := range []struct{ key, prefix string }{{"schema", " AS "}, {"row_format_after", " "}, {"record_reader", " RECORDREADER "}} {
			if s := g.sqlKey(e, part.key); s != "" {
				sql += part.prefix + s
			}
		}
		return sql
	}
	// generator.py:4061-4068 fromiso8601*_sql
	dispatch[expressions.KindFromISO8601Date] = func(g *Generator, e expressions.Expression) string {
		return g.gen(expressions.Cast(expressions.Args{"this": e.This(), "to": expressions.DTypeDate.IntoExpr(nil)}))
	}
	for _, kind := range []expressions.Kind{expressions.KindFromISO8601Timestamp, expressions.KindFromISO8601TimestampNanos} {
		dispatch[kind] = func(g *Generator, e expressions.Expression) string {
			return g.gen(expressions.Cast(expressions.Args{"this": e.This(), "to": expressions.DTypeTimestampTz.IntoExpr(nil)}))
		}
	}
	// generator.py:6143-6167 decodecase_sql
	dispatch[expressions.KindDecodeCase] = func(g *Generator, e expressions.Expression) string {
		exprs := e.Expressions()
		if !g.dialect.DecodeCaseUnsupported {
			args := make([]any, 0, len(exprs))
			for _, x := range exprs {
				args = append(args, x)
			}
			return g.funcCall("DECODE", args, "(", ")", true)
		}
		decodeExpr, rest := exprs[0], exprs[1:]
		var ifs []expressions.Expression
		for i := 0; i+1 < len(rest); i += 2 {
			search, result := rest[i], rest[i+1]
			var cond expressions.Expression
			switch {
			case search.Kind() == expressions.KindLiteral:
				cond = expressions.EQ(expressions.Args{"this": decodeExpr.Copy(), "expression": search})
			case search.Kind() == expressions.KindNull:
				cond = expressions.Is(expressions.Args{"this": decodeExpr.Copy(), "expression": expressions.Null()})
			default:
				if search.Is(expressions.TraitBinary) {
					search = expressions.Paren(expressions.Args{"this": search})
				}
				cond = expressions.Or(expressions.Args{
					"this": expressions.EQ(expressions.Args{"this": decodeExpr.Copy(), "expression": search}),
					"expression": expressions.And(expressions.Args{
						"this":       expressions.Is(expressions.Args{"this": decodeExpr.Copy(), "expression": expressions.Null()}),
						"expression": expressions.Is(expressions.Args{"this": search.Copy(), "expression": expressions.Null()}),
					}),
				})
			}
			ifs = append(ifs, expressions.If(expressions.Args{"this": cond, "true": result}))
		}
		args := expressions.Args{"ifs": ifs}
		if len(rest)%2 == 1 {
			args["default"] = rest[len(rest)-1]
		}
		return g.gen(expressions.Case(args))
	}
	// generator.py:4613-4618 try_sql
	dispatch[expressions.KindTry] = func(g *Generator, e expressions.Expression) string {
		if g.dialect.TryUnsupported {
			g.unsupported("Unsupported TRY function")
			return g.sqlKey(e, "this")
		}
		return g.funcCall("TRY", []any{e.This()}, "(", ")", true)
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
