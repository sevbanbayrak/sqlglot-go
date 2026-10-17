package generator

import (
	"strconv"
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/optimizer"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// generators/snowflake.py: SnowflakeGenerator, a direct Generator subclass.
func init() {
	registerDialectTypeMapping("snowflake", map[exp.DType]string{
		exp.DTypeBigDecimal: "DOUBLE",
		exp.DTypeJSON:       "VARIANT",
		exp.DTypeNested:     "OBJECT",
		exp.DTypeStruct:     "OBJECT",
		exp.DTypeText:       "VARCHAR",
	})
	registerJSONPathParts("snowflake", exp.KindJSONPathKey, exp.KindJSONPathRoot, exp.KindJSONPathSubscript)
	registerDialectPropertyLocations("snowflake", map[exp.Kind]propertyLocation{
		exp.KindCredentialsProperty:   propertyLocationPostWith,
		exp.KindLocationProperty:      propertyLocationPostWith,
		exp.KindPartitionedByProperty: propertyLocationPostSchema,
		exp.KindRowAccessProperty:     propertyLocationPostSchema,
		exp.KindSetProperty:           propertyLocationUnsupported,
		exp.KindVolatileProperty:      propertyLocationUnsupported,
	})
	rename := prestoRename
	tryPrefixed := func(name string, keys ...string) func(*Generator, exp.Expression) string {
		return func(g *Generator, e exp.Expression) string {
			prefix := ""
			if truthy(e.Arg("safe")) {
				prefix = "TRY_"
			}
			args := []any{e.This()}
			for _, key := range keys {
				args = append(args, e.Arg(key))
			}
			return g.funcCall(prefix+name, args, "(", ")", true)
		}
	}
	registerDialectDispatch("snowflake", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindApproxDistinct: rename("APPROX_COUNT_DISTINCT"),
		exp.KindArgMax:         rename("MAX_BY"),
		exp.KindArgMin:         rename("MIN_BY"),
		exp.KindArray: func(g *Generator, e exp.Expression) string {
			return g.snowflakeArraySQL(transforms.InheritStructFieldNames(e))
		},
		exp.KindArrayConcat:  arrayConcatSQL("ARRAY_CAT", false),
		exp.KindArrayAppend:  arrayAppendSQLFunc("ARRAY_APPEND", false),
		exp.KindArrayPrepend: arrayAppendSQLFunc("ARRAY_PREPEND", false),
		exp.KindArrayContains: func(g *Generator, e exp.Expression) string {
			var value any = e.Expr()
			if v, ok := e.Arg("ensure_variant").(bool); !ok || v {
				value = exp.CastTo(e.Expr(), exp.DTypeVariant)
			}
			return g.funcCall("ARRAY_CONTAINS", []any{value, e.This()}, "(", ")", true)
		},
		exp.KindArrayPosition: func(g *Generator, e exp.Expression) string {
			return g.funcCall("ARRAY_POSITION", []any{e.Expr(), e.This()}, "(", ")", true)
		},
		exp.KindArrayIntersect: rename("ARRAY_INTERSECTION"),
		exp.KindArrayOverlaps:  rename("ARRAYS_OVERLAP"),
		exp.KindAtTimeZone: func(g *Generator, e exp.Expression) string {
			return g.funcCall("CONVERT_TIMEZONE", []any{e.Arg("zone"), e.This()}, "(", ")", true)
		},
		exp.KindBitwiseOr:         rename("BITOR"),
		exp.KindBitwiseXor:        rename("BITXOR"),
		exp.KindBitwiseAnd:        rename("BITAND"),
		exp.KindBitwiseAndAgg:     rename("BITANDAGG"),
		exp.KindBitwiseOrAgg:      rename("BITORAGG"),
		exp.KindBitwiseXorAgg:     rename("BITXORAGG"),
		exp.KindBitwiseNot:        rename("BITNOT"),
		exp.KindBitwiseLeftShift:  rename("BITSHIFTLEFT"),
		exp.KindBitwiseRightShift: rename("BITSHIFTRIGHT"),
		exp.KindCreate: func(g *Generator, e exp.Expression) string {
			return g.createSQL(snowflakeFlattenStructuredTypesUnlessIceberg(e))
		},
		exp.KindCurrentTimestamp: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("sysdate")) {
				return g.funcCall("SYSDATE", nil, "(", ")", true)
			}
			return g.functionFallbackSQL(e)
		},
		exp.KindCurrentSchemas: func(g *Generator, _ exp.Expression) string {
			return g.funcCall("CURRENT_SCHEMAS", nil, "(", ")", true)
		},
		exp.KindLocaltime: func(g *Generator, e exp.Expression) string {
			if e.This() != nil {
				return g.funcCall("CURRENT_TIME", []any{e.This()}, "(", ")", true)
			}
			return "CURRENT_TIME"
		},
		exp.KindLocaltimestamp: func(g *Generator, e exp.Expression) string {
			if e.This() != nil {
				return g.funcCall("CURRENT_TIMESTAMP", []any{e.This()}, "(", ")", true)
			}
			return "CURRENT_TIMESTAMP"
		},
		exp.KindDateAdd:       dateDeltaSQL("DATEADD", false),
		exp.KindDateDiff:      dateDeltaSQL("DATEDIFF", false),
		exp.KindDatetimeAdd:   dateDeltaSQL("TIMESTAMPADD", false),
		exp.KindDatetimeDiff:  timestampDiffSQLFunc,
		exp.KindDateStrToDate: dateStrToDateSQL,
		exp.KindDecrypt:       tryPrefixed("DECRYPT", "passphrase", "aad", "encryption_method"),
		exp.KindDecryptRaw:    tryPrefixed("DECRYPT_RAW", "key", "iv", "aad", "encryption_method", "aead"),
		exp.KindDayOfMonth:    rename("DAYOFMONTH"),
		exp.KindDayOfWeek:     rename("DAYOFWEEK"),
		exp.KindDayOfWeekIso:  rename("DAYOFWEEKISO"),
		exp.KindDayOfYear:     rename("DAYOFYEAR"),
		exp.KindDotProduct:    rename("VECTOR_INNER_PRODUCT"),
		exp.KindExplode:       rename("FLATTEN"),
		exp.KindExtract: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_PART", []any{dialects.MapDatePart(e.This(), g.dialect), e.Expr()}, "(", ")", true)
		},
		exp.KindCosineDistance:    rename("VECTOR_COSINE_SIMILARITY"),
		exp.KindEuclideanDistance: rename("VECTOR_L2_DISTANCE"),
		exp.KindHandlerProperty: func(g *Generator, e exp.Expression) string {
			return "HANDLER = " + g.sqlKey(e, "this")
		},
		exp.KindFileFormatProperty: func(g *Generator, e exp.Expression) string {
			return "FILE_FORMAT=(" + g.expressions(exprsOptions{expression: e, key: "expressions", sep: " "}) + ")"
		},
		exp.KindFromTimeZone: func(g *Generator, e exp.Expression) string {
			return g.funcCall("CONVERT_TIMEZONE", []any{e.Arg("zone"), "'UTC'", e.This()}, "(", ")", true)
		},
		exp.KindGenerateSeries: func(g *Generator, e exp.Expression) string {
			end := e.Arg("end")
			if !truthy(e.Arg("is_end_exclusive")) {
				end = exp.BinOp(exp.KindAdd, asExpression(end), 1)
			}
			return g.funcCall("ARRAY_GENERATE_RANGE", []any{e.Arg("start"), end, e.Arg("step")}, "(", ")", true)
		},
		exp.KindGetExtract: rename("GET"),
		exp.KindGroupConcat: func(g *Generator, e exp.Expression) string {
			return g.listaggSQL(e, "LISTAGG", "", true, false)
		},
		exp.KindIf: func(g *Generator, e exp.Expression) string {
			falseValue := e.Arg("false")
			if asExpression(falseValue) == nil {
				falseValue = "NULL"
			}
			return g.funcCall("IFF", []any{e.This(), e.Arg("true"), falseValue}, "(", ")", true)
		},
		exp.KindJSONArray: func(g *Generator, e exp.Expression) string {
			args := []any{}
			for _, item := range e.Expressions() {
				args = append(args, item)
			}
			return g.funcCall("TO_VARIANT", []any{g.funcCall("ARRAY_CONSTRUCT", args, "(", ")", true)}, "(", ")", true)
		},
		exp.KindJSONExtractArray: (*Generator).snowflakeJSONExtractValueArraySQL,
		exp.KindJSONExtractScalar: func(g *Generator, e exp.Expression) string {
			return g.funcCall("JSON_EXTRACT_PATH_TEXT", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		exp.KindJSONKeys: rename("OBJECT_KEYS"),
		exp.KindJSONObject: func(g *Generator, e exp.Expression) string {
			args := []any{}
			for _, item := range e.Expressions() {
				args = append(args, item)
			}
			return g.funcCall("OBJECT_CONSTRUCT_KEEP_NULL", args, "(", ")", true)
		},
		exp.KindJSONPathRoot:   func(*Generator, exp.Expression) string { return "" },
		exp.KindJSONValueArray: (*Generator).snowflakeJSONExtractValueArraySQL,
		exp.KindLevenshtein: func(g *Generator, e exp.Expression) string {
			g.unsupportedArgs(e, "ins_cost", "del_cost", "sub_cost")
			return g.funcCall("EDITDISTANCE", g.fallbackArgs(e), "(", ")", true)
		},
		exp.KindLocationProperty: func(g *Generator, e exp.Expression) string {
			return "LOCATION=" + g.sqlKey(e, "this")
		},
		exp.KindLogicalAnd: rename("BOOLAND_AGG"),
		exp.KindLogicalOr:  rename("BOOLOR_AGG"),
		exp.KindMap: func(g *Generator, e exp.Expression) string {
			return g.varMapSQL(e, "OBJECT_CONSTRUCT")
		},
		exp.KindManhattanDistance: rename("VECTOR_L1_DISTANCE"),
		exp.KindMakeInterval:      (*Generator).noMakeIntervalSQL,
		exp.KindMax: func(g *Generator, e exp.Expression) string {
			if len(e.Expressions()) > 0 {
				return g.funcCall("GREATEST", g.fallbackArgs(e), "(", ")", true)
			}
			return g.funcCall("MAX", g.fallbackArgs(e), "(", ")", true)
		},
		exp.KindMin: func(g *Generator, e exp.Expression) string {
			if len(e.Expressions()) > 0 {
				return g.funcCall("LEAST", g.fallbackArgs(e), "(", ")", true)
			}
			return g.funcCall("MIN", g.fallbackArgs(e), "(", ")", true)
		},
		exp.KindParseJSON:  tryPrefixed("PARSE_JSON"),
		exp.KindToBinary:   tryPrefixed("TO_BINARY", "format"),
		exp.KindToBoolean:  tryPrefixed("TO_BOOLEAN"),
		exp.KindToDouble:   tryPrefixed("TO_DOUBLE", "format"),
		exp.KindToFile:     tryPrefixed("TO_FILE", "path"),
		exp.KindJSONFormat: rename("TO_JSON"),
		exp.KindPartitionedByProperty: func(g *Generator, e exp.Expression) string {
			return "PARTITION BY " + g.sqlKey(e, "this")
		},
		exp.KindPercentileCont: func(g *Generator, e exp.Expression) string {
			if out := transforms.AddWithinGroupForPercentiles(e); out.Kind() == exp.KindWithinGroup {
				return g.gen(out)
			}
			return g.functionFallbackSQL(e)
		},
		exp.KindPercentileDisc: func(g *Generator, e exp.Expression) string {
			if out := transforms.AddWithinGroupForPercentiles(e); out.Kind() == exp.KindWithinGroup {
				return g.gen(out)
			}
			return g.functionFallbackSQL(e)
		},
		exp.KindPivot: func(g *Generator, e exp.Expression) string {
			return g.pivotSQL(snowflakeUnqualifyPivotColumns(e))
		},
		exp.KindRegexpExtract:    (*Generator).snowflakeRegexpExtractSQL,
		exp.KindRegexpExtractAll: (*Generator).snowflakeRegexpExtractSQL,
		exp.KindRegexpILike: func(g *Generator, e exp.Expression) string {
			flag := e.Text("flag")
			if !strings.Contains(flag, "i") {
				flag += "i"
			}
			return g.funcCall("REGEXP_LIKE", []any{e.This(), e.Expr(), exp.LiteralString(flag)}, "(", ")", true)
		},
		exp.KindRowAccessProperty: (*Generator).snowflakeRowAccessPropertySQL,
		exp.KindSelect: func(g *Generator, e exp.Expression) string {
			out := transforms.EliminateWindowClause(e)
			out = transforms.EliminateDistinctOn(out)
			out = transforms.ExplodeProjectionToUnnest(0)(out)
			out = transforms.EliminateSemiAndAntiJoins(out)
			out = snowflakeTransformGenerateDateArray(out)
			out = snowflakeQualifyUnnestedColumns(out)
			out = snowflakeEliminateDotVariantLookup(out)
			if out.Kind() != exp.KindSelect {
				return g.gen(out)
			}
			return g.snowflakeSelectSQL(out)
		},
		exp.KindSHA:              rename("SHA1"),
		exp.KindSHA1Digest:       rename("SHA1_BINARY"),
		exp.KindMD5Digest:        rename("MD5_BINARY"),
		exp.KindMD5NumberLower64: rename("MD5_NUMBER_LOWER64"),
		exp.KindMD5NumberUpper64: rename("MD5_NUMBER_UPPER64"),
		exp.KindHex:              rename("HEX_ENCODE"),
		exp.KindLowerHex:         rename("TO_CHAR"),
		exp.KindSkewness:         rename("SKEW"),
		exp.KindStarMap:          rename("OBJECT_CONSTRUCT"),
		exp.KindStartsWith:       rename("STARTSWITH"),
		exp.KindEndsWith:         rename("ENDSWITH"),
		exp.KindRand: func(g *Generator, e exp.Expression) string {
			return g.funcCall("RANDOM", []any{e.This()}, "(", ")", true)
		},
		exp.KindStrPosition: func(g *Generator, e exp.Expression) string {
			return g.strPositionSQLFull(e, "CHARINDEX", true, false, true)
		},
		exp.KindStrToDate: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE", []any{e.This(), g.formatTime(e)}, "(", ")", true)
		},
		exp.KindStringToArray: rename("STRTOK_TO_ARRAY"),
		exp.KindStrtokToArray: rename("STRTOK_TO_ARRAY"),
		exp.KindStuff:         rename("INSERT"),
		exp.KindStPoint:       rename("ST_MAKEPOINT"),
		exp.KindTimeAdd:       dateDeltaSQL("TIMEADD", false),
		exp.KindTimeSlice: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TIME_SLICE", []any{e.This(), e.Expr(), unitToStr(e), e.Arg("kind")}, "(", ")", true)
		},
		exp.KindTimestamp:    (*Generator).noTimestampSQL,
		exp.KindTimestampAdd: dateDeltaSQL("TIMESTAMPADD", false),
		exp.KindTimestampDiff: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TIMESTAMPDIFF", []any{e.Arg("unit"), e.Expr(), e.This()}, "(", ")", true)
		},
		exp.KindTimestampTrunc: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_TRUNC", []any{weekstartUnitToStr(e), e.This()}, "(", ")", true)
		},
		exp.KindTimeStrToTime: func(g *Generator, e exp.Expression) string {
			return timeStrToTimeSQL(g, e, false)
		},
		exp.KindTimeToUnix: func(g *Generator, e exp.Expression) string {
			return "EXTRACT(epoch_second FROM " + g.sqlKey(e, "this") + ")"
		},
		exp.KindToArray:    rename("TO_ARRAY"),
		exp.KindToChar:     func(g *Generator, e exp.Expression) string { return g.functionFallbackSQL(e) },
		exp.KindTsOrDsAdd:  dateDeltaSQL("DATEADD", true),
		exp.KindTsOrDsDiff: dateDeltaSQL("DATEDIFF", false),
		exp.KindTsOrDsToDate: func(g *Generator, e exp.Expression) string {
			prefix := ""
			if truthy(e.Arg("safe")) {
				prefix = "TRY_"
			}
			return g.funcCall(prefix+"TO_DATE", []any{e.This(), g.formatTime(e)}, "(", ")", true)
		},
		exp.KindTsOrDsToTime: func(g *Generator, e exp.Expression) string {
			prefix := ""
			if truthy(e.Arg("safe")) {
				prefix = "TRY_"
			}
			return g.funcCall(prefix+"TO_TIME", []any{e.This(), g.formatTime(e)}, "(", ")", true)
		},
		exp.KindUnhex: rename("HEX_DECODE_BINARY"),
		exp.KindUnixToTime: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_TIMESTAMP", []any{e.This(), e.Arg("scale")}, "(", ")", true)
		},
		exp.KindUuid: rename("UUID_STRING"),
		exp.KindVarMap: func(g *Generator, e exp.Expression) string {
			return g.varMapSQL(e, "OBJECT_CONSTRUCT")
		},
		exp.KindBooland:       rename("BOOLAND"),
		exp.KindBoolor:        rename("BOOLOR"),
		exp.KindWeekOfYear:    rename("WEEKISO"),
		exp.KindYearOfWeek:    rename("YEAROFWEEK"),
		exp.KindYearOfWeekIso: rename("YEAROFWEEKISO"),
		exp.KindXor:           rename("BOOLXOR"),
		exp.KindByteLength:    rename("OCTET_LENGTH"),
		exp.KindFlatten:       rename("ARRAY_FLATTEN"),
		exp.KindArrayConcatAgg: func(g *Generator, e exp.Expression) string {
			return g.funcCall("ARRAY_FLATTEN", []any{exp.New(exp.KindArrayAgg, exp.Args{"this": e.This()})}, "(", ")", true)
		},
		exp.KindSHA2Digest: func(g *Generator, e exp.Expression) string {
			length := e.Arg("length")
			if asExpression(length) == nil {
				length = exp.LiteralNumber(256)
			}
			return g.funcCall("SHA2_BINARY", []any{e.This(), length}, "(", ")", true)
		},

		// Methods of SnowflakeGenerator.
		exp.KindDynamicIdentifier: (*Generator).snowflakeDynamicIdentifierSQL,
		exp.KindSortArray:         (*Generator).snowflakeSortArraySQL,
		exp.KindNthValue:          (*Generator).snowflakeNthValueSQL,
		exp.KindValues: func(g *Generator, e exp.Expression) string {
			valuesAsTable := g.dialect.ValuesAsTable
			if e.Find(exp.AnyKind{exp.KindMap, exp.KindStarMap, exp.KindStruct, exp.KindVarMap}) != nil {
				valuesAsTable = false
			}
			return g.valuesSQLWith(e, valuesAsTable)
		},
		exp.KindDataType: (*Generator).snowflakeDataTypeSQL,
		exp.KindToNumber: (*Generator).snowflakeToNumberSQL,
		exp.KindTimestampFromParts: func(g *Generator, e exp.Expression) string {
			if milli := asExpression(e.Arg("milli")); milli != nil {
				e.Set("nano", exp.BinOp(exp.KindMul, milli.Pop(), exp.LiteralNumber(1000000)))
			}
			return g.funcCall("TIMESTAMP_FROM_PARTS", g.fallbackArgs(e), "(", ")", true)
		},
		exp.KindCast:      (*Generator).snowflakeCastSQL,
		exp.KindTryCast:   (*Generator).snowflakeTryCastSQL,
		exp.KindLog:       (*Generator).snowflakeLogSQL,
		exp.KindGreatest:  snowflakeGreatestLeastSQL("GREATEST"),
		exp.KindLeast:     snowflakeGreatestLeastSQL("LEAST"),
		exp.KindGenerator: (*Generator).snowflakeGeneratorSQL,
		exp.KindUnnest:    (*Generator).snowflakeUnnestSQL,
		exp.KindUndrop:    (*Generator).snowflakeUndropSQL,
		exp.KindShow:      (*Generator).snowflakeShowSQL,
		exp.KindDescribe:  (*Generator).snowflakeDescribeSQL,
		exp.KindGeneratedAsIdentityColumnConstraint: (*Generator).snowflakeGeneratedAsIdentitySQL,
		exp.KindStruct:         (*Generator).snowflakeStructSQL,
		exp.KindApproxQuantile: (*Generator).snowflakeApproxQuantileSQL,
		exp.KindAlterSet:       (*Generator).snowflakeAlterSetSQL,
		exp.KindStrToTime:      (*Generator).snowflakeStrToTimeSQL,
		exp.KindTimestampSub: func(g *Generator, e exp.Expression) string {
			return g.gen(exp.New(exp.KindTimestampAdd, exp.Args{
				"this":       e.This(),
				"expression": exp.BinOp(exp.KindMul, e.Expr(), -1),
				"unit":       e.Arg("unit"),
			}))
		},
		exp.KindJSONExtract:   (*Generator).snowflakeJSONExtractSQL,
		exp.KindTimeToStr:     (*Generator).snowflakeTimeToStrSQL,
		exp.KindDateSub:       (*Generator).snowflakeDateSubSQL,
		exp.KindArrayAgg:      (*Generator).snowflakeArrayAggSQL,
		exp.KindArrayDistinct: (*Generator).snowflakeArrayDistinctSQL,
		exp.KindArrayToString: func(g *Generator, e exp.Expression) string {
			return g.funcCall("ARRAY_TO_STRING", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		exp.KindCurrentDate: (*Generator).snowflakeCurrentDateSQL,
		exp.KindDot:         (*Generator).snowflakeDotSQL,
		exp.KindModelAttribute: func(g *Generator, e exp.Expression) string {
			return g.sqlKey(e, "this") + "!" + g.sqlKey(e, "expression")
		},
		exp.KindFormat:      (*Generator).snowflakeFormatSQL,
		exp.KindSplitPart:   (*Generator).snowflakeSplitPartSQL,
		exp.KindUniform:     (*Generator).snowflakeUniformSQL,
		exp.KindWindow:      (*Generator).snowflakeWindowSQL,
		exp.KindFilter:      (*Generator).snowflakeFilterSQL,
		exp.KindWithinGroup: (*Generator).snowflakeWithinGroupSQL,
	})
}

