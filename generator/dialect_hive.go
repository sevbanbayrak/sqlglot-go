package generator

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	"github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// Hive overrides follow generators/hive.py:260-393 and 474-642.
func init() {
	registerDialectTypeMapping("hive", map[expressions.DType]string{
		expressions.DTypeBit: "BOOLEAN", expressions.DTypeBlob: "BINARY", expressions.DTypeDatetime: "TIMESTAMP", expressions.DTypeRowVersion: "BINARY", expressions.DTypeText: "STRING", expressions.DTypeTime: "TIMESTAMP", expressions.DTypeTimestampNtz: "TIMESTAMP", expressions.DTypeTimestampTz: "TIMESTAMP", expressions.DTypeUTinyInt: "SMALLINT", expressions.DTypeVarBinary: "BINARY",
	})
	registerDialectPropertyLocations("hive", map[expressions.Kind]propertyLocation{
		expressions.KindExternalProperty:       propertyLocationPostCreate,
		expressions.KindLocationProperty:       propertyLocationPostSchema,
		expressions.KindClusteredByProperty:    propertyLocationPostSchema,
		expressions.KindStorageHandlerProperty: propertyLocationPostSchema,
		expressions.KindUsingProperty:          propertyLocationPostExpression,
		expressions.KindFileFormatProperty:     propertyLocationPostSchema,
		expressions.KindPartitionedByProperty:  propertyLocationPostSchema,
		expressions.KindWithDataProperty:       propertyLocationUnsupported,
	})
	handlers := map[expressions.Kind]func(*Generator, expressions.Expression) string{
		expressions.KindParameter: (*Generator).hiveParameterSQL,
		expressions.KindVersion: func(g *Generator, e expressions.Expression) string {
			return e.Name() + " " + e.Text("kind") + " " + g.sqlKey(e, "expression")
		},
		expressions.KindProperties: (*Generator).hivePropertiesSQL,
		// property_sql (dialect.py:1317-1318): property_name(string_key=True) keeps a Dot key.
		expressions.KindProperty: func(g *Generator, e expressions.Expression) string {
			if this := asExpression(e.Arg("this")); this != nil && this.Kind() == expressions.KindDot {
				return g.sqlKey(e, "this") + "=" + g.sqlKey(e, "value")
			}
			return g.gen(expressions.LiteralString(e.Name())) + "=" + g.sqlKey(e, "value")
		},
		expressions.KindExternalProperty:       func(*Generator, expressions.Expression) string { return "EXTERNAL" },
		expressions.KindLocationProperty:       hivePrefix("LOCATION "),
		expressions.KindStorageHandlerProperty: hivePrefix("STORED BY "),
		expressions.KindSchemaCommentProperty:  hivePrefix("COMMENT "),
		expressions.KindPartitionedByProperty:  hivePrefix("PARTITIONED BY "),
		expressions.KindUsingProperty: func(g *Generator, e expressions.Expression) string {
			return "USING " + e.Text("kind") + " " + g.sqlKey(e, "this")
		},
		expressions.KindFileFormatProperty: func(g *Generator, e expressions.Expression) string {
			this := strings.ToUpper(e.Name())
			if t := e.This(); t != nil && t.Kind() == expressions.KindInputOutputFormat {
				this = g.gen(t)
			}
			return "STORED AS " + this
		},
		expressions.KindInputOutputFormat: func(g *Generator, e expressions.Expression) string {
			var p []string
			for _, a := range []struct{ k, p string }{{"input_format", "INPUTFORMAT "}, {"output_format", "OUTPUTFORMAT "}} {
				if s := g.sqlKey(e, a.k); s != "" {
					p = append(p, a.p+s)
				}
			}
			return strings.Join(p, " ")
		},
		expressions.KindClusteredByProperty: func(g *Generator, e expressions.Expression) string {
			s := "CLUSTERED BY (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
			if v := g.expressions(exprsOptions{expression: e, key: "sorted_by", flat: true}); v != "" {
				s += " SORTED BY (" + v + ")"
			}
			return s + " INTO " + g.sqlKey(e, "buckets") + " BUCKETS"
		},
		expressions.KindAlterSet:    (*Generator).hiveAlterSetSQL,
		expressions.KindAlterColumn: (*Generator).hiveAlterColumnSQL,
		expressions.KindColumnDef: func(g *Generator, e expressions.Expression) string {
			s := g.columnDefSQL(e)
			if p := e.Parent(); p != nil && p.Kind() == expressions.KindDataType && expressions.IsType(p, expressions.DTypeStruct) {
				column := g.sqlKey(e, "this")
				s = strings.Replace(s, column+" ", column+": ", 1)
			}
			return s
		},
		expressions.KindDataType:       (*Generator).hiveDataTypeSQL,
		expressions.KindTryCast:        func(g *Generator, e expressions.Expression) string { return g.castSQLWithPrefix(e, "") },
		expressions.KindApproxDistinct: hiveCall("APPROX_COUNT_DISTINCT", "this"),
		expressions.KindRegexpLike:     func(g *Generator, e expressions.Expression) string { return g.binary(e, "RLIKE") },
		expressions.KindIntDiv:         func(g *Generator, e expressions.Expression) string { return g.binary(e, "DIV") },
		expressions.KindIgnoreNulls: func(g *Generator, e expressions.Expression) string {
			// hive.py:432-437: FIRST/LAST/FIRST_VALUE/LAST_VALUE take IGNORE NULLS as a TRUE argument.
			if this := e.This(); this != nil {
				switch this.Kind() {
				case expressions.KindFirst, expressions.KindLast, expressions.KindFirstValue, expressions.KindLastValue:
					return g.funcCall(g.sqlName(this.Kind()), []any{this.This(), expressions.Boolean(expressions.Args{"this": true})}, "(", ")", true)
				}
			}
			return g.ignoreNullsSQL(e)
		},
		expressions.KindStrToDate: func(g *Generator, e expressions.Expression) string {
			return "CAST(" + hiveStrToTimeInner(g, e) + " AS DATE)"
		},
		expressions.KindStrToTime: func(g *Generator, e expressions.Expression) string {
			return "CAST(" + hiveStrToTimeInner(g, e) + " AS TIMESTAMP)"
		},
		expressions.KindDateAdd:      (*Generator).hiveAddDateSQL,
		expressions.KindTsOrDsAdd:    (*Generator).hiveAddDateSQL,
		expressions.KindTsOrDsToDate: (*Generator).hiveToDateSQL,
		expressions.KindTimeToStr: func(g *Generator, e expressions.Expression) string {
			// hive.py:593-598: DATE_FORMAT(TimeStrToTime(x), f) renders the inner x directly.
			this := e.This()
			if this != nil && this.Kind() == expressions.KindTimeStrToTime {
				this = this.This()
			}
			return g.funcCall("DATE_FORMAT", []any{this, hiveFormatTimeLiteral(g, e)}, "(", ")", true)
		},
		expressions.KindUnixToStr: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("FROM_UNIXTIME", []any{e.This(), hiveNonDefaultTimeFormat(g, e)}, "(", ")", true)
		},
		expressions.KindStrToUnix: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("UNIX_TIMESTAMP", []any{e.This(), hiveNonDefaultTimeFormat(g, e)}, "(", ")", true)
		},
		expressions.KindArrayAgg: func(g *Generator, e expressions.Expression) string {
			this := e.This()
			if this != nil && this.Kind() == expressions.KindOrder {
				this = this.This()
			}
			return g.funcCall("COLLECT_LIST", []any{this}, "(", ")", true)
		},
		expressions.KindStruct: func(g *Generator, e expressions.Expression) string {
			args := []any{}
			for _, x := range e.Expressions() {
				if x.Kind() == expressions.KindPropertyEQ {
					g.unsupported("Hive does not support named structs.")
					args = append(args, x.Arg("expression"))
				} else {
					args = append(args, x)
				}
			}
			return g.funcCall("STRUCT", args, "(", ")", true)
		},
		expressions.KindVarMap: (*Generator).hiveVarMapSQL,
		expressions.KindExists: func(g *Generator, e expressions.Expression) string {
			// hive.py:587-591: EXISTS(array, lambda) is a function call, not the predicate.
			if e.Expr() != nil {
				return g.functionFallbackSQL(e)
			}
			return g.existsSQL(e)
		},
		expressions.KindRegexpExtract:     (*Generator).hiveRegexpExtractSQL,
		expressions.KindRegexpExtractAll:  (*Generator).hiveRegexpExtractSQL,
		expressions.KindJSONExtract:       hiveCall("GET_JSON_OBJECT", "this", "expression"),
		expressions.KindJSONExtractScalar: hiveCall("GET_JSON_OBJECT", "this", "expression"),
		expressions.KindGenerateSeries:    (*Generator).sequenceSQL,
		expressions.KindGenerateDateArray: (*Generator).sequenceSQL,
		expressions.KindDateSub:           (*Generator).hiveAddDateSQL,
		expressions.KindDateDiff:          (*Generator).hiveDateDiffSQL,
		expressions.KindTsOrDsDiff:        (*Generator).hiveDateDiffSQL,
		expressions.KindDateStrToDate: func(g *Generator, e expressions.Expression) string {
			return g.gen(expressions.New(expressions.KindCast, expressions.Args{"this": e.This(), "to": expressions.DTypeDate.IntoExpr(nil)}))
		},
		expressions.KindDateToDi: func(g *Generator, e expressions.Expression) string {
			return "CAST(DATE_FORMAT(" + g.sqlKey(e, "this") + ", 'yyyyMMdd') AS INT)"
		},
		expressions.KindDiToDate: func(g *Generator, e expressions.Expression) string {
			return "TO_DATE(CAST(" + g.sqlKey(e, "this") + " AS STRING), 'yyyyMMdd')"
		},
		expressions.KindTsOrDiToDi: func(g *Generator, e expressions.Expression) string {
			return "CAST(SUBSTR(REPLACE(CAST(" + g.sqlKey(e, "this") + " AS STRING), '-', ''), 1, 8) AS INT)"
		},
		expressions.KindUnixToTime: func(g *Generator, e expressions.Expression) string {
			// hive.py:188-194
			scale := asExpression(e.Arg("scale"))
			if scale == nil || (scale.IsNumber() && scale.Name() == "0") {
				return g.funcCall("FROM_UNIXTIME", g.fallbackArgs(e), "(", ")", true)
			}
			return "FROM_UNIXTIME(" + g.sqlKey(e, "this") + " / POW(10, " + g.gen(scale) + "))"
		},
		expressions.KindIf: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("IF", []any{e.This(), e.Arg("true"), e.Arg("false")}, "(", ")", true)
		},
		expressions.KindILike: func(g *Generator, e expressions.Expression) string {
			// dialect.py:1269-1276 no_ilike_sql
			return g.likeSQL(expressions.Like(expressions.Args{
				"this":       expressions.New(expressions.KindLower, expressions.Args{"this": e.This()}),
				"expression": expressions.New(expressions.KindLower, expressions.Args{"this": e.Expr()}),
				"negate":     e.Arg("negate"),
			}))
		},
		expressions.KindTrim: (*Generator).trimSQLStandard,
		expressions.KindStrPosition: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("LOCATE", []any{e.Arg("substr"), e.Arg("this"), e.Arg("position")}, "(", ")", true)
		},
		expressions.KindTimeStrToTime: func(g *Generator, e expressions.Expression) string { return timeStrToTimeSQL(g, e, false) },
		expressions.KindLevenshtein: func(g *Generator, e expressions.Expression) string {
			g.unsupportedArgs(e, "ins_cost", "del_cost", "sub_cost", "max_dist")
			return g.funcCall("LEVENSHTEIN", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		expressions.KindLeft: func(g *Generator, e expressions.Expression) string {
			return g.gen(expressions.New(expressions.KindSubstring, expressions.Args{"this": e.This(), "start": expressions.LiteralNumber(1), "length": e.Expr()}))
		},
		expressions.KindRight: func(g *Generator, e expressions.Expression) string {
			start := expressions.Sub(expressions.Args{
				"this":       expressions.New(expressions.KindLength, expressions.Args{"this": e.This().Copy()}),
				"expression": expressions.Paren(expressions.Args{"this": expressions.Sub(expressions.Args{"this": e.Expr(), "expression": expressions.LiteralNumber(1)})}),
			})
			return g.gen(expressions.New(expressions.KindSubstring, expressions.Args{"this": e.This(), "start": start}))
		},
		expressions.KindArrayToString: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("CONCAT_WS", []any{e.Expr(), e.This()}, "(", ")", true)
		},
		expressions.KindMap: (*Generator).hiveVarMapSQL,
		expressions.KindSplit: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("SPLIT", []any{e.This(), g.funcCall("CONCAT", []any{"'\\\\Q'", e.Expr(), "'\\\\E'"}, "(", ")", true)}, "(", ")", true)
		},
		expressions.KindArraySort: func(g *Generator, e expressions.Expression) string {
			if e.Expr() != nil {
				g.unsupported("Hive's SORT_ARRAY does not support a comparator.")
			}
			return g.funcCall("SORT_ARRAY", []any{e.This()}, "(", ")", true)
		},
		expressions.KindWith: func(g *Generator, e expressions.Expression) string {
			// dialect.py:1284-1288 no_recursive_cte_sql
			if boolValue(e.Arg("recursive")) {
				g.unsupported("Recursive CTEs are unsupported")
				e.Set("recursive", false)
			}
			return g.withSQL(e)
		},
		expressions.KindTable: func(g *Generator, e expressions.Expression) string {
			if out := transforms.UnnestGenerateSeries(e); out.Kind() != expressions.KindTable {
				return g.gen(out)
			}
			return g.tableSQL(e)
		},
		expressions.KindSelect: func(g *Generator, e expressions.Expression) string {
			// generators/hive.py:359-366 Select preprocess chain.
			out := transforms.EliminateQualify(e)
			out = transforms.EliminateDistinctOn(out)
			out, err := transforms.UnnestToExplode(out, false)
			if err != nil {
				g.unsupported(err.Error())
			}
			out = transforms.AnyToExists(out)
			if out.Kind() != expressions.KindSelect {
				return g.gen(out)
			}
			return g.selectSQL(out)
		},
		expressions.KindStrToMap: hiveCall("STR_TO_MAP", "this", "pair_delim", "key_value_delim"),
		expressions.KindTimestampTrunc: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("TRUNC", []any{e.This(), expressions.LiteralString(e.Text("unit"))}, "(", ")", true)
		},
	}
	for k, name := range map[expressions.Kind]string{expressions.KindAnyValue: "FIRST", expressions.KindFromBase64: "UNBASE64", expressions.KindToBase64: "BASE64", expressions.KindJSONFormat: "TO_JSON", expressions.KindQuantile: "PERCENTILE", expressions.KindApproxQuantile: "PERCENTILE_APPROX", expressions.KindRegexpSplit: "SPLIT", expressions.KindArrayUniqueAgg: "COLLECT_SET", expressions.KindTimeStrToDate: "TO_DATE", expressions.KindTimeStrToUnix: "UNIX_TIMESTAMP", expressions.KindTimeToUnix: "UNIX_TIMESTAMP", expressions.KindWeekOfYear: "WEEKOFYEAR", expressions.KindDayOfMonth: "DAYOFMONTH", expressions.KindDayOfWeek: "DAYOFWEEK", expressions.KindUnnest: "EXPLODE", expressions.KindStarMap: "MAP", expressions.KindUnixToTimeStr: "FROM_UNIXTIME", expressions.KindIsNan: "ISNAN", expressions.KindArrayConcat: "CONCAT", expressions.KindNumberToStr: "FORMAT_NUMBER", expressions.KindUnicode: "ASCII"} {
		name := name
		handlers[k] = func(g *Generator, e expressions.Expression) string {
			return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
		}
	}
	// generators/hive.py:442-447.
	handlers[expressions.KindJSONPathKey] = func(g *Generator, e expressions.Expression) string {
		if this := e.This(); this != nil && this.Kind() == expressions.KindJSONPathWildcard {
			g.unsupported("Unsupported wildcard in JSONPathKey expression")
			return ""
		}
		return g.jsonPathKeySQL(e)
	}
	registerDialectDispatch("hive", handlers)
	// generators/hive.py:253-258.
	registerJSONPathParts("hive", expressions.KindJSONPathKey, expressions.KindJSONPathRoot, expressions.KindJSONPathSubscript, expressions.KindJSONPathWildcard)
}

