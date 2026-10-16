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
	// generator.py:4821 kwarg_sql
	dispatch[expressions.KindKwarg] = func(g *Generator, e expressions.Expression) string { return g.binary(e, "=>") }
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

// arrayAnySQL ports arrayany_sql (generator.py:5252-5266) for dialects that cannot implement
// ARRAY_ANY (CAN_IMPLEMENT_ARRAY_ANY is false for every ported dialect).
func (g *Generator) arrayAnySQL(e expressions.Expression) string {
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