// dateDeltaSQL ports date_delta_sql (dialect.py:2014-2026).
func dateDeltaSQL(name string, cast bool) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		if cast && e.Kind() == exp.KindTsOrDsAdd {
			tsOrDsAddCast(e)
		}
		return g.funcCall(name, []any{unitToVar(e, "DAY"), e.Expr(), e.This()}, "(", ")", true)
	}
}

// timestampDiffSQLFunc ports timestampdiff_sql (dialect.py:2519-2520).
func timestampDiffSQLFunc(g *Generator, e exp.Expression) string {
	return g.funcCall("TIMESTAMPDIFF", []any{e.Arg("unit"), e.Expr(), e.This()}, "(", ")", true)
}

// dateStrToDateSQL ports datestrtodate_sql (dialect.py:1787-1788).
func dateStrToDateSQL(g *Generator, e exp.Expression) string {
	return g.gen(exp.CastTo(e.This(), exp.DTypeDate))
}

// arrayAppendSQLFunc ports array_append_sql (dialect.py:1370-1414).
func arrayAppendSQLFunc(name string, swap bool) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string { return g.arrayAppendSQL(e, name, swap) }
}

// arrayConcatSQL ports array_concat_sql (dialect.py:1439-1518).
func arrayConcatSQL(name string, varLen bool) func(*Generator, exp.Expression) string {
	buildFuncCall := func(g *Generator, args []exp.Expression) string {
		if varLen {
			items := make([]any, len(args))
			for i, arg := range args {
				items[i] = arg
			}
			return g.funcCall(name, items, "(", ")", true)
		}
		if len(args) == 1 {
			// Single arg gets empty array to preserve semantics
			return g.funcCall(name, []any{args[0], exp.Array(exp.Args{"expressions": []exp.Expression{}})}, "(", ")", true)
		}
		// Snowflake/PostgreSQL/Redshift require binary nesting: ARRAY_CAT(a, ARRAY_CAT(b, c))
		result := g.funcCall(name, []any{args[len(args)-2], args[len(args)-1]}, "(", ")", true)
		for i := len(args) - 3; i >= 0; i-- {
			result = name + "(" + g.gen(args[i]) + ", " + result + ")"
		}
		return result
	}
	return func(g *Generator, e exp.Expression) string {
		this := e.This()
		exprs := e.Expressions()
		allArgs := append([]exp.Expression{this}, exprs...)
		source := truthy(e.Arg("null_propagation"))
		target := g.dialect.ArrayFuncsPropagatesNulls
		if source == target || (this != nil && this.Kind() == exp.KindArray) || len(exprs) == 0 {
			return buildFuncCall(g, allArgs)
		}
		if source {
			var combined exp.Expression
			for _, arg := range allArgs {
				check := exp.Is(exp.Args{"this": arg.Copy(), "expression": exp.Null()})
				if combined == nil {
					combined = check
				} else {
					combined = exp.Or(exp.Args{"this": combined, "expression": check})
				}
			}
			return g.gen(exp.If(exp.Args{"this": combined, "true": exp.Null(), "false": buildFuncCall(g, allArgs)}))
		}
		wrapped := make([]exp.Expression, len(allArgs))
		for i, arg := range allArgs {
			wrapped[i] = exp.Coalesce(exp.Args{"expressions": []exp.Expression{arg.Copy(), exp.Array(exp.Args{"expressions": []exp.Expression{}})}})
		}
		return buildFuncCall(g, wrapped)
	}
}