func hiveCall(name string, keys ...string) func(*Generator, expressions.Expression) string {
	return func(g *Generator, e expressions.Expression) string {
		args := make([]any, 0, len(keys))
		for _, key := range keys {
			args = append(args, e.Arg(key))
		}
		return g.funcCall(name, args, "(", ")", true)
	}
}

// hiveFormatTimeLiteral is Generator.format_time under Hive's INVERSE_TIME_MAPPING.
func hiveFormatTimeLiteral(g *Generator, e expressions.Expression) any {
	format := asExpression(e.Arg("format"))
	if format == nil || !format.IsString() {
		return format
	}
	name := format.Name()
	// generators/hive.py:416-428: only a strict TIME_MAPPING (Hive, Spark 3+) renders lenient
	// M/d for single-digit sources; Spark2 parses leniently already.
	if !g.dialect.LenientTimeMapping {
		switch e.Kind() {
		case expressions.KindStrToTime, expressions.KindStrToDate, expressions.KindStrToUnix, expressions.KindTsOrDsToDate:
			name = hiveLenientParseFormat(name)
		}
	}
	return expressions.LiteralString(dialects.HiveFormatTime(name))
}

// hiveNonDefaultTimeFormat ports dialect.py:1622-1633 time_format("hive"): nil for TIME_FORMAT.
func hiveNonDefaultTimeFormat(g *Generator, e expressions.Expression) any {
	format := hiveFormatTimeLiteral(g, e)
	if lit, ok := format.(expressions.Expression); ok && lit != nil && lit.Name() == dialects.HiveTimeFormat {
		return nil
	}
	return format
}

