package generator

import (
	"strconv"
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// generators/spark2.py: Spark2Generator on top of HiveGenerator (dialectParents).
func init() {
	registerDialectPropertyLocations("spark2", map[exp.Kind]propertyLocation{
		exp.KindEngineProperty:        propertyLocationUnsupported,
		exp.KindAutoIncrementProperty: propertyLocationUnsupported,
		exp.KindCharacterSetProperty:  propertyLocationUnsupported,
		exp.KindCollateProperty:       propertyLocationUnsupported,
	})
	// generators/spark2.py:206-210: exp.X: None entries fall back to the base generator.
	registerDialectDispatchRemovals("spark2", exp.KindArraySort, exp.KindILike, exp.KindLeft, exp.KindMonthsBetween, exp.KindRight)
	registerDialectDispatch("spark2", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindApproxDistinct:    prestoRename("APPROX_COUNT_DISTINCT"),
		exp.KindArrayToString:     prestoRename("ARRAY_JOIN"),
		exp.KindArraySlice:        prestoRename("SLICE"),
		exp.KindBitwiseLeftShift:  prestoRename("SHIFTLEFT"),
		exp.KindBitwiseRightShift: prestoRename("SHIFTRIGHT"),
		exp.KindDateFromParts:     prestoRename("MAKE_DATE"),
		exp.KindDayOfMonth:        prestoRename("DAYOFMONTH"),
		exp.KindDayOfWeek:         prestoRename("DAYOFWEEK"),
		exp.KindDayOfYear:         prestoRename("DAYOFYEAR"),
		exp.KindFormat:            prestoRename("FORMAT_STRING"),
		exp.KindLogicalAnd:        prestoRename("BOOL_AND"),
		exp.KindLogicalOr:         prestoRename("BOOL_OR"),
		exp.KindReduce:            prestoRename("AGGREGATE"),
		exp.KindVariancePop:       prestoRename("VAR_POP"),
		exp.KindWeekOfYear:        prestoRename("WEEKOFYEAR"),
		exp.KindArraySum: func(g *Generator, e exp.Expression) string {
			return "AGGREGATE(" + g.sqlKey(e, "this") + ", 0, (acc, x) -> acc + x, acc -> acc)"
		},
		exp.KindAtTimeZone:   prestoCall("FROM_UTC_TIMESTAMP", "this", "zone"),
		exp.KindFromTimeZone: prestoCall("TO_UTC_TIMESTAMP", "this", "zone"),
		exp.KindDayOfWeekIso: func(g *Generator, e exp.Expression) string {
			return "((" + g.funcCall("DAYOFWEEK", []any{e.This()}, "(", ")", true) + " % 7) + 1)"
		},
		exp.KindRegexpReplace: prestoCall("REGEXP_REPLACE", "this", "expression", "replacement", "position"),
		exp.KindSHA2Digest: func(g *Generator, e exp.Expression) string {
			var length any = e.Arg("length")
			if !truthy(length) {
				length = exp.LiteralNumber(256)
			}
			return g.funcCall("SHA2", []any{e.This(), length}, "(", ")", true)
		},
		exp.KindDateTrunc: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TRUNC", []any{e.This(), unitToStr(e)}, "(", ")", true)
		},
		exp.KindTimestampTrunc: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_TRUNC", []any{unitToStr(e), e.This()}, "(", ")", true)
		},
		exp.KindStrToDate: func(g *Generator, e exp.Expression) string {
			format := hiveFormatTimeLiteral(g, e)
			if lit, ok := format.(exp.Expression); ok && lit != nil && lit.Name() == dialects.HiveDateFormat {
				return g.funcCall("TO_DATE", []any{e.This()}, "(", ")", true)
			}
			return g.funcCall("TO_DATE", []any{e.This(), format}, "(", ")", true)
		},
		exp.KindStrToTime: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_TIMESTAMP", []any{e.This(), hiveFormatTimeLiteral(g, e)}, "(", ")", true)
		},
		exp.KindUnixToTime: (*Generator).spark2UnixToTimeSQL,
		exp.KindJSONFormat: (*Generator).spark2JSONFormatSQL,
		exp.KindMap: func(g *Generator, e exp.Expression) string {
			keys, values := e.Arg("keys"), e.Arg("values")
			if !truthy(keys) || !truthy(values) {
				return "MAP()"
			}
			return g.funcCall("MAP_FROM_ARRAYS", []any{keys, values}, "(", ")", true)
		},
		exp.KindStruct: (*Generator).structSQL,
		exp.KindCast:   (*Generator).spark2CastSQL,
		exp.KindFileFormatProperty: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("hive_format")) {
				return dialectDispatch["hive"][exp.KindFileFormatProperty](g, e)
			}
			return "USING " + strings.ToUpper(e.Name())
		},
		exp.KindAlterColumn: (*Generator).spark2AlterColumnSQL,
		exp.KindBracket: func(g *Generator, e exp.Expression) string {
			if safe, ok := e.Arg("safe").(bool); ok && !safe {
				return g.bracketToElementAtSQL(e)
			}
			return g.bracketSQL(e)
		},
		exp.KindFrom: func(g *Generator, e exp.Expression) string {
			return g.fromSQL(spark2UnaliasPivot(e))
		},
		exp.KindPivot: func(g *Generator, e exp.Expression) string {
			return g.pivotSQL(transforms.UnqualifyPivotFields(e))
		},
		exp.KindWithinGroup: func(g *Generator, e exp.Expression) string {
			out := transforms.RemoveWithinGroupForPercentiles(e)
			if out.Kind() != exp.KindWithinGroup {
				return g.gen(out)
			}
			return g.withinGroupSQL(out)
		},
		exp.KindCreate: func(g *Generator, e exp.Expression) string {
			out := transforms.RemoveUniqueConstraints(e)
			out = transforms.CtasWithTmpTablesToCreateTmpView(out, spark2TemporaryStorageProvider)
			out = transforms.MoveSchemaColumnsToPartitionedBy(out)
			return g.createSQL(out)
		},
	})
}