// varMapSQL ports var_map_sql (dialect.py:1521-1537).
func (g *Generator) varMapSQL(e exp.Expression, mapFuncName string) string {
	keys, values := asExpression(e.Arg("keys")), asExpression(e.Arg("values"))
	if keys == nil || values == nil || keys.Kind() != exp.KindArray || values.Kind() != exp.KindArray {
		g.unsupported("Cannot convert array columns into map.")
		return g.funcCall(mapFuncName, []any{keys, values}, "(", ")", true)
	}
	args := []any{}
	ks, vs := keys.Expressions(), values.Expressions()
	for i := 0; i < len(ks) && i < len(vs); i++ {
		args = append(args, g.gen(ks[i]), g.gen(vs[i]))
	}
	return g.funcCall(mapFuncName, args, "(", ")", true)
}

// noMakeIntervalSQL ports no_make_interval_sql (dialect.py:2523-2531).
func (g *Generator) noMakeIntervalSQL(e exp.Expression) string {
	args := []any{}
	for _, unit := range exp.ArgKeys(e.Kind()) {
		value := e.Arg(unit)
		if value == nil {
			continue
		}
		if v := asExpression(value); v != nil && v.Kind() == exp.KindKwarg {
			value = v.Expr()
		}
		args = append(args, g.gen(value)+" "+unit)
	}
	return "INTERVAL '" + g.formatArgs(args, ", ") + "'"
}