// hiveStrToTimeInner ports the shared body of _str_to_date_sql/_str_to_time_sql (hive.py:199-209).
func hiveStrToTimeInner(g *Generator, e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if format, ok := hiveFormatTimeLiteral(g, e).(expressions.Expression); ok && format != nil {
		if !hiveIsCastTimeFormat(e, format.Name()) {
			return "FROM_UNIXTIME(UNIX_TIMESTAMP(" + this + ", " + g.gen(format) + "))"
		}
	}
	return this
}

// hiveToDateSQL ports _to_date_sql (hive.py:211-219).
func (g *Generator) hiveToDateSQL(e expressions.Expression) string {
	if format, ok := hiveFormatTimeLiteral(g, e).(expressions.Expression); ok && format != nil {
		if !hiveIsCastTimeFormat(e, format.Name()) {
			return g.funcCall("TO_DATE", []any{e.This(), format}, "(", ")", true)
		}
	}
	if p := e.Parent(); p != nil {
		switch p.Kind() {
		case expressions.KindDateDiff, expressions.KindDay, expressions.KindMonth, expressions.KindYear:
			return g.sqlKey(e, "this")
		case expressions.KindDayOfMonth, expressions.KindDayOfWeek, expressions.KindDayOfYear, expressions.KindWeekOfYear:
			// generators/spark2.py:143-149 TS_OR_DS_EXPRESSIONS.
			if g.isDialect("spark2") {
				return g.sqlKey(e, "this")
			}
		}
	}
	return g.funcCall("TO_DATE", []any{e.This()}, "(", ")", true)
}

