package generator

import (
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// MySQL overrides: the generators/mysql.py TRANSFORMS entries and *_sql methods that the shared
// tables do not already carry under a g.dialect.Name == "mysql" branch.
func init() {
	rename := func(name string) func(*Generator, exp.Expression) string {
		return func(g *Generator, e exp.Expression) string {
			return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
		}
	}
	removeThenRename := func(name string) func(*Generator, exp.Expression) string {
		return func(g *Generator, e exp.Expression) string {
			removeTsOrDsToDate(e, "this")
			return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
		}
	}
	removeThenFallback := func(g *Generator, e exp.Expression) string {
		removeTsOrDsToDate(e, "this")
		return g.functionFallbackSQL(e)
	}
	castTo := func(e exp.Expression, t exp.DType) exp.Expression {
		return exp.New(exp.KindCast, exp.Args{"this": e, "to": t.IntoExpr(nil)})
	}
	unsupportedFallback := func(message string) func(*Generator, exp.Expression) string {
		return func(g *Generator, e exp.Expression) string {
			g.unsupported(message)
			return g.functionFallbackSQL(e)
		}
	}
	dateAddInterval := func(kind string) func(*Generator, exp.Expression) string {
		// dialect.py:1691-1697 date_add_interval_sql("DATE", kind)
		return func(g *Generator, e exp.Expression) string {
			interval := exp.Interval(exp.Args{"this": e.Expr(), "unit": unitToVarValue(asExpression(e.Arg("unit")))})
			return "DATE_" + kind + "(" + g.sqlKey(e, "this") + ", " + g.gen(interval) + ")"
		}
	}
	table := map[exp.Kind]func(*Generator, exp.Expression) string{
		// generators/mysql.py:168.
		exp.KindJSONExtractScalar: (*Generator).arrowJSONExtractSQL,

		exp.KindArrayAgg: rename("GROUP_CONCAT"),
		exp.KindArray:    unsupportedFallback("Arrays are not supported by MySQL"),
		// generators/mysql.py:222.
		exp.KindUtcTimestamp:     prestoRename("UTC_TIMESTAMP"),
		exp.KindArrayContainsAll: unsupportedFallback("Array operations are not supported by MySQL"),
		exp.KindArrayContainedBy: unsupportedFallback("Array operations are not supported by MySQL"),
		exp.KindDateDiff: func(g *Generator, e exp.Expression) string {
			removeTsOrDsToDate(e, "this", "expression")
			return g.funcCall("DATEDIFF", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		exp.KindDateAdd: func(g *Generator, e exp.Expression) string {
			removeTsOrDsToDate(e, "this")
			return g.mysqlDateAddSQL("ADD", e)
		},
		exp.KindDateSub: func(g *Generator, e exp.Expression) string {
			removeTsOrDsToDate(e, "this")
			return g.mysqlDateAddSQL("SUB", e)
		},
		exp.KindDateStrToDate: func(g *Generator, e exp.Expression) string { return g.gen(castTo(e.This(), exp.DTypeDate)) },
		exp.KindDateTrunc:     (*Generator).mysqlDateTruncSQL,
		exp.KindDay:           removeThenFallback,
		exp.KindMonth:         removeThenFallback,
		exp.KindYear:          removeThenFallback,
		exp.KindWeek:          removeThenFallback,
		exp.KindDayOfMonth:    removeThenRename("DAYOFMONTH"),
		exp.KindDayOfWeek:     removeThenRename("DAYOFWEEK"),
		exp.KindDayOfYear:     removeThenRename("DAYOFYEAR"),
		exp.KindWeekOfYear:    removeThenRename("WEEKOFYEAR"),
		exp.KindILike:         (*Generator).noILikeSQL,
		exp.KindNullSafeEQ:    func(g *Generator, e exp.Expression) string { return g.binary(e, "<=>") },
		exp.KindNullSafeNEQ:   func(g *Generator, e exp.Expression) string { return "NOT " + g.binary(e, "<=>") },
		exp.KindStrToDate:     (*Generator).mysqlStrToDateSQL,
		exp.KindStrToTime:     (*Generator).mysqlStrToDateSQL,
		exp.KindTimestampAdd:  dateAddInterval("ADD"),
		exp.KindTimestampSub:  dateAddInterval("SUB"),
		exp.KindTimestampDiff: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TIMESTAMPDIFF", []any{unitToVarValue(asExpression(e.Arg("unit"))), e.Expr(), e.This()}, "(", ")", true)
		},
		exp.KindTimeToStr: func(g *Generator, e exp.Expression) string {
			removeTsOrDsToDate(e, "this")
			return g.funcCall("DATE_FORMAT", []any{e.This(), mysqlFormatTime(e)}, "(", ")", true)
		},
		exp.KindTsOrDsAdd: func(g *Generator, e exp.Expression) string { return g.mysqlDateAddSQL("ADD", e) },
		exp.KindTsOrDsDiff: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATEDIFF", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		exp.KindTsOrDsToDate: func(g *Generator, e exp.Expression) string {
			// generators/mysql.py:107-109 _ts_or_ds_to_date_sql
			if e.Arg("format") != nil {
				return g.mysqlStrToDateSQL(e)
			}
			return g.funcCall("DATE", []any{e.This()}, "(", ")", true)
		},
		exp.KindUnixToTime: func(g *Generator, e exp.Expression) string {
			// generators/mysql.py:60-72 _unix_to_time_sql
			scale := asExpression(e.Arg("scale"))
			var timestamp exp.Expression = e.This()
			if scale != nil && !(scale.IsNumber() && scale.Name() == "0") {
				pow := exp.FunctionByName["POW"]([]exp.Expression{exp.LiteralNumber(10), scale})
				timestamp = exp.New(exp.KindDiv, exp.Args{"this": timestamp, "expression": pow})
			}
			return g.funcCall("FROM_UNIXTIME", []any{timestamp, mysqlFormatTime(e)}, "(", ")", true)
		},
		exp.KindIsAscii: func(g *Generator, e exp.Expression) string {
			return "REGEXP_LIKE(" + g.sqlKey(e, "this") + ", '^[[:ascii:]]*$')"
		},
		exp.KindDPipe: func(g *Generator, e exp.Expression) string {
			return g.funcCall("CONCAT", flattenBinary(e), "(", ")", true)
		},
		exp.KindExtract: func(g *Generator, e exp.Expression) string {
			if strings.EqualFold(e.Name(), "epoch") {
				return g.funcCall("UNIX_TIMESTAMP", []any{e.Expr()}, "(", ")", true)
			}
			return g.extractSQL(e)
		},
		exp.KindTimestampTrunc: (*Generator).mysqlTimestampTruncSQL,
		exp.KindMakeInterval:   (*Generator).mysqlMakeIntervalSQL,
		exp.KindSelect: func(g *Generator, e exp.Expression) string {
			// generators/mysql.py:189-197 Select preprocess chain.
			out := transforms.EliminateDistinctOn(e)
			out = transforms.EliminateSemiAndAntiJoins(out)
			out = transforms.EliminateQualify(out)
			out = transforms.EliminateFullOuterJoin(out)
			out = transforms.UnnestGenerateDateArrayUsingRecursiveCte(out)
			if out.Kind() != exp.KindSelect {
				return g.gen(out)
			}
			return g.selectSQL(out)
		},
	}
	registerDialectDispatch("mysql", table)
}

// removeTsOrDsToDate ports remove_ts_or_ds_to_date (dialect.py:1997-2011): a format-less
// TsOrDsToDate/TsOrDsToTimestamp argument is replaced by its operand.
func removeTsOrDsToDate(e exp.Expression, keys ...string) {
	for _, key := range keys {
		arg := asExpression(e.Arg(key))
		if arg == nil || (arg.Kind() != exp.KindTsOrDsToDate && arg.Kind() != exp.KindTsOrDsToTimestamp) || arg.Arg("format") != nil {
			continue
		}
		e.Set(key, arg.This())
	}
}

// flattenBinary ports Expression.flatten for a same-kind binary chain: the leaves in DFS order.
func flattenBinary(e exp.Expression) []any {
	var out []any
	var walk func(x exp.Expression)
	walk = func(x exp.Expression) {
		if x != nil && x.Kind() == e.Kind() {
			walk(x.This())
			walk(x.Expr())
			return
		}
		out = append(out, x)
	}
	walk(e)
	return out
}

// mysqlFormatTime is Generator.format_time under MySQL's INVERSE_TIME_MAPPING.
func mysqlFormatTime(e exp.Expression) any {
	format := asExpression(e.Arg("format"))
	if format != nil && format.IsString() {
		return exp.LiteralString(dialects.MySQLFormatTime(format.Name()))
	}
	return format
}

// mysqlStrToDateSQL ports _str_to_date_sql (generators/mysql.py:54-57).
func (g *Generator) mysqlStrToDateSQL(e exp.Expression) string {
	return g.funcCall("STR_TO_DATE", []any{e.This(), mysqlFormatTime(e)}, "(", ")", true)
}

// mysqlDateTruncSQL ports _date_trunc_sql (generators/mysql.py:31-51); the WeekStart unit is not
// modeled in this port.
func (g *Generator) mysqlDateTruncSQL(e exp.Expression) string {
	expr := g.sqlKey(e, "this")
	unit := strings.ToUpper(e.Text("unit"))
	var concat, format string
	switch unit {
	case "WEEK":
		concat = "CONCAT(YEAR(" + expr + "), ' ', WEEK(" + expr + ", 1), ' 1')"
		format = "%Y %u %w"
	case "MONTH":
		concat = "CONCAT(YEAR(" + expr + "), ' ', MONTH(" + expr + "), ' 1')"
		format = "%Y %c %e"
	case "QUARTER":
		concat = "CONCAT(YEAR(" + expr + "), ' ', QUARTER(" + expr + ") * 3 - 2, ' 1')"
		format = "%Y %c %e"
	case "YEAR":
		concat = "CONCAT(YEAR(" + expr + "), ' 1 1')"
		format = "%Y %c %e"
	default:
		if unit != "DAY" {
			g.unsupported("Unexpected interval unit: " + unit)
		}
		return g.funcCall("DATE", []any{expr}, "(", ")", true)
	}
	return g.funcCall("STR_TO_DATE", []any{concat, "'" + format + "'"}, "(", ")", true)
}

// mysqlTimestampTruncSQL ports timestamptrunc_sql (generators/mysql.py:763-776): truncate by
// adding the whole-unit distance from an old anchor timestamp back onto that anchor.
func (g *Generator) mysqlTimestampTruncSQL(e exp.Expression) string {
	unit := asExpression(e.Arg("unit"))
	start := "0000-01-01 00:00:00"
	diff := exp.New(exp.KindTimestampDiff, exp.Args{"this": e.This(), "expression": exp.LiteralString(start), "unit": unit})
	interval := exp.Interval(exp.Args{"this": diff, "unit": unit.Copy()})
	dateAdd := exp.New(exp.KindDateAdd, exp.Args{"this": exp.LiteralString(start), "expression": interval.This(), "unit": unitToStrValue(asExpression(interval.Arg("unit")))})
	return g.gen(dateAdd)
}

// generators/mysql.py:86-96
var mysqlMakeIntervalUnitAliases = map[string]string{
	"years": "year", "months": "month", "weeks": "week", "days": "day",
	"hours": "hour", "minutes": "minute", "mins": "minute", "seconds": "second", "secs": "second",
}

// mysqlMakeIntervalSQL ports makeinterval_sql (generators/mysql.py:614-635).
func (g *Generator) mysqlMakeIntervalSQL(e exp.Expression) string {
	var intervals []string
	for _, key := range exp.ArgKeys(e.Kind()) {
		value := asExpression(e.Arg(key))
		if value == nil {
			continue
		}
		unitName := key
		if value.Kind() == exp.KindKwarg {
			unitName = strings.ToLower(value.This().Name())
			if alias, ok := mysqlMakeIntervalUnitAliases[unitName]; ok {
				unitName = alias
			}
			value = value.Expr()
		}
		intervals = append(intervals, g.gen(exp.Interval(exp.Args{"this": value.Copy(), "unit": exp.Var(exp.Args{"this": strings.ToUpper(unitName)})})))
	}
	if len(intervals) == 0 {
		return g.functionFallbackSQL(e)
	}
	sep := " + "
	if parent := e.Parent(); parent != nil && parent.Kind() == exp.KindSub && parent.Expr() == e {
		sep = " - "
	}
	return strings.Join(intervals, sep)
}

// generators/mysql.py TRANSFORMS and methods reached from Snowflake sources.
func init() {
	registerDialectDispatch("mysql", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindCurrentVersion: prestoRename("VERSION"),
		exp.KindPivot:          noPivotSQL,
		exp.KindTableSample: func(g *Generator, e exp.Expression) string {
			// no_tablesample_sql (dialects/dialect.py:1291-1293)
			g.unsupported("TABLESAMPLE unsupported")
			return g.sqlKey(e, "this")
		},
		exp.KindTimeFromParts: prestoRename("MAKETIME"),
		exp.KindBitwiseAndAgg: prestoRename("BIT_AND"),
		exp.KindBitwiseOrAgg:  prestoRename("BIT_OR"),
		exp.KindBitwiseXorAgg: prestoRename("BIT_XOR"),
		exp.KindLogicalAnd:    prestoRename("MIN"),
		exp.KindStuff:         prestoRename("INSERT"),
		exp.KindConvertTimezone: func(g *Generator, e exp.Expression) string {
			// converttimezone_sql (generators/mysql.py:785-790).
			return g.funcCall("CONVERT_TZ", []any{e.Arg("timestamp"), e.Arg("source_tz"), e.Arg("target_tz")}, "(", ")", true)
		},
		exp.KindIgnoreNulls: func(g *Generator, e exp.Expression) string {
			// ignorenulls_sql (generators/mysql.py:799-802).
			g.unsupported("MySQL does not support IGNORE NULLS.")
			return g.gen(e.This())
		},
	})
	registerDialectPropertyLocations("mysql", map[exp.Kind]propertyLocation{
		exp.KindTransientProperty: propertyLocationUnsupported,
	})
}