// noTimestampSQL ports no_timestamp_sql (dialect.py:1712-1726).
func (g *Generator) noTimestampSQL(e exp.Expression) string {
	zone := asExpression(e.Arg("zone"))
	if zone == nil {
		targetType := exp.DTypeTimestamp
		if t := g.annotateTypes(e).Type(); t != nil {
			if d, ok := t.Arg("this").(exp.DType); ok {
				targetType = d
			}
		}
		return g.gen(exp.CastTo(e.This(), targetType))
	}
	if dialects.IsTimezone(zone.Name()) {
		return g.gen(exp.AtTimeZone(exp.Args{"this": exp.CastTo(e.This(), exp.DTypeTimestamp), "zone": zone}))
	}
	return g.funcCall("TIMESTAMP", []any{e.This(), zone}, "(", ")", true)
}

// listaggSQL ports groupconcat_sql (dialect.py:2539-2583).
func (g *Generator) listaggSQL(e exp.Expression, funcName, sep string, withinGroup, onOverflow bool) string {
	this := e.This()
	separatorExpr := asExpression(e.Arg("separator"))
	if separatorExpr == nil && sep != "" {
		separatorExpr = exp.LiteralString(sep)
	}
	separator := g.gen(separatorExpr)
	onOverflowSQL := g.sqlKey(e, "on_overflow")
	if onOverflow && onOverflowSQL != "" {
		onOverflowSQL = " ON OVERFLOW " + onOverflowSQL
	} else {
		onOverflowSQL = ""
	}
	var limit exp.Expression
	if this != nil && this.Kind() == exp.KindLimit && this.This() != nil {
		limit = this
		this = limit.This().Pop()
	}
	var order exp.Expression
	if this != nil {
		order = this.Find(exp.KindOrder)
	}
	if order != nil && order.This() != nil {
		this = order.This().Pop()
	}
	var separatorArg any
	if separator != "" || onOverflowSQL != "" {
		separatorArg = separator + onOverflowSQL
	}
	args := g.formatArgs([]any{this, separatorArg}, ", ")
	listagg := exp.Anonymous(exp.Args{"this": funcName, "expressions": []any{args}})
	modifiers := g.gen(limit)
	if order != nil {
		if withinGroup {
			listagg = exp.New(exp.KindWithinGroup, exp.Args{"this": listagg, "expression": order})
		} else {
			modifiers = g.gen(order) + modifiers
		}
	}
	if modifiers != "" {
		target := listagg
		if target.Kind() == exp.KindWithinGroup {
			target = target.This()
		}
		target.Set("expressions", []any{args + modifiers})
	}
	return g.gen(listagg)
}

// snowflakeRegexpExtractSQL ports _regexpextract_sql (generators/snowflake.py:197-220).
func (g *Generator) snowflakeRegexpExtractSQL(e exp.Expression) string {
	// Other dialects don't support all of the following parameters, so we need to
	// generate default values as necessary to ensure the transpilation is correct
	group := asExpression(e.Arg("group"))
	// To avoid generating all these default values, we set group to None if
	// it's 0 (also default value) which doesn't trigger the following chain
	if group != nil && group.Name() == "0" {
		group = nil
	}
	parameters := asExpression(e.Arg("parameters"))
	if parameters == nil && group != nil {
		parameters = exp.LiteralString("c")
	}
	occurrence := asExpression(e.Arg("occurrence"))
	if occurrence == nil && parameters != nil {
		occurrence = exp.LiteralNumber(1)
	}
	position := asExpression(e.Arg("position"))
	if position == nil && occurrence != nil {
		position = exp.LiteralNumber(1)
	}
	name := "REGEXP_SUBSTR_ALL"
	if e.Kind() == exp.KindRegexpExtract {
		name = "REGEXP_SUBSTR"
	}
	return g.funcCall(name, []any{e.This(), e.Expr(), position, occurrence, parameters, group}, "(", ")", true)
}

// snowflakeJSONExtractValueArraySQL ports _json_extract_value_array_sql (generators/
// snowflake.py:223-237).
func (g *Generator) snowflakeJSONExtractValueArraySQL(e exp.Expression) string {
	jsonExtract := exp.New(exp.KindJSONExtract, exp.Args{"this": e.This(), "expression": e.Expr()})
	ident := exp.ToIdentifier("x")
	var this exp.Expression
	if e.Kind() == exp.KindJSONValueArray {
		this = exp.CastTo(ident, exp.DTypeVarchar)
	} else {
		this = exp.New(exp.KindParseJSON, exp.Args{"this": "TO_JSON(x)"})
	}
	lambda := exp.New(exp.KindLambda, exp.Args{"expressions": []exp.Expression{ident.Copy()}, "this": this})
	return g.funcCall("TRANSFORM", []any{jsonExtract, lambda}, "(", ")", true)
}

// snowflakeUnqualifyPivotColumns ports _unqualify_pivot_columns (generators/snowflake.py:68-89).
func snowflakeUnqualifyPivotColumns(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindPivot {
		return e
	}
	if truthy(e.Arg("unpivot")) {
		return transforms.UnqualifyColumns(e)
	}
	for _, field := range exp.ExpressionsOf(e, "fields") {
		var fieldExpr exp.Expression
		if field != nil {
			if exprs := field.Expressions(); len(exprs) > 0 {
				fieldExpr = exprs[0]
			}
		}
		if fieldExpr != nil && fieldExpr.Kind() == exp.KindPivotAny {
			field.SetAt("expressions", transforms.UnqualifyColumns(fieldExpr), 0, true)
		}
	}
	return e
}

// snowflakeFlattenStructuredTypesUnlessIceberg ports _flatten_structured_types_unless_iceberg
// (generators/snowflake.py:92-110).
func snowflakeFlattenStructuredTypesUnlessIceberg(e exp.Expression) exp.Expression {
	props := asExpression(e.Arg("properties"))
	this := e.This()
	if this == nil || this.Kind() != exp.KindSchema || (props != nil && props.Find(exp.KindIcebergProperty) != nil) {
		return e
	}
	for _, schemaExpression := range this.Expressions() {
		if schemaExpression.Kind() != exp.KindColumnDef {
			continue
		}
		columnType := asExpression(schemaExpression.Arg("kind"))
		if columnType == nil || columnType.Kind() != exp.KindDataType {
			continue
		}
		for _, node := range columnType.Walk() {
			if node.Kind() == exp.KindDataType {
				if dtype, ok := node.Arg("this").(exp.DType); ok && exp.NestedTypes[dtype] {
					node.Set("expressions", nil)
				}
			}
		}
	}
	return e
}

func (g *Generator) snowflakeDynamicIdentifierSQL(e exp.Expression) string {
	this := g.funcCall("IDENTIFIER", []any{e.This()}, "(", ")", true)
	if exp.HasArg(e, "expressions") {
		// `IDENTIFIER(...)` invoked as a function, e.g. `IDENTIFIER('my_func')(1, 2)`
		args := []any{}
		for _, item := range e.Expressions() {
			args = append(args, item)
		}
		return g.funcCall(this, args, "(", ")", false)
	}
	return this
}

func (g *Generator) snowflakeSortArraySQL(e exp.Expression) string {
	asc := asExpression(e.Arg("asc"))
	nullsFirst := asExpression(e.Arg("nulls_first"))
	if isBooleanLiteral(asc, false) && isBooleanLiteral(nullsFirst, true) {
		nullsFirst = nil
	}
	return g.funcCall("ARRAY_SORT", []any{e.This(), asc, nullsFirst}, "(", ")", true)
}

func isBooleanLiteral(e exp.Expression, value bool) bool {
	if e == nil || e.Kind() != exp.KindBoolean {
		return false
	}
	b, ok := e.Arg("this").(bool)
	return ok && b == value
}