// hiveVarMapSQL ports var_map_sql (dialect.py:1521-1536).
func (g *Generator) hiveVarMapSQL(e expressions.Expression) string {
	keys, values := asExpression(e.Arg("keys")), asExpression(e.Arg("values"))
	if keys == nil || values == nil || keys.Kind() != expressions.KindArray || values.Kind() != expressions.KindArray {
		g.unsupported("Cannot convert array columns into map.")
		return g.funcCall("MAP", []any{keys, values}, "(", ")", true)
	}
	args := []any{}
	ks, vs := keys.Expressions(), values.Expressions()
	for i := 0; i < len(ks) && i < len(vs); i++ {
		args = append(args, ks[i], vs[i])
	}
	return g.funcCall("MAP", args, "(", ")", true)
}

// hiveRegexpExtractSQL ports regexp_extract_sql (dialect.py:1855-1865).
func (g *Generator) hiveRegexpExtractSQL(e expressions.Expression) string {
	group := asExpression(e.Arg("group"))
	if group != nil && group.Name() == strconv.Itoa(g.dialect.RegexpExtractDefaultGroup) {
		group = nil
	}
	return g.funcCall(g.sqlName(e.Kind()), []any{e.This(), e.Arg("expression"), group}, "(", ")", true)
}

func hivePrefix(prefix string) func(*Generator, expressions.Expression) string {
	return func(g *Generator, e expressions.Expression) string { return prefix + g.sqlKey(e, "this") }
}

