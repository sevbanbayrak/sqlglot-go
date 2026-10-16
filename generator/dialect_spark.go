package generator

import (
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// generators/spark.py: SparkGenerator on top of Spark2Generator (dialectParents).
func init() {
	registerDialectTypeMapping("spark", map[exp.DType]string{
		exp.DTypeMoney: "DECIMAL", exp.DTypeSmallMoney: "DECIMAL", exp.DTypeUUID: "STRING",
		exp.DTypeTimestampLtz: "TIMESTAMP_LTZ", exp.DTypeTimestampNtz: "TIMESTAMP_NTZ",
	})
	// generators/spark.py:148-150 exp.X: None entries; IgnoreNulls reverts to the base generator
	// (generators/spark.py:155-156) and AnyValue to the function fallback (:168-169).
	registerDialectDispatchRemovals("spark", exp.KindAnyValue, exp.KindDateDiff, exp.KindWith, exp.KindIgnoreNulls)
	registerDialectDispatch("spark", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindArrayConstructCompact: func(g *Generator, e exp.Expression) string {
			args := make([]any, 0, len(e.Expressions()))
			for _, x := range e.Expressions() {
				args = append(args, x)
			}
			return g.funcCall("ARRAY_COMPACT", []any{g.funcCall("ARRAY", args, "(", ")", true)}, "(", ")", true)
		},
		exp.KindArrayInsert:        prestoCall("ARRAY_INSERT", "this", "position", "expression"),
		exp.KindArrayAppend:        func(g *Generator, e exp.Expression) string { return g.arrayAppendSQL(e, "ARRAY_APPEND", false) },
		exp.KindArrayPrepend:       func(g *Generator, e exp.Expression) string { return g.arrayAppendSQL(e, "ARRAY_PREPEND", false) },
		exp.KindBitwiseAndAgg:      prestoRename("BIT_AND"),
		exp.KindBitwiseOrAgg:       prestoRename("BIT_OR"),
		exp.KindBitwiseXorAgg:      prestoRename("BIT_XOR"),
		exp.KindBitwiseCount:       prestoRename("BIT_COUNT"),
		exp.KindCurrentVersion:     prestoRename("VERSION"),
		exp.KindDateFromUnixDate:   prestoRename("DATE_FROM_UNIX_DATE"),
		exp.KindEndsWith:           prestoRename("ENDSWITH"),
		exp.KindJSONKeys:           prestoRename("JSON_OBJECT_KEYS"),
		exp.KindSafeAdd:            prestoRename("TRY_ADD"),
		exp.KindSafeDivide:         prestoRename("TRY_DIVIDE"),
		exp.KindSafeMultiply:       prestoRename("TRY_MULTIPLY"),
		exp.KindSafeSubtract:       prestoRename("TRY_SUBTRACT"),
		exp.KindStartsWith:         prestoRename("STARTSWITH"),
		exp.KindTimestampFromParts: prestoRename("MAKE_TIMESTAMP"),
		exp.KindDatetimeAdd:        (*Generator).dateDeltaToBinaryIntervalOpSQL,
		exp.KindDatetimeSub:        (*Generator).dateDeltaToBinaryIntervalOpSQL,
		exp.KindTimeAdd:            (*Generator).dateDeltaToBinaryIntervalOpSQL,
		exp.KindTimeSub:            (*Generator).dateDeltaToBinaryIntervalOpSQL,
		exp.KindTimestampSub:       (*Generator).dateDeltaToBinaryIntervalOpSQL,
		exp.KindTsOrDsAdd:          (*Generator).sparkDateAddSQL,
		exp.KindTimestampAdd:       (*Generator).sparkDateAddSQL,
		exp.KindDatetimeDiff:       (*Generator).timestampDiffSQL,
		exp.KindTimestampDiff:      (*Generator).timestampDiffSQL,
		exp.KindGroupConcat: func(g *Generator, e exp.Expression) string {
			return g.groupConcatStandardSQL(e, false)
		},
		exp.KindPartitionedByProperty: func(g *Generator, e exp.Expression) string {
			var sqls []any
			for _, x := range e.This().Expressions() {
				if x.Kind() == exp.KindLiteral {
					x = exp.ToIdentifier(x.Name())
				}
				sqls = append(sqls, g.gen(x))
			}
			return "PARTITIONED BY " + g.wrap(g.expressions(exprsOptions{sqls: sqls, skipFirst: true}))
		},
		exp.KindTryCast: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("safe")) {
				return g.castSQLWithPrefix(e, "TRY_")
			}
			return g.spark2CastSQL(e)
		},
		exp.KindBracket: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("safe")) {
				return g.bracketToElementAtSQLNamed(e, "TRY_ELEMENT_AT")
			}
			if safe, ok := e.Arg("safe").(bool); ok && !safe {
				return g.bracketToElementAtSQL(e)
			}
			return g.bracketSQL(e)
		},
		exp.KindComputedColumnConstraint: func(g *Generator, e exp.Expression) string {
			return "GENERATED ALWAYS AS (" + g.sqlKey(e, "this") + ")"
		},
		exp.KindDateDiff: func(g *Generator, e exp.Expression) string {
			end, start := e.Arg("this"), e.Arg("expression")
			if truthy(e.Arg("unit")) {
				return g.funcCall("DATEDIFF", []any{unitToVarValue(asExpression(e.Arg("unit"))), start, end}, "(", ")", true)
			}
			return g.funcCall("DATEDIFF", []any{end, start}, "(", ")", true)
		},
		exp.KindCreate: func(g *Generator, e exp.Expression) string {
			out := transforms.RemoveUniqueConstraints(e)
			out = transforms.CtasWithTmpTablesToCreateTmpView(out, spark2TemporaryStorageProvider)
			out = transforms.MovePartitionedByToSchemaColumns(out)
			return g.createSQL(out)
		},
		exp.KindDeclare: func(g *Generator, e exp.Expression) string {
			replace := ""
			if truthy(e.Arg("replace")) {
				replace = "OR REPLACE "
			}
			return "DECLARE " + replace + g.expressions(exprsOptions{expression: e, flat: true})
		},
		exp.KindDeclareItem: func(g *Generator, e exp.Expression) string {
			sql := g.expressions(exprsOptions{expression: e, key: "this", flat: true})
			if kind := asExpression(e.Arg("kind")); kind != nil {
				if kind.Kind() == exp.KindSchema {
					sql += " TABLE"
				}
				sql += " " + g.gen(kind)
			}
			if dflt := g.sqlKey(e, "default"); dflt != "" {
				sql += " = " + dflt
			}
			return sql
		},
		exp.KindReadParquet: func(g *Generator, e exp.Expression) string {
			if len(e.Expressions()) != 1 {
				g.unsupported("READ_PARQUET with multiple arguments is not supported")
				return ""
			}
			return "parquet.`" + e.Expressions()[0].Name() + "`"
		},
	})
}