// spark2TemporaryStorageProvider ports temporary_storage_provider (generators/spark2.py:117-121).
func spark2TemporaryStorageProvider(e exp.Expression) exp.Expression {
	provider := exp.New(exp.KindFileFormatProperty, exp.Args{"this": exp.LiteralString("parquet")})
	props := asExpression(e.Arg("properties"))
	if props == nil {
		props = exp.Properties(exp.Args{"expressions": []exp.Expression{}})
		e.Set("properties", props)
	}
	props.Set("expressions", append(props.Expressions(), provider))
	return e
}

// spark2UnaliasPivot ports _unalias_pivot (generators/spark2.py:84-105).
func spark2UnaliasPivot(e exp.Expression) exp.Expression {
	this := e.This()
	if this == nil {
		return e
	}
	pivots := listArg(this, "pivots")
	if len(pivots) == 0 {
		return e
	}
	alias := asExpression(pivots[0].Arg("alias"))
	if alias == nil {
		return e
	}
	alias.Pop()
	inner := exp.Select(exp.Args{"expressions": []exp.Expression{exp.Star(nil)}, "from_": exp.From(exp.Args{"this": this.Copy()})})
	return exp.From(exp.Args{"this": this.Replace(exp.Subquery(exp.Args{"this": inner, "alias": alias}))})
}

func listArg(e exp.Expression, key string) []exp.Expression {
	var out []exp.Expression
	for _, v := range listFromValue(e.Arg(key)) {
		if x, ok := v.(exp.Expression); ok && x != nil {
			out = append(out, x)
		}
	}
	return out
}

// spark2IsParseJSON ports is_parse_json (dialect.py:1958-1961).
func spark2IsParseJSON(e exp.Expression) bool {
	if e == nil {
		return false
	}
	return e.Kind() == exp.KindParseJSON || (e.Kind() == exp.KindCast && exp.IsType(asExpression(e.Arg("to")), exp.DTypeJSON))
}