// generators/hive.py:235 supplies WITH_PROPERTIES_PREFIX for generator.py:2019-2042.
func (g *Generator) hivePropertiesSQL(e expressions.Expression) string {
	locations := g.locateProperties(e)
	root := g.rootPropertiesSQL(g.propertiesExpression(locations[propertyLocationPostSchema], e.Parent()))
	with := g.renderProperties(g.propertiesExpression(locations[propertyLocationPostWith], e.Parent()), g.seg("TBLPROPERTIES", ""), ", ", "", true)
	if root != "" && with != "" && !g.pretty {
		with = " " + with
	}
	return root + with
}

// generators/hive.py:590-605.
func (g *Generator) hiveAlterSetSQL(e expressions.Expression) string {
	s := "SET"
	if v := g.sqlKey(e, "serde"); v != "" {
		s += " SERDE " + v
	}
	if v := g.expressions(exprsOptions{expression: e, flat: true}); v != "" {
		s += " " + v
	}
	if v := g.sqlKey(e, "location"); v != "" {
		s += " LOCATION " + v
	}
	if v := g.expressions(exprsOptions{expression: e, key: "file_format", flat: true, sep: " "}); v != "" {
		s += " FILEFORMAT " + v
	}
	if v := g.expressions(exprsOptions{expression: e, key: "tag", flat: true, sep: ""}); v != "" {
		s += " TAGS " + v
	}
	return s
}