func (g *Generator) snowflakeNthValueSQL(e exp.Expression) string {
	result := g.funcCall("NTH_VALUE", []any{e.This(), e.Arg("offset")}, "(", ")", true)
	if fromFirst, ok := e.Arg("from_first").(bool); ok {
		if fromFirst {
			result += " FROM FIRST"
		} else {
			result += " FROM LAST"
		}
	}
	return result
}

func (g *Generator) snowflakeDataTypeSQL(e exp.Expression) string {
	// Check if this is a FLOAT type nested inside a VECTOR type
	// VECTOR only accepts FLOAT (not DOUBLE), INT, and STRING as element types
	if exp.IsType(e, exp.DTypeDouble) {
		if parent := e.Parent(); parent != nil && exp.IsType(parent, exp.DTypeVector) {
			// Preserve FLOAT for VECTOR types instead of mapping to synonym DOUBLE
			return "FLOAT"
		}
	}
	expressions := e.Expressions()
	if dtype, ok := e.Arg("this").(exp.DType); ok && len(expressions) > 0 && exp.StructTypes[dtype] {
		for _, fieldType := range expressions {
			// The correct syntax is OBJECT [ (<key> <value_type [NOT NULL] [, ...]) ]
			if fieldType.Kind() == exp.KindDataType {
				return "OBJECT"
			}
			if fieldType.Kind() == exp.KindColumnDef && fieldType.This() != nil && fieldType.This().IsString() {
				// Doing OBJECT('foo' VARCHAR) is invalid snowflake Syntax. Moreover, besides
				// converting 'foo' into an identifier, we also need to quote it because these
				// keys are case-sensitive.
				fieldType.This().Replace(exp.ToIdentifier(fieldType.Name(), true))
			}
		}
	}
	return g.dataTypeSQL(e)
}

func (g *Generator) snowflakeToNumberSQL(e exp.Expression) string {
	precision := asExpression(e.Arg("precision"))
	scale := asExpression(e.Arg("scale"))
	defaultPrecision := precision != nil && precision.Kind() == exp.KindLiteral && precision.Name() == "38"
	defaultScale := scale != nil && scale.Kind() == exp.KindLiteral && scale.Name() == "0"
	if defaultPrecision && defaultScale {
		precision, scale = nil, nil
	} else if defaultScale {
		scale = nil
	}
	name := "TO_NUMBER"
	if truthy(e.Arg("safe")) {
		name = "TRY_TO_NUMBER"
	}
	return g.funcCall(name, []any{e.This(), e.Arg("format"), precision, scale}, "(", ")", true)
}

func (g *Generator) snowflakeCastSQL(e exp.Expression) string {
	to := asExpression(e.Arg("to"))
	if exp.IsType(to, exp.DTypeGeography) {
		return g.funcCall("TO_GEOGRAPHY", []any{e.This()}, "(", ")", true)
	}
	if exp.IsType(to, exp.DTypeGeometry) {
		return g.funcCall("TO_GEOMETRY", []any{e.This()}, "(", ")", true)
	}
	return g.castSQL(e)
}

// snowflakeTryCastSQL ports trycast_sql (generators/snowflake.py:729-743).
func (g *Generator) snowflakeTryCastSQL(e exp.Expression) string {
	value := e.This()
	if value != nil && value.Type() == nil {
		value = g.annotateTypes(value)
	}

	// Snowflake requires that TRY_CAST's value be a string
	// If TRY_CAST is being roundtripped (since Snowflake is the only dialect that sets "requires_string") or
	// if we can deduce that the value is a string, then we can generate TRY_CAST
	if truthy(e.Arg("requires_string")) || (value != nil && value.IsType(typeArgs(exp.TextTypes)...)) {
		return g.castSQLWithPrefix(e, "TRY_")
	}
	return g.snowflakeCastSQL(e)
}

func (g *Generator) snowflakeLogSQL(e exp.Expression) string {
	if e.Expr() == nil {
		return g.funcCall("LN", []any{e.This()}, "(", ")", true)
	}
	return g.logSQL(e)
}

func snowflakeGreatestLeastSQL(name string) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		name := name
		if truthy(e.Arg("ignore_nulls")) {
			name += "_IGNORE_NULLS"
		}
		args := []any{e.This()}
		for _, item := range e.Expressions() {
			args = append(args, item)
		}
		return g.funcCall(name, args, "(", ")", true)
	}
}

func (g *Generator) snowflakeGeneratorSQL(e exp.Expression) string {
	args := []any{}
	if rowcount := asExpression(e.Arg("rowcount")); rowcount != nil {
		args = append(args, exp.New(exp.KindKwarg, exp.Args{"this": exp.Var(exp.Args{"this": "ROWCOUNT"}), "expression": rowcount}))
	}
	if timelimit := asExpression(e.Arg("timelimit")); timelimit != nil {
		args = append(args, exp.New(exp.KindKwarg, exp.Args{"this": exp.Var(exp.Args{"this": "TIMELIMIT"}), "expression": timelimit}))
	}
	return g.funcCall("GENERATOR", args, "(", ")", true)
}

func (g *Generator) snowflakeUnnestSQL(e exp.Expression) string {
	unnestAlias := asExpression(e.Arg("alias"))
	offset := e.Arg("offset")
	var aliasColumns []exp.Expression
	if unnestAlias != nil {
		aliasColumns = exp.ExpressionsOf(unnestAlias, "columns")
	}
	var value exp.Expression
	if len(aliasColumns) > 0 {
		value = aliasColumns[0]
	} else {
		value = exp.ToIdentifier("value")
	}
	var index exp.Expression
	if offsetExpr := asExpression(offset); offsetExpr != nil {
		index = offsetExpr.Pop()
	} else {
		index = exp.ToIdentifier("index")
	}
	columns := []exp.Expression{exp.ToIdentifier("seq"), exp.ToIdentifier("key"), exp.ToIdentifier("path"), index, value, exp.ToIdentifier("this")}
	if unnestAlias != nil {
		unnestAlias.Set("columns", columns)
	} else {
		unnestAlias = exp.TableAlias(exp.Args{"this": exp.ToIdentifier("_u"), "columns": columns})
	}
	tableInput := g.gen(e.Expressions()[0])
	if !strings.HasPrefix(tableInput, "INPUT =>") {
		tableInput = "INPUT => " + tableInput
	}
	parent := e.Parent()
	explode := "TABLE(FLATTEN(" + tableInput + "))"
	if parent != nil && parent.Kind() == exp.KindLateral {
		explode = "FLATTEN(" + tableInput + ")"
	}
	alias := g.gen(unnestAlias)
	if alias != "" {
		alias = " AS " + alias
	}
	valueSQL := g.gen(value) + " FROM "
	if parent != nil && (parent.Kind() == exp.KindFrom || parent.Kind() == exp.KindJoin || parent.Kind() == exp.KindLateral) {
		valueSQL = ""
	}
	return valueSQL + explode + alias
}

func (g *Generator) snowflakeUndropSQL(e exp.Expression) string {
	rename := g.sqlKey(e, "rename")
	if rename != "" {
		rename = " RENAME TO " + rename
	}
	return "UNDROP " + e.Text("kind") + " " + g.sqlKey(e, "this") + rename
}

func (g *Generator) snowflakeShowSQL(e exp.Expression) string {
	terse := ""
	if truthy(e.Arg("terse")) {
		terse = "TERSE "
	}
	iceberg := ""
	if truthy(e.Arg("iceberg")) {
		iceberg = "ICEBERG "
	}
	history := ""
	if truthy(e.Arg("history")) {
		history = " HISTORY"
	}
	like := g.sqlKey(e, "like")
	if like != "" {
		like = " LIKE " + like
	}
	scope := g.sqlKey(e, "scope")
	if scope != "" {
		scope = " " + scope
	}
	scopeKind := g.sqlKey(e, "scope_kind")
	if scopeKind != "" {
		scopeKind = " IN " + scopeKind
	}
	startsWith := g.sqlKey(e, "starts_with")
	if startsWith != "" {
		startsWith = " STARTS WITH " + startsWith
	}
	limit := g.sqlKey(e, "limit")
	from := g.sqlKey(e, "from_")
	if from != "" {
		from = " FROM " + from
	}
	privileges := g.expressions(exprsOptions{expression: e, key: "privileges", flat: true})
	if privileges != "" {
		privileges = " WITH PRIVILEGES " + privileges
	}
	return "SHOW " + terse + iceberg + e.Name() + history + like + scopeKind + scope + startsWith + limit + from + privileges
}