// spark2JSONFormatSQL ports _json_format_sql (generators/spark2.py:22-41).
func (g *Generator) spark2JSONFormatSQL(e exp.Expression) string {
	this := e.This()
	if spark2IsParseJSON(this) {
		if inner := this.This(); inner != nil && inner.IsString() {
			wrapped := exp.LiteralString("[" + inner.Name() + "]")
			fromJSON := g.funcCall("FROM_JSON", []any{wrapped, g.funcCall("SCHEMA_OF_JSON", []any{wrapped}, "(", ")", true)}, "(", ")", true)
			toJSON := g.funcCall("TO_JSON", []any{fromJSON}, "(", ")", true)
			return g.funcCall("REGEXP_EXTRACT", []any{toJSON, "'^.(.*).$'", "1"}, "(", ")", true)
		}
		return g.gen(this)
	}
	return g.funcCall("TO_JSON", []any{this, e.Arg("options")}, "(", ")", true)
}

// spark2UnixToTimeSQL ports _unix_to_time_sql (generators/spark2.py:59-75).
func (g *Generator) spark2UnixToTimeSQL(e exp.Expression) string {
	scale := asExpression(e.Arg("scale"))
	timestamp := e.This()
	if scale == nil {
		fromUnixtime := exp.New(exp.KindAnonymous, exp.Args{"this": "from_unixtime", "expressions": []exp.Expression{timestamp}})
		return g.gen(exp.Cast(exp.Args{"this": fromUnixtime, "to": exp.DTypeTimestamp.IntoExpr(nil)}))
	}
	switch scale.Name() {
	case "0":
		return g.funcCall("TIMESTAMP_SECONDS", []any{timestamp}, "(", ")", true)
	case "3":
		return g.funcCall("TIMESTAMP_MILLIS", []any{timestamp}, "(", ")", true)
	case "6":
		return g.funcCall("TIMESTAMP_MICROS", []any{timestamp}, "(", ")", true)
	}
	pow := exp.FunctionByName["POW"]([]exp.Expression{exp.LiteralNumber(10), scale})
	return g.funcCall("TIMESTAMP_SECONDS", []any{exp.Div(exp.Args{"this": timestamp, "expression": pow})}, "(", ")", true)
}

// spark2CastSQL ports Spark2Generator.cast_sql (generators/spark2.py:168-184).
func (g *Generator) spark2CastSQL(e exp.Expression) string {
	arg := e.This()
	to := asExpression(e.Arg("to"))
	isJSONExtract := arg != nil && (arg.Kind() == exp.KindJSONExtract || arg.Kind() == exp.KindJSONExtractScalar) && !truthy(arg.Arg("variant_extract"))
	if to != nil && truthy(to.Arg("nested")) && (spark2IsParseJSON(arg) || isJSONExtract) {
		schema := "'" + g.gen(to) + "'"
		source := arg
		if !isJSONExtract {
			source = arg.This()
		}
		return g.funcCall("FROM_JSON", []any{source, schema}, "(", ")", true)
	}
	if spark2IsParseJSON(e) {
		return g.funcCall("TO_JSON", []any{arg}, "(", ")", true)
	}
	return g.castSQLWithPrefix(e, "")
}

// spark2AlterColumnSQL ports Spark2Generator.altercolumn_sql (generators/spark2.py:192-204).
func (g *Generator) spark2AlterColumnSQL(e exp.Expression) string {
	if truthy(e.Arg("exists")) {
		g.unsupported("ALTER COLUMN IF EXISTS is not supported by this dialect")
	}
	this := g.sqlKey(e, "this")
	newName := g.sqlKey(e, "rename_to")
	if newName == "" || newName == this {
		if comment := g.sqlKey(e, "comment"); comment != "" {
			return "ALTER COLUMN " + this + " COMMENT " + comment
		}
		return g.alterColumnSQL(e)
	}
	return "RENAME COLUMN " + this + " TO " + newName
}

// bracketToElementAtSQL ports bracket_to_element_at_sql (dialect.py:1218-1228); only a literal
// index is shifted by apply_index_offset, non-literal indexes pass through unchanged.
func (g *Generator) bracketToElementAtSQL(e exp.Expression) string {
	offset := 1
	if v, ok := e.Arg("offset").(int); ok {
		offset = 1 - v
	}
	var index any
	if xs := e.Expressions(); len(xs) > 0 {
		index = xs[0]
		if offset != 0 && xs[0].IsNumber() {
			if n, err := strconv.Atoi(xs[0].Name()); err == nil {
				index = exp.LiteralNumber(n + offset)
			}
		}
	}
	return g.funcCall("ELEMENT_AT", []any{e.This(), index}, "(", ")", true)
}