// generators/hive.py:560-584.
func (g *Generator) hiveAlterColumnSQL(e expressions.Expression) string {
	if boolValue(e.Arg("exists")) {
		g.unsupported("ALTER COLUMN IF EXISTS is not supported by this dialect")
	}
	if truthy(e.Arg("default")) || truthy(e.Arg("drop")) || truthy(e.Arg("visible")) || e.Arg("allow_null") != nil {
		g.unsupported("Unsupported CHANGE COLUMN syntax")
	}
	this := g.sqlKey(e, "this")
	name := g.sqlKey(e, "rename_to")
	if name == "" {
		name = this
	}
	dtype := g.sqlKey(e, "dtype")
	if dtype == "" {
		g.unsupported("CHANGE COLUMN without a type is not supported")
	}
	s := "CHANGE COLUMN " + this + " " + name + " " + dtype
	if c := g.sqlKey(e, "comment"); c != "" {
		s += " COMMENT " + c
	}
	return s
}

// generators/hive.py:502-520.
func (g *Generator) hiveDataTypeSQL(e expressions.Expression) string {
	e = e.Copy()
	params := e.Expressions()
	switch e.Arg("this") {
	case expressions.DTypeVarchar, expressions.DTypeNVarchar, expressions.DTypeChar, expressions.DTypeNChar:
		if len(params) == 0 || params[0].Name() == "MAX" {
			e.Set("this", expressions.DTypeText)
			e.Set("expressions", nil)
		}
	case expressions.DTypeText:
		if len(params) > 0 {
			e.Set("this", expressions.DTypeVarchar)
		}
	case expressions.DTypeFloat:
		if len(params) > 0 {
			if n, err := strconv.Atoi(params[0].Name()); err == nil {
				if n > 32 {
					e.Set("this", expressions.DTypeDouble)
				}
				e.Set("expressions", nil)
			}
		}
	}
	if dtype, ok := e.Arg("this").(expressions.DType); ok && expressions.TemporalTypes[dtype] {
		e.Set("expressions", nil)
	}
	return g.dataTypeSQL(e)
}