func (g *Generator) snowflakeRowAccessPropertySQL(e exp.Expression) string {
	if e.This() == nil {
		return "ROW ACCESS"
	}
	on := ""
	if len(e.Expressions()) > 0 {
		on = " ON (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
	}
	return "WITH ROW ACCESS POLICY " + g.sqlKey(e, "this") + on
}

func (g *Generator) snowflakeDescribeSQL(e exp.Expression) string {
	kindValue := e.Text("kind")
	if kindValue == "" {
		kindValue = "TABLE"
	}
	kind := " " + kindValue
	if properties := asExpression(e.Arg("properties")); properties != nil {
		kind = " " + g.expressions(exprsOptions{expression: properties, sep: " "}) + " " + kindValue
	}
	this := " " + g.sqlKey(e, "this")
	expressions := g.expressions(exprsOptions{expression: e, flat: true})
	if expressions != "" {
		expressions = " " + expressions
	}
	return "DESCRIBE" + kind + this + expressions
}

func (g *Generator) snowflakeGeneratedAsIdentitySQL(e exp.Expression) string {
	start := g.sqlKey(e, "start")
	if start != "" {
		start = " START " + start
	}
	increment := g.sqlKey(e, "increment")
	if increment != "" {
		increment = " INCREMENT " + increment
	}
	orderClause := ""
	if order, ok := e.Arg("order").(bool); ok {
		if order {
			orderClause = " ORDER"
		} else {
			orderClause = " NOORDER"
		}
	}
	return "AUTOINCREMENT" + start + increment + orderClause
}

func (g *Generator) snowflakeStructSQL(e exp.Expression) string {
	expressions := e.Expressions()
	if len(expressions) == 1 {
		arg := expressions[0]
		if arg.IsStar() || (arg.Kind() == exp.KindILike && arg.This() != nil && arg.This().IsStar()) {
			// Wildcard syntax: https://docs.snowflake.com/en/sql-reference/data-types-semistructured#object
			return "{" + g.gen(arg) + "}"
		}
	}
	args := []any{}
	for i, item := range expressions {
		if item.Kind() == exp.KindPropertyEQ {
			var key exp.Expression = item.This()
			if key != nil && key.Kind() == exp.KindIdentifier {
				key = exp.LiteralString(item.Name())
			}
			args = append(args, key, item.Expr())
		} else {
			args = append(args, exp.LiteralString("_"+strconv.Itoa(i)), item)
		}
	}
	return g.funcCall("OBJECT_CONSTRUCT", args, "(", ")", true)
}

func (g *Generator) snowflakeApproxQuantileSQL(e exp.Expression) string {
	g.unsupportedArgs(e, "weight", "accuracy")
	return g.funcCall("APPROX_PERCENTILE", []any{e.This(), e.Arg("quantile")}, "(", ")", true)
}

func (g *Generator) snowflakeAlterSetSQL(e exp.Expression) string {
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = " " + exprs
	}
	fileFormat := g.expressions(exprsOptions{expression: e, key: "file_format", flat: true, sep: " "})
	if fileFormat != "" {
		fileFormat = " STAGE_FILE_FORMAT = (" + fileFormat + ")"
	}
	copyOptions := g.expressions(exprsOptions{expression: e, key: "copy_options", flat: true, sep: " "})
	if copyOptions != "" {
		copyOptions = " STAGE_COPY_OPTIONS = (" + copyOptions + ")"
	}
	tag := g.expressions(exprsOptions{expression: e, key: "tag", flat: true})
	if tag != "" {
		tag = " TAG " + tag
	}
	return "SET" + exprs + fileFormat + copyOptions + tag
}

func (g *Generator) snowflakeStrToTimeSQL(e exp.Expression) string {
	// target_type is stored as a DataType instance
	// Get the type enum from DataType instance or from type annotation
	typeEnum := exp.DTypeTimestamp
	if targetType := asExpression(e.Arg("target_type")); targetType != nil && targetType.Kind() == exp.KindDataType {
		typeEnum, _ = targetType.Arg("this").(exp.DType)
	} else if t := e.Type(); t != nil {
		typeEnum, _ = t.Arg("this").(exp.DType)
	}
	funcName, ok := dialects.SnowflakeTimestampFunc(typeEnum)
	if !ok {
		funcName = "TO_TIMESTAMP"
	}
	if truthy(e.Arg("safe")) {
		funcName = "TRY_" + funcName
	}
	return g.funcCall(funcName, []any{e.This(), g.formatTime(e)}, "(", ")", true)
}

func (g *Generator) snowflakeJSONExtractSQL(e exp.Expression) string {
	this := e.This()
	// JSON strings are valid coming from other dialects such as BQ so
	// for these cases we PARSE_JSON preemptively
	if this != nil && this.Kind() != exp.KindParseJSON && this.Kind() != exp.KindJSONExtract && !truthy(e.Arg("requires_json")) {
		this = exp.New(exp.KindParseJSON, exp.Args{"this": this})
	}
	return g.funcCall("GET_PATH", []any{this, e.Expr()}, "(", ")", true)
}

func (g *Generator) snowflakeTimeToStrSQL(e exp.Expression) string {
	this := e.This()
	if this != nil && this.IsString() {
		this = exp.CastTo(this, exp.DTypeTimestamp)
	}
	return g.funcCall("TO_CHAR", []any{this, g.formatTime(e)}, "(", ")", true)
}

func (g *Generator) snowflakeDateSubSQL(e exp.Expression) string {
	if value := e.Expr(); value != nil {
		value.Replace(exp.BinOp(exp.KindMul, value, -1))
	} else {
		g.unsupported("DateSub cannot be transpiled if the subtracted count is unknown")
	}
	return dateDeltaSQL("DATEADD", false)(g, e)
}

func (g *Generator) snowflakeSelectSQL(e exp.Expression) string {
	if e.Arg("offset") != nil && e.Arg("limit") == nil {
		e.Set("limit", exp.New(exp.KindLimit, exp.Args{"expression": exp.Null()}))
	}
	return g.selectSQL(e)
}

// snowflakeCreateableSQL ports createable_sql (generators/snowflake.py:916-935).
func (g *Generator) snowflakeCreateableSQL(e exp.Expression, locations map[propertyLocation][]exp.Expression) string {
	isMaterialized := e.Find(exp.KindMaterializedProperty)
	copyGrants := e.Find(exp.KindCopyGrantsProperty)
	if e.Text("kind") == "VIEW" && isMaterialized != nil && copyGrants != nil {
		// For materialized views, COPY GRANTS is located *before* the columns list
		postSchema := locations[propertyLocationPostSchema]
		for i, prop := range postSchema {
			if prop == copyGrants {
				locations[propertyLocationPostSchema] = append(postSchema[:i:i], postSchema[i+1:]...)
				break
			}
		}
		thisName := g.sqlKey(e.This(), "this")
		copyGrantsSQL := g.gen(copyGrants)
		thisSchema := g.schemaColumnsSQL(e.This())
		if thisSchema != "" {
			thisSchema = g.sep() + thisSchema
		}
		return thisName + g.sep() + copyGrantsSQL + thisSchema
	}
	return g.sqlKey(e, "this")
}

func (g *Generator) snowflakeArrayAggSQL(e exp.Expression) string {
	this := e.This()
	// If an ORDER BY clause is present, we need to remove it from ARRAY_AGG
	// and add it later as part of the WITHIN GROUP clause
	var order exp.Expression
	if this != nil && this.Kind() == exp.KindOrder {
		order = this
		e.Set("this", order.This().Pop())
	}
	sql := g.arrayAggSQL(e)
	if order != nil {
		sql = g.gen(exp.New(exp.KindWithinGroup, exp.Args{"this": sql, "expression": order}))
	}
	return sql
}

func (g *Generator) snowflakeArrayDistinctSQL(e exp.Expression) string {
	if truthy(e.Arg("check_null")) {
		return g.funcCall("ARRAY_DISTINCT", []any{e.This()}, "(", ")", true)
	}
	return g.funcCall("ARRAY_DISTINCT", []any{exp.New(exp.KindArrayCompact, exp.Args{"this": e.This()})}, "(", ")", true)
}

func (g *Generator) snowflakeArraySQL(e exp.Expression) string {
	expressions := e.Expressions()
	if len(expressions) > 0 && expressions[0].Kind() == exp.KindSelect {
		first := expressions[0]
		// SELECT AS STRUCT foo AS alias_foo -> ARRAY_AGG(OBJECT_CONSTRUCT('alias_foo', foo))
		if strings.ToUpper(first.Text("kind")) == "STRUCT" {
			args := []exp.Expression{}
			for _, item := range first.Expressions() {
				name := item
				if item.Kind() == exp.KindAlias {
					name = item.This()
				}
				args = append(args, exp.LiteralString(item.AliasOrName()), name)
			}
			arrayAgg := exp.New(exp.KindArrayAgg, exp.Args{"this": dialects.SnowflakeBuildObjectConstruct(args)})
			first.Set("kind", nil)
			first.Set("expressions", []exp.Expression{arrayAgg})
			return g.gen(exp.Subquery(exp.Args{"this": first}))
		}
	}
	return g.inlineArraySQL(e)
}

