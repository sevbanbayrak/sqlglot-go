package generator

import (
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// generators/databricks.py: DatabricksGenerator on top of SparkGenerator (dialectParents).
func init() {
	registerDialectTypeMapping("databricks", map[exp.DType]string{exp.DTypeNull: "VOID"})
	// generators/databricks.py:63-64 exp.X: None entries.
	registerDialectDispatchRemovals("databricks", exp.KindRegexpLike, exp.KindTryCast)
	regr := func(g *Generator, e exp.Expression) string { return g.databricksRegrSQL(e) }
	registerDialectDispatch("databricks", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindCurrentVersion: func(*Generator, exp.Expression) string { return "CURRENT_VERSION()" },
		exp.KindDateAdd:        databricksDateDeltaSQL("DATEADD"),
		exp.KindDateDiff:       databricksDateDeltaSQL("DATEDIFF"),
		exp.KindDatetimeAdd: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TIMESTAMPADD", []any{e.Arg("unit"), e.Expr(), e.This()}, "(", ")", true)
		},
		exp.KindDatetimeSub: func(g *Generator, e exp.Expression) string {
			negated := exp.Mul(exp.Args{"this": e.Expr(), "expression": exp.LiteralNumber(-1)})
			return g.funcCall("TIMESTAMPADD", []any{e.Arg("unit"), negated, e.This()}, "(", ")", true)
		},
		// timestamptrunc_sql() (dialect.py:1700-1709).
		exp.KindDatetimeTrunc: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_TRUNC", []any{weekstartUnitToStr(e), e.This()}, "(", ")", true)
		},
		exp.KindSelect: func(g *Generator, e exp.Expression) string {
			out := transforms.EliminateDistinctOn(e)
			out, err := transforms.UnnestToExplode(out, true)
			if err != nil {
				g.unsupported(err.Error())
			}
			out = transforms.AnyToExists(out)
			if out.Kind() != exp.KindSelect {
				return g.gen(out)
			}
			return g.selectSQL(out)
		},
		exp.KindJSONExtract: func(g *Generator, e exp.Expression) string {
			return g.sqlKey(e, "this") + ":" + g.sqlKey(e, "expression")
		},
		exp.KindJSONPathRoot: func(_ *Generator, e exp.Expression) string {
			if parent := e.Parent(); parent != nil && parent.Parent() != nil && parent.Parent().Kind() == exp.KindJSONExtractScalar {
				return "$"
			}
			return ""
		},
		exp.KindJSONPath: func(g *Generator, e exp.Expression) string {
			path := g.jsonPathSQL(e)
			if parent := e.Parent(); parent != nil && parent.Kind() == exp.KindJSONExtractScalar {
				return g.dialect.QuoteStart + path + g.dialect.QuoteEnd
			}
			return path
		},
		exp.KindToChar: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("is_numeric")) {
				return g.castSQL(exp.Cast(exp.Args{"this": e.This(), "to": exp.DTypeText.IntoExpr(nil)}))
			}
			return g.functionFallbackSQL(e)
		},
		exp.KindCurrentCatalog: func(*Generator, exp.Expression) string { return "CURRENT_CATALOG()" },
		exp.KindRegrAvgx:       regr,
		exp.KindRegrSxx:        regr,
		exp.KindRegrSyy:        regr,
		exp.KindCreate: func(g *Generator, e exp.Expression) string {
			return g.databricksCreateSQL(sparkCreatePreprocess(e))
		},
		exp.KindColumnDef:     (*Generator).databricksColumnDefSQL,
		exp.KindTimeseriesKey: func(g *Generator, e exp.Expression) string { return g.sqlKey(e, "this") + " TIMESERIES" },
		exp.KindUniform:       (*Generator).databricksUniformSQL,
		exp.KindClusterProperty: func(g *Generator, e exp.Expression) string {
			this := g.sqlKey(e, "this")
			if this == "" {
				this = "(" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
			}
			return "CLUSTER BY " + this
		},
	})
}

// databricksDateDeltaSQL ports date_delta_sql (dialect.py:2014-2026) without the TsOrDsAdd cast.
func databricksDateDeltaSQL(name string) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		return g.funcCall(name, []any{unitToVarValue(asExpression(e.Arg("unit"))), e.Expr(), e.This()}, "(", ")", true)
	}
}

// weekstartUnitToStr ports weekstart_unit_to_str (dialect.py) with the DAY default.
func weekstartUnitToStr(e exp.Expression) exp.Expression {
	if unit := asExpression(e.Arg("unit")); unit != nil && unit.Kind() == exp.KindWeekStart {
		return exp.LiteralString("WEEK(" + unit.Name() + ")")
	}
	return unitToStr(e)
}

// databricksCreateSQL ports create_sql (generators/databricks.py:74-83) on a copy, since
// Generate stays side-effect free: a FUNCTION returning a TABLE wraps its body in RETURN.
func (g *Generator) databricksCreateSQL(e exp.Expression) string {
	body := e.Expr()
	if body != nil && body.Kind() != exp.KindReturn && e.Text("kind") == "FUNCTION" {
		for _, p := range e.FindAll(exp.KindReturnsProperty) {
			if truthy(p.Arg("is_table")) {
				e = e.Copy()
				e.Set("expression", exp.New(exp.KindReturn, exp.Args{"this": e.Expr()}))
				break
			}
		}
	}
	return g.createSQL(e)
}

// databricksColumnDefSQL ports columndef_sql (generators/databricks.py:85-96) on a copy: only
// BIGINT generated identity constraints are supported.
func (g *Generator) databricksColumnDefSQL(e exp.Expression) string {
	if kind := asExpression(e.Arg("kind")); kind != nil && kind.Kind() == exp.KindDataType && e.Find(exp.KindGeneratedAsIdentityColumnConstraint) != nil {
		if dtype, ok := kind.Arg("this").(exp.DType); ok && exp.IntegerTypes[dtype] {
			e = e.Copy()
			e.Set("kind", exp.DTypeBigInt.IntoExpr(nil))
		}
	}
	return dialectDispatch["hive"][exp.KindColumnDef](g, e)
}

// databricksUniformSQL ports uniform_sql (generators/databricks.py:111-119).
func (g *Generator) databricksUniformSQL(e exp.Expression) string {
	seed := e.Arg("seed")
	// From Snowflake UNIFORM(min, max, gen) as RANDOM(), RANDOM(seed), or constant value -> Extract seed
	if gen := asExpression(e.Arg("gen")); gen != nil {
		seed = gen.Arg("this")
	}
	return g.funcCall("UNIFORM", []any{e.This(), e.Expr(), seed}, "(", ")", true)
}

// databricksRegrSQL ports _regr_sql (generators/databricks.py:121-126).
func (g *Generator) databricksRegrSQL(e exp.Expression) string {
	name := exp.SQLName(e.Kind())
	x := e.Expr()
	if x != nil && x.Kind() == exp.KindDistinct {
		args := []any{exp.Distinct(exp.Args{"expressions": []exp.Expression{e.This()}})}
		for _, arg := range x.Expressions() {
			args = append(args, arg)
		}
		return g.funcCall(name, args, "(", ")", true)
	}
	return g.funcCall(name, []any{e.This(), x}, "(", ")", true)
}