// generators/hive.py:118-139.
func (g *Generator) hiveAddDateSQL(e expressions.Expression) string {
	if e.Kind() == expressions.KindTsOrDsAdd && e.Arg("unit") == nil {
		return g.funcCall("DATE_ADD", []any{e.This(), e.Expr()}, "(", ")", true)
	}
	unit := strings.ToUpper(e.Text("unit"))
	name := "DATE_ADD"
	mult := 1
	switch unit {
	case "WEEK":
		mult = 7
	case "MONTH":
		name = "ADD_MONTHS"
	case "QUARTER":
		name = "ADD_MONTHS"
		mult = 3
	case "YEAR":
		name = "ADD_MONTHS"
		mult = 12
	}
	if e.Kind() == expressions.KindDateSub {
		mult *= -1
	}
	var inc any = e.Expr()
	if lit := asExpression(inc); lit != nil && lit.Kind() == expressions.KindLiteral {
		if n, err := strconv.ParseFloat(lit.Name(), 64); err == nil {
			inc = strconv.FormatFloat(n*float64(mult), 'f', -1, 64)
		} else {
			g.unsupported("Invalid date increment")
		}
	} else if mult != 1 {
		if lit != nil && lit.Is(expressions.TraitBinary) {
			inc = expressions.Paren(expressions.Args{"this": lit})
		}
		inc = expressions.Mul(expressions.Args{"this": inc, "expression": expressions.LiteralNumber(mult)})
	}
	return g.funcCall(name, []any{e.This(), inc}, "(", ")", true)
}

// hiveDateDiffSQL ports _date_diff_sql (hive.py:127-149).
func (g *Generator) hiveDateDiffSQL(e expressions.Expression) string {
	unit := strings.ToUpper(e.Text("unit"))
	if factor, ok := map[string]string{"MILLISECOND": " * 1000", "SECOND": "", "MINUTE": " / 60", "HOUR": " / 3600"}[unit]; ok {
		secDiff := "UNIX_TIMESTAMP(" + g.sqlKey(e, "this") + ") - UNIX_TIMESTAMP(" + g.sqlKey(e, "expression") + ")"
		if factor != "" {
			return "(" + secDiff + ")" + factor
		}
		return secDiff
	}
	monthsBetween := unit == "YEAR" || unit == "QUARTER" || unit == "MONTH"
	sqlFunc := "DATEDIFF"
	if monthsBetween {
		sqlFunc = "MONTHS_BETWEEN"
	}
	multiplier := map[string]int{"YEAR": 12, "QUARTER": 3, "WEEK": 7}[unit]
	multiplierSQL := ""
	if multiplier > 1 {
		multiplierSQL = " / " + strconv.Itoa(multiplier)
	}
	diff := sqlFunc + "(" + g.formatArgs([]any{e.This(), e.Expr()}, ", ") + ")"
	if monthsBetween || multiplierSQL != "" {
		diff = "CAST(" + diff + multiplierSQL + " AS INT)"
	}
	return diff
}