func (g *Generator) snowflakeCurrentDateSQL(e exp.Expression) string {
	zone := g.sqlKey(e, "this")
	if zone == "" {
		return g.currentDateSQL(e)
	}
	return g.gen(exp.Cast(exp.Args{
		"this": exp.New(exp.KindConvertTimezone, exp.Args{"target_tz": e.This(), "timestamp": exp.New(exp.KindCurrentTimestamp, nil)}),
		"to":   exp.DTypeDate.IntoExpr(nil),
	}))
}

// snowflakeDotSQL ports dot_sql (generators/snowflake.py:996-1009).
func (g *Generator) snowflakeDotSQL(e exp.Expression) string {
	this := e.This()
	if this != nil && this.Type() == nil {
		this = g.annotateTypes(this)
	}

	if this != nil && this.Kind() != exp.KindDot && this.IsType(exp.DTypeStruct) {
		// Generate colon notation for the top level STRUCT
		return g.gen(this) + ":" + g.sqlKey(e, "expression")
	}
	return g.dotSQL(e)
}

func (g *Generator) snowflakeFormatSQL(e exp.Expression) string {
	if strings.ToLower(e.Name()) == "%s" && len(e.Expressions()) == 1 {
		return g.funcCall("TO_CHAR", []any{e.Expressions()[0]}, "(", ")", true)
	}
	return g.functionFallbackSQL(e)
}

func (g *Generator) snowflakeSplitPartSQL(e exp.Expression) string {
	// Set part_index to 1 if missing
	if e.Arg("delimiter") == nil {
		e.Set("delimiter", exp.LiteralString(" "))
	}
	if e.Arg("part_index") == nil {
		e.Set("part_index", exp.LiteralNumber(1))
	}
	return g.funcCall("SPLIT_PART", g.fallbackArgs(e), "(", ")", true)
}

func (g *Generator) snowflakeUniformSQL(e exp.Expression) string {
	gen := asExpression(e.Arg("gen"))
	// From Databricks UNIFORM(min, max, seed) -> Wrap gen in RANDOM(seed)
	if seed := asExpression(e.Arg("seed")); seed != nil {
		gen = exp.New(exp.KindRand, exp.Args{"this": seed})
	}
	// No gen argument (from Databricks 2-arg UNIFORM(min, max)) -> Add RANDOM()
	if gen == nil {
		gen = exp.New(exp.KindRand, nil)
	}
	return g.funcCall("UNIFORM", []any{e.This(), e.Expr(), gen}, "(", ")", true)
}

func snowflakeIsRankingWithFrame(e exp.Expression) bool {
	if e == nil {
		return false
	}
	switch e.Kind() {
	case exp.KindFirstValue, exp.KindLastValue, exp.KindNthValue:
		return true
	}
	return false
}

func (g *Generator) snowflakeWindowSQL(e exp.Expression) string {
	spec := asExpression(e.Arg("spec"))
	this := e.This()
	ranking := snowflakeIsRankingWithFrame(this) ||
		(this != nil && (this.Kind() == exp.KindRespectNulls || this.Kind() == exp.KindIgnoreNulls) && snowflakeIsRankingWithFrame(this.This()))
	if ranking && spec != nil &&
		strings.ToUpper(spec.Text("kind")) == "ROWS" &&
		strings.ToUpper(spec.Text("start")) == "UNBOUNDED" &&
		strings.ToUpper(spec.Text("start_side")) == "PRECEDING" &&
		strings.ToUpper(spec.Text("end")) == "UNBOUNDED" &&
		strings.ToUpper(spec.Text("end_side")) == "FOLLOWING" {
		// omit the default window from window ranking functions
		e.Set("spec", nil)
	}
	return g.windowSQL(e)
}

func (g *Generator) snowflakeFilterSQL(e exp.Expression) string {
	// Snowflake doesn't support FILTER (WHERE cond), so we rewrite it into an
	// equivalent conditional aggregation, i.e. wrap the input values in an IFF
	agg := e.This()
	aggArg := agg.This()
	cond := e.Expr().This()

	if agg.Kind() == exp.KindWithinGroup {
		// Ordered-set aggregates take their input from the ORDER BY key, so the
		// condition has to wrap that instead of the aggregate's own argument
		if aggArg != nil && (aggArg.Kind() == exp.KindMode || aggArg.Kind() == exp.KindPercentileCont || aggArg.Kind() == exp.KindPercentileDisc) {
			for _, ordered := range agg.Expr().Expressions() {
				key := ordered.This()
				key.Replace(exp.If(exp.Args{"this": cond.Copy(), "true": key.Copy()}))
			}
			return g.gen(agg)
		}
		// Besides the percentile functions, these are the only functions Snowflake
		// accepts WITHIN GROUP for, so anything else can't be rewritten correctly
		if aggArg != nil && (aggArg.Kind() == exp.KindArrayAgg || aggArg.Kind() == exp.KindGroupConcat) {
			aggArg = aggArg.This()
		} else {
			g.unsupported("Unable to rewrite FILTER into the aggregate's arguments")
			return g.gen(agg)
		}
	}

	// `COUNT(*/t.*) FILTER (WHERE cond)` counts qualifying rows, but a star can't be an IFF
	// argument. Use its native COUNT_IF instead.
	if agg.Kind() == exp.KindCount && aggArg != nil && aggArg.IsStar() {
		return g.funcCall("COUNT_IF", []any{cond}, "(", ")", true)
	}

	// `DISTINCT` and `ORDER BY` are part of the aggregate's own argument list, so the
	// condition has to wrap the values underneath them rather than the whole clause.
	if aggArg != nil && aggArg.Kind() == exp.KindOrder {
		aggArg = aggArg.This()
	}
	targets := []exp.Expression{aggArg}
	if aggArg != nil && aggArg.Kind() == exp.KindDistinct {
		targets = aggArg.Expressions()
	}
	for _, target := range targets {
		if target != nil {
			target.Replace(exp.If(exp.Args{"this": cond.Copy(), "true": target.Copy()}))
		}
	}
	return g.gen(agg)
}

func (g *Generator) snowflakeWithinGroupSQL(e exp.Expression) string {
	// Snowflake's MODE doesn't support the ordered-set syntax, i.e. it only
	// accepts the value to aggregate as an argument: MODE(<expr>)
	if this := e.This(); this != nil && this.Kind() == exp.KindMode && this.This() == nil {
		if order := e.Expr(); order != nil && order.Kind() == exp.KindOrder && len(order.Expressions()) == 1 {
			return g.gen(exp.New(exp.KindMode, exp.Args{"this": order.Expressions()[0].This()}))
		}
	}
	return g.withinGroupSQL(e)
}

// snowflakeBuildDateTimeAdd ports _build_date_time_add (generators/snowflake.py:52-60).
func snowflakeBuildDateTimeAdd(kind exp.Kind, args []exp.Expression) exp.Expression {
	return exp.New(kind, exp.Args{"this": args[2], "expression": args[1], "unit": dialects.MapDatePart(args[0], nil)})
}