// sparkDateAddSQL ports _dateadd_sql (generators/spark.py:31-55); the TsOrDsAdd return type
// comes from the return_type arg, DATE by default.
func (g *Generator) sparkDateAddSQL(e exp.Expression) string {
	unit := asExpression(e.Arg("unit"))
	if unit == nil || (e.Kind() == exp.KindTsOrDsAdd && strings.ToUpper(e.Text("unit")) == "DAY") {
		return g.funcCall("DATE_ADD", []any{e.This(), e.Expr()}, "(", ")", true)
	}
	this := g.funcCall("DATE_ADD", []any{unitToVarValue(unit), e.Expr(), e.This()}, "(", ")", true)
	if e.Kind() == exp.KindTsOrDsAdd {
		returnType := asExpression(e.Arg("return_type"))
		if returnType == nil {
			returnType = exp.DTypeDate.IntoExpr(nil)
		}
		if !exp.IsType(returnType, exp.DTypeTimestamp) && !exp.IsType(returnType, exp.DTypeDatetime) {
			this = "CAST(" + this + " AS " + g.gen(returnType) + ")"
		}
	}
	return this
}

// dateDeltaToBinaryIntervalOpSQL ports date_delta_to_binary_interval_op(cast=False)
// (dialect.py:2029-2054).
func (g *Generator) dateDeltaToBinaryIntervalOpSQL(e exp.Expression) string {
	op := " - "
	switch e.Kind() {
	case exp.KindDatetimeAdd, exp.KindTimeAdd, exp.KindTimestampAdd, exp.KindDateAdd, exp.KindTsOrDsAdd:
		op = " + "
	}
	expr := e.Expr()
	interval := expr
	if expr == nil || expr.Kind() != exp.KindInterval {
		interval = exp.Interval(exp.Args{"this": expr, "unit": unitToVarValue(asExpression(e.Arg("unit")))})
	}
	return g.gen(e.This()) + op + g.gen(interval)
}

// timestampDiffSQL ports timestampdiff_sql (dialect.py:2519-2520).
func (g *Generator) timestampDiffSQL(e exp.Expression) string {
	return g.funcCall("TIMESTAMPDIFF", []any{e.Arg("unit"), e.Expr(), e.This()}, "(", ")", true)
}

// arrayAppendSQL ports array_append_sql (dialect.py:1370-1414).
func (g *Generator) arrayAppendSQL(e exp.Expression, name string, swap bool) string {
	this, element := e.This(), e.Expr()
	args := []any{this, element}
	if swap {
		args = []any{element, this}
	}
	funcSQL := g.funcCall(name, args, "(", ")", true)
	source := truthy(e.Arg("null_propagation"))
	if source == g.dialect.ArrayFuncsPropagatesNulls {
		return funcSQL
	}
	if source {
		return g.gen(exp.If(exp.Args{"this": exp.Is(exp.Args{"this": this, "expression": exp.Null()}), "true": exp.Null(), "false": funcSQL}))
	}
	coalesced := exp.Coalesce(exp.Args{"expressions": []exp.Expression{this, exp.Array(exp.Args{"expressions": []exp.Expression{}})}})
	args = []any{coalesced, element}
	if swap {
		args = []any{element, coalesced}
	}
	return g.funcCall(name, args, "(", ")", true)
}

// bracketToElementAtSQLNamed is bracket_to_element_at_sql with the Spark TRY_ELEMENT_AT name.
func (g *Generator) bracketToElementAtSQLNamed(e exp.Expression, name string) string {
	sql := g.bracketToElementAtSQL(e)
	return name + sql[len("ELEMENT_AT"):]
}