// sequenceSQL ports sequence_sql (dialect.py:2396-2443) without the is_end_exclusive branch,
// which needs simplify and is only set by dialects outside this port.
func (g *Generator) sequenceSQL(e expressions.Expression) string {
	start, end, step := asExpression(e.Arg("start")), asExpression(e.Arg("end")), asExpression(e.Arg("step"))
	var targetType expressions.Expression
	if start != nil && start.Kind() == expressions.KindCast {
		targetType = asExpression(start.Arg("to"))
	} else if end != nil && end.Kind() == expressions.KindCast {
		targetType = asExpression(end.Arg("to"))
	}
	if start != nil && end != nil && targetType != nil && (expressions.IsType(targetType, expressions.DTypeDate) || expressions.IsType(targetType, expressions.DTypeTimestamp)) {
		if start.Kind() == expressions.KindCast && asExpression(start.Arg("to")) == targetType {
			// exp.cast never re-casts an expression already cast to that type (builders.py:488).
			if !(end.Kind() == expressions.KindCast && asExpression(end.Arg("to")).Equal(targetType)) {
				end = expressions.New(expressions.KindCast, expressions.Args{"this": end, "to": targetType.Copy()})
			}
		} else {
			start = expressions.New(expressions.KindCast, expressions.Args{"this": start, "to": targetType.Copy()})
		}
	}
	return g.funcCall("SEQUENCE", []any{start, end, step}, "(", ")", true)
}

// generators/hive.py:449-460 omits braces for SET assignments.
func (g *Generator) hiveParameterSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if value := g.sqlKey(e, "expression"); value != "" {
		this += ":" + value
	}
	if parent := e.Parent(); parent != nil && parent.Kind() == expressions.KindEQ {
		if item := parent.Parent(); item != nil && item.Kind() == expressions.KindSetItem {
			return this
		}
	}
	return "${" + this + "}"
}

var hiveCanonicalTimeFormat = regexp.MustCompile(`%(?:[mdHIMS]strict|[-:].|.)`)

// generators/hive.py:67-92 keeps adjacent numeric fields padded.
func hiveLenientParseFormat(format string) string {
	matches := hiveCanonicalTimeFormat.FindAllStringIndex(format, -1)
	var result strings.Builder
	end := 0
	for i, match := range matches {
		start, stop := match[0], match[1]
		result.WriteString(format[end:start])
		token := format[start:stop]
		if len(token) == 2 && strings.ContainsRune("mdHIMS", rune(token[1])) {
			leftAdjacent := i > 0 && matches[i-1][1] == start
			rightAdjacent := i+1 < len(matches) && matches[i+1][0] == stop
			if start > 0 {
				r, _ := utf8.DecodeLastRuneInString(format[:start])
				leftAdjacent = leftAdjacent || unicode.IsDigit(r)
			}
			if stop < len(format) {
				r, _ := utf8.DecodeRuneInString(format[stop:])
				rightAdjacent = rightAdjacent || unicode.IsDigit(r)
			}
			if !leftAdjacent && !rightAdjacent {
				token = "%-" + token[1:]
			}
		}
		result.WriteString(token)
		end = stop
	}
	result.WriteString(format[end:])
	return result.String()
}

func hiveIsCastTimeFormat(e expressions.Expression, format string) bool {
	if format == dialects.HiveTimeFormat || format == dialects.HiveDateFormat {
		return true
	}
	if format == "yyyy-M-d H:m:s" || format == "yyyy-M-d" {
		if original := asExpression(e.Arg("format")); original != nil && original.IsString() {
			padded := dialects.HiveFormatTime(original.Name())
			return padded == dialects.HiveTimeFormat || padded == dialects.HiveDateFormat
		}
	}
	return false
}