// snowflakeUnnestGenerateDateArray ports _unnest_generate_date_array (generators/snowflake.py:
// 113-171).
func snowflakeUnnestGenerateDateArray(unnest exp.Expression) {
	generateDateArray := unnest.Expressions()[0]
	start := asExpression(generateDateArray.Arg("start"))
	end := asExpression(generateDateArray.Arg("end"))
	step := asExpression(generateDateArray.Arg("step"))
	if start == nil || end == nil || step == nil || step.Kind() != exp.KindInterval || step.Name() != "1" {
		return
	}
	unit := asExpression(step.Arg("unit"))

	unnestAlias := asExpression(unnest.Arg("alias"))
	var sequenceValue exp.Expression = exp.Column(exp.Args{"this": exp.ToIdentifier("value")})
	sequenceValueName := "value"
	if unnestAlias != nil {
		unnestAlias = unnestAlias.Copy()
		if columns := exp.ExpressionsOf(unnestAlias, "columns"); len(columns) > 0 {
			sequenceValue = columns[0]
			sequenceValueName = columns[0].Name()
		}
	}

	// We'll add the next sequence value to the starting date and project the result
	dateAdd := snowflakeBuildDateTimeAdd(exp.KindDateAdd, []exp.Expression{unit, exp.CastTo(sequenceValue.Copy(), exp.DTypeInt), exp.CastTo(start.Copy(), exp.DTypeDate)})

	// We use DATEDIFF to compute the number of sequence values needed
	dateDiff := exp.New(exp.KindDateDiff, exp.Args{"this": end, "expression": start, "unit": dialects.MapDatePart(unit, nil), "date_part_boundary": true})
	numberSequence := dialects.SnowflakeArrayGenerateRange([]exp.Expression{exp.LiteralNumber(0), exp.BinOp(exp.KindAdd, dateDiff, 1)})
	unnest.Set("expressions", []exp.Expression{numberSequence})

	unnestParent := unnest.Parent()
	if unnestParent != nil && unnestParent.Kind() == exp.KindJoin {
		if sel := unnestParent.Parent(); sel != nil && sel.Kind() == exp.KindSelect {
			if scope := optimizer.BuildScope(sel); scope != nil {
				for _, column := range scope.Columns() {
					if strings.EqualFold(column.Name(), sequenceValueName) {
						if parent := column.Parent(); parent != nil && parent.Kind() == exp.KindSelect {
							column.Replace(exp.AliasNode(exp.Args{"this": dateAdd.Copy(), "alias": exp.ToIdentifier(sequenceValueName)}))
						} else {
							column.Replace(dateAdd)
						}
					}
				}
			}
			lateral := exp.New(exp.KindLateral, exp.Args{"this": unnestParent.This().Pop()})
			unnestParent.Replace(exp.Join(exp.Args{"this": lateral}))
		}
		return
	}
	subquery := exp.Args{"this": exp.Select(exp.Args{
		"expressions": []exp.Expression{exp.AliasNode(exp.Args{"this": dateAdd.Copy(), "alias": exp.ToIdentifier(sequenceValueName)})},
		"from_":       exp.New(exp.KindFrom, exp.Args{"this": unnest.Copy()}),
	})}
	if unnestAlias != nil {
		subquery["alias"] = unnestAlias
	}
	unnest.Replace(exp.Subquery(subquery))
}

// snowflakeTransformGenerateDateArray ports _transform_generate_date_array (generators/
// snowflake.py:174-194).
func snowflakeTransformGenerateDateArray(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	for _, generateDateArray := range e.FindAll(exp.KindGenerateDateArray) {
		parent := generateDateArray.Parent()
		// If GENERATE_DATE_ARRAY is used directly as an array (e.g passed into ARRAY_LENGTH), the
		// transformed Snowflake query is SELECT ref((SELECT ARRAY_AGG(*) FROM UNNEST(...)))
		if parent == nil || parent.Kind() != exp.KindUnnest {
			unnest := exp.New(exp.KindUnnest, exp.Args{"expressions": []exp.Expression{generateDateArray.Copy()}})
			generateDateArray.Replace(exp.Subquery(exp.Args{"this": exp.Select(exp.Args{
				"expressions": []exp.Expression{exp.New(exp.KindArrayAgg, exp.Args{"this": exp.Star(nil)})},
				"from_":       exp.New(exp.KindFrom, exp.Args{"this": unnest}),
			})}))
		}
		if parent != nil && parent.Kind() == exp.KindUnnest {
			if grand := parent.Parent(); grand != nil && (grand.Kind() == exp.KindFrom || grand.Kind() == exp.KindJoin) && len(parent.Expressions()) == 1 {
				snowflakeUnnestGenerateDateArray(parent)
			}
		}
	}
	return e
}

// snowflakeQualifyUnnestedColumns ports _qualify_unnested_columns (generators/snowflake.py:
// 240-320).
func snowflakeQualifyUnnestedColumns(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	scope := optimizer.BuildScope(e)
	if scope == nil {
		return e
	}
	unnests := scope.FindAll(exp.KindUnnest)
	if len(unnests) == 0 {
		return e
	}
	takenSourceNames := map[string]bool{}
	for name := range scope.Sources {
		takenSourceNames[name] = true
	}
	columnSource := map[string]exp.Expression{}
	unnestToIdentifier := map[exp.Expression]exp.Expression{}
	var unnestIdentifier exp.Expression
	orig := e.Copy()

	for _, unnest := range unnests {
		if parent := unnest.Parent(); parent == nil || (parent.Kind() != exp.KindFrom && parent.Kind() != exp.KindJoin) {
			continue
		}
		// Try to infer column names produced by an unnest operator. This is only possible
		// when we can peek into the (statically known) contents of the unnested value.
		unnestColumns := []string{}
		for _, unnestExpr := range unnest.Expressions() {
			if unnestExpr.Kind() != exp.KindArray {
				continue
			}
			for _, arrayExpr := range unnestExpr.Expressions() {
				if arrayExpr.Kind() != exp.KindStruct || len(arrayExpr.Expressions()) == 0 {
					continue
				}
				allPropertyEQ := true
				for _, structExpr := range arrayExpr.Expressions() {
					if structExpr.Kind() != exp.KindPropertyEQ {
						allPropertyEQ = false
						break
					}
				}
				if !allPropertyEQ {
					continue
				}
				for _, structExpr := range arrayExpr.Expressions() {
					unnestColumns = append(unnestColumns, strings.ToLower(structExpr.This().Name()))
				}
				break
			}
			if len(unnestColumns) > 0 {
				break
			}
		}

		unnestAlias := asExpression(unnest.Arg("alias"))
		if unnestAlias == nil {
			aliasName := findNewName(takenSourceNames, "value")
			takenSourceNames[aliasName] = true
			// Produce a `TableAlias` AST similar to what is produced for BigQuery. This
			// will be corrected later, when we generate SQL for the `Unnest` AST node.
			aliased := unnest.Copy()
			aliased.Set("alias", exp.TableAlias(exp.Args{"columns": []exp.Expression{exp.ToIdentifier(aliasName)}}))
			scope.Replace(unnest, aliased)
			unnestIdentifier = exp.ExpressionsOf(asExpression(aliased.Arg("alias")), "columns")[0]
		} else {
			unnestIdentifier = unnestAlias.This()
			if unnestIdentifier == nil {
				if columns := exp.ExpressionsOf(unnestAlias, "columns"); len(columns) > 0 {
					unnestIdentifier = columns[0]
				}
			}
		}
		if unnestIdentifier == nil || unnestIdentifier.Kind() != exp.KindIdentifier {
			return orig
		}
		unnestToIdentifier[unnest] = unnestIdentifier
		for _, c := range unnestColumns {
			columnSource[c] = unnestIdentifier
		}
	}

	for _, column := range scope.Columns() {
		if column.TableName() != "" {
			continue
		}
		table := columnSource[strings.ToLower(column.Name())]
		if unnestIdentifier != nil && table == nil && len(scope.Sources) == 1 && !strings.EqualFold(column.Name(), unnestIdentifier.Name()) {
			if ancestor := column.FindAncestor(exp.KindUnnest, exp.KindSelect); ancestor != nil && ancestor.Kind() == exp.KindUnnest {
				if ancestorIdentifier := unnestToIdentifier[ancestor]; ancestorIdentifier != nil && strings.EqualFold(ancestorIdentifier.Name(), unnestIdentifier.Name()) {
					continue
				}
			}
			table = unnestIdentifier
		}
		if table != nil {
			column.Set("table", table.Copy())
		} else {
			column.Set("table", nil)
		}
	}
	return e
}

// snowflakeEliminateDotVariantLookup ports _eliminate_dot_variant_lookup (generators/
// snowflake.py:323-356).
func snowflakeEliminateDotVariantLookup(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	// This transformation is used to facilitate transpilation of BigQuery `UNNEST` operations
	// to Snowflake. It should not affect roundtrip because `Unnest` nodes cannot be produced
	// by Snowflake's parser.
	unnestAliases := map[string]bool{}
	for _, unnest := range optimizer.FindAllInScope(e, exp.KindUnnest) {
		alias := asExpression(unnest.Arg("alias"))
		if alias != nil && alias.Kind() == exp.KindTableAlias && alias.This() == nil {
			if columns := exp.ExpressionsOf(alias, "columns"); len(columns) == 1 {
				unnestAliases[columns[0].Name()] = true
			}
		}
	}
	if len(unnestAliases) == 0 {
		return e
	}
	for _, c := range optimizer.FindAllInScope(e, exp.KindColumn) {
		if !unnestAliases[c.TableName()] {
			continue
		}
		bracket := exp.New(exp.KindBracket, exp.Args{"this": c.Arg("table"), "expressions": []exp.Expression{exp.LiteralString(c.Name())}})
		if c.Parent() == e {
			// Retain column projection names by using aliases
			c.Replace(exp.AliasNode(exp.Args{"this": bracket, "alias": c.This().Copy()}))
		} else {
			c.Replace(bracket)
		}
	}
	return e
}

func findNewName(taken map[string]bool, base string) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		name := base + "_" + strconv.Itoa(i)
		if !taken[name] {
			return name
		}
	}
}
