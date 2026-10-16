package generator

import (
	"strconv"
	"strings"
	"time"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
)

// generators/clickhouse.py: ClickHouseGenerator, a direct Generator subclass.
func init() {
	registerDialectTypeMapping("clickhouse", map[exp.DType]string{
		exp.DTypeBlob:                    "String",
		exp.DTypeChar:                    "String",
		exp.DTypeLongBlob:                "String",
		exp.DTypeLongText:                "String",
		exp.DTypeMediumBlob:              "String",
		exp.DTypeMediumText:              "String",
		exp.DTypeTinyBlob:                "String",
		exp.DTypeTinyText:                "String",
		exp.DTypeText:                    "String",
		exp.DTypeVarBinary:               "String",
		exp.DTypeVarchar:                 "String",
		exp.DTypeArray:                   "Array",
		exp.DTypeBoolean:                 "Bool",
		exp.DTypeBigInt:                  "Int64",
		exp.DTypeDate32:                  "Date32",
		exp.DTypeDatetime:                "DateTime",
		exp.DTypeDatetime2:               "DateTime",
		exp.DTypeSmallDatetime:           "DateTime",
		exp.DTypeDatetime64:              "DateTime64",
		exp.DTypeDecimal:                 "Decimal",
		exp.DTypeDecimal32:               "Decimal32",
		exp.DTypeDecimal64:               "Decimal64",
		exp.DTypeDecimal128:              "Decimal128",
		exp.DTypeDecimal256:              "Decimal256",
		exp.DTypeTimestamp:               "DateTime",
		exp.DTypeTimestampNtz:            "DateTime",
		exp.DTypeTimestampTz:             "DateTime",
		exp.DTypeDouble:                  "Float64",
		exp.DTypeEnum:                    "Enum",
		exp.DTypeEnum8:                   "Enum8",
		exp.DTypeEnum16:                  "Enum16",
		exp.DTypeFixedString:             "FixedString",
		exp.DTypeFloat:                   "Float32",
		exp.DTypeInt:                     "Int32",
		exp.DTypeMediumInt:               "Int32",
		exp.DTypeInt128:                  "Int128",
		exp.DTypeInt256:                  "Int256",
		exp.DTypeLowCardinality:          "LowCardinality",
		exp.DTypeMap:                     "Map",
		exp.DTypeNested:                  "Nested",
		exp.DTypeNothing:                 "Nothing",
		exp.DTypeSmallInt:                "Int16",
		exp.DTypeStruct:                  "Tuple",
		exp.DTypeTinyInt:                 "Int8",
		exp.DTypeUBigInt:                 "UInt64",
		exp.DTypeUInt:                    "UInt32",
		exp.DTypeUInt128:                 "UInt128",
		exp.DTypeUInt256:                 "UInt256",
		exp.DTypeUSmallInt:               "UInt16",
		exp.DTypeUTinyInt:                "UInt8",
		exp.DTypeIpv4:                    "IPv4",
		exp.DTypeIpv6:                    "IPv6",
		exp.DTypePoint:                   "Point",
		exp.DTypeRing:                    "Ring",
		exp.DTypeLineString:              "LineString",
		exp.DTypeMultiLineString:         "MultiLineString",
		exp.DTypePolygon:                 "Polygon",
		exp.DTypeMultiPolygon:            "MultiPolygon",
		exp.DTypeAggregateFunction:       "AggregateFunction",
		exp.DTypeSimpleAggregateFunction: "SimpleAggregateFunction",
		exp.DTypeDynamic:                 "Dynamic",
	})
	registerJSONPathParts("clickhouse", exp.KindJSONPathKey, exp.KindJSONPathRoot, exp.KindJSONPathSubscript)
	registerDialectPropertyLocations("clickhouse", map[exp.Kind]propertyLocation{
		exp.KindAutoRefreshProperty:   propertyLocationPostName,
		exp.KindDefinerProperty:       propertyLocationPostSchema,
		exp.KindOnCluster:             propertyLocationPostName,
		exp.KindPartitionedByProperty: propertyLocationPostSchema,
		exp.KindToTableProperty:       propertyLocationPostName,
		exp.KindUuidProperty:          propertyLocationPostName,
		exp.KindVolatileProperty:      propertyLocationUnsupported,
	})
	rename := prestoRename
	registerDialectDispatch("clickhouse", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindAnyValue:       rename("any"),
		exp.KindApproxDistinct: rename("uniq"),
		exp.KindArrayDistinct:  rename("arrayDistinct"),
		exp.KindArrayConcat:    rename("arrayConcat"),
		exp.KindArrayContains:  rename("has"),
		exp.KindArrayFilter: func(g *Generator, e exp.Expression) string {
			return g.funcCall("arrayFilter", []any{e.Expr(), e.This()}, "(", ")", true)
		},
		exp.KindTransform: func(g *Generator, e exp.Expression) string {
			return g.funcCall("arrayMap", []any{e.Expr(), e.This()}, "(", ")", true)
		},
		exp.KindArrayRemove:  (*Generator).removeFromArrayUsingFilterSQL,
		exp.KindArrayReverse: rename("arrayReverse"),
		exp.KindArraySlice:   rename("arraySlice"),
		exp.KindArraySum:     rename("arraySum"),
		exp.KindArrayMax:     rename("arrayMax"),
		exp.KindArrayMin:     rename("arrayMin"),
		exp.KindArgMax:       argMaxOrMinNoCount("argMax"),
		exp.KindArgMin:       argMaxOrMinNoCount("argMin"),
		exp.KindArray: func(g *Generator, e exp.Expression) string {
			return g.inlineArraySQL(e)
		},
		exp.KindCityHash64:      rename("cityHash64"),
		exp.KindCastToStrType:   rename("CAST"),
		exp.KindCurrentDatabase: rename("CURRENT_DATABASE"),
		exp.KindCurrentSchemas:  rename("CURRENT_SCHEMAS"),
		exp.KindCountIf:         rename("countIf"),
		exp.KindCosineDistance:  rename("cosineDistance"),
		exp.KindCompressColumnConstraint: func(g *Generator, e exp.Expression) string {
			return "CODEC(" + g.expressions(exprsOptions{expression: e, key: "this", flat: true}) + ")"
		},
		exp.KindComputedColumnConstraint: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("persisted")) {
				return "MATERIALIZED " + g.sqlKey(e, "this")
			}
			return "ALIAS " + g.sqlKey(e, "this")
		},
		exp.KindCurrentDate: func(g *Generator, _ exp.Expression) string {
			return g.funcCall("CURRENT_DATE", nil, "(", ")", true)
		},
		exp.KindCurrentVersion:  rename("VERSION"),
		exp.KindDateAdd:         clickHouseDatetimeDeltaSQL("DATE_ADD"),
		exp.KindDateDiff:        clickHouseDatetimeDeltaSQL("DATE_DIFF"),
		exp.KindDateStrToDate:   rename("toDate"),
		exp.KindDateSub:         clickHouseDatetimeDeltaSQL("DATE_SUB"),
		exp.KindExplode:         rename("arrayJoin"),
		exp.KindFarmFingerprint: rename("farmFingerprint64"),
		exp.KindFinal: func(g *Generator, e exp.Expression) string {
			return g.sqlKey(e, "this") + " FINAL"
		},
		exp.KindIsNan: rename("isNaN"),
		exp.KindJarowinklerSimilarity: func(g *Generator, e exp.Expression) string {
			this, expr := any(e.This()), any(e.Expr())
			if truthy(e.Arg("case_insensitive")) {
				this = exp.New(exp.KindUpper, exp.Args{"this": e.This()})
				expr = exp.New(exp.KindUpper, exp.Args{"this": e.Expr()})
			}
			return g.funcCall("jaroWinklerSimilarity", []any{this, expr}, "(", ")", true)
		},
		exp.KindJSONCast: (*Generator).clickHouseJSONCastSQL,
		exp.KindJSONExtract: func(g *Generator, e exp.Expression) string {
			return g.jsonExtractSegments(e, "JSONExtractString", false, "")
		},
		exp.KindJSONExtractScalar: func(g *Generator, e exp.Expression) string {
			return g.jsonExtractSegments(e, "JSONExtractString", false, "")
		},
		exp.KindJSONPathKey:  (*Generator).jsonPathKeyOnlyName,
		exp.KindJSONPathRoot: func(*Generator, exp.Expression) string { return "" },
		exp.KindJSONPathSubscript: func(g *Generator, e exp.Expression) string {
			// _jsonpathsubscript_sql (generators/clickhouse.py:470-472): 1-based indices.
			this := g.jsonPathPart(e.Arg("this"))
			if n, err := strconv.Atoi(this); err == nil {
				return strconv.Itoa(n + 1)
			}
			return this
		},
		exp.KindLength: func(g *Generator, e exp.Expression) string {
			name := "CHAR_LENGTH"
			if truthy(e.Arg("binary")) {
				name = "LENGTH"
			}
			return g.funcCall(name, []any{e.This()}, "(", ")", true)
		},
		exp.KindMap:    (*Generator).clickHouseMapSQL,
		exp.KindMedian: rename("median"),
		exp.KindNullif: rename("nullIf"),
		exp.KindPartitionedByProperty: func(g *Generator, e exp.Expression) string {
			return "PARTITION BY " + g.sqlKey(e, "this")
		},
		exp.KindPivot: func(g *Generator, _ exp.Expression) string {
			g.unsupported("PIVOT unsupported")
			return ""
		},
		exp.KindQuantile: (*Generator).clickHouseQuantileSQL,
		exp.KindRegexpLike: func(g *Generator, e exp.Expression) string {
			return g.funcCall("match", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		exp.KindRand:              rename("randCanonical"),
		exp.KindStartsWith:        rename("startsWith"),
		exp.KindStruct:            rename("tuple"),
		exp.KindTrunc:             rename("trunc"),
		exp.KindEndsWith:          rename("endsWith"),
		exp.KindEuclideanDistance: rename("L2Distance"),
		exp.KindStrPosition: func(g *Generator, e exp.Expression) string {
			return g.strPositionSQLFull(e, "POSITION", true, false, false)
		},
		exp.KindTimeToStr: func(g *Generator, e exp.Expression) string {
			this := e.This()
			if this != nil && this.Kind() == exp.KindTsOrDsToTimestamp {
				this = this.This()
			}
			return g.funcCall("formatDateTime", []any{this, g.formatTime(e), e.Arg("zone")}, "(", ")", true)
		},
		exp.KindTimeStrToTime: (*Generator).clickHouseTimeStrToTimeSQL,
		exp.KindTimestampAdd:  clickHouseDatetimeDeltaSQL("TIMESTAMP_ADD"),
		exp.KindTimestampSub:  clickHouseDatetimeDeltaSQL("TIMESTAMP_SUB"),
		exp.KindTypeof:        rename("toTypeName"),
		exp.KindVarMap:        (*Generator).clickHouseMapSQL,
		exp.KindXor: func(g *Generator, e exp.Expression) string {
			return g.funcCall("xor", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		exp.KindMD5Digest: rename("MD5"),
		exp.KindMD5: func(g *Generator, e exp.Expression) string {
			return g.funcCall("LOWER", []any{g.funcCall("HEX", []any{g.funcCall("MD5", []any{e.This()}, "(", ")", true)}, "(", ")", true)}, "(", ")", true)
		},
		exp.KindSHA:        rename("SHA1"),
		exp.KindSHA1Digest: rename("SHA1"),
		exp.KindSHA2:       sha256SQL,
		exp.KindSHA2Digest: sha256SQL,
		exp.KindSplit: func(g *Generator, e exp.Expression) string {
			return g.funcCall("splitByString", []any{e.Arg("expression"), e.This(), e.Arg("limit")}, "(", ")", true)
		},
		exp.KindRegexpSplit: func(g *Generator, e exp.Expression) string {
			return g.funcCall("splitByRegexp", []any{e.Arg("expression"), e.This(), e.Arg("limit")}, "(", ")", true)
		},
		exp.KindUnixToTime: (*Generator).clickHouseUnixToTimeSQL,
		exp.KindTrim: func(g *Generator, e exp.Expression) string {
			return g.trimSQLWithDefault(e, "BOTH")
		},
		exp.KindVariance: rename("varSamp"),
		exp.KindSchemaCommentProperty: func(g *Generator, e exp.Expression) string {
			return g.nakedPropertySQL(e)
		},
		exp.KindStddev: rename("stddevSamp"),
		exp.KindChr:    rename("CHAR"),
		exp.KindLag: func(g *Generator, e exp.Expression) string {
			return g.funcCall("lagInFrame", []any{e.This(), e.Arg("offset"), e.Arg("default")}, "(", ")", true)
		},
		exp.KindLead: func(g *Generator, e exp.Expression) string {
			return g.funcCall("leadInFrame", []any{e.This(), e.Arg("offset"), e.Arg("default")}, "(", ")", true)
		},
		exp.KindLevenshtein: func(g *Generator, e exp.Expression) string {
			g.unsupportedArgs(e, "ins_cost", "del_cost", "sub_cost", "max_dist")
			return g.funcCall("editDistance", g.fallbackArgs(e), "(", ")", true)
		},
		exp.KindParseDatetime: func(g *Generator, e exp.Expression) string {
			return g.funcCall("parseDateTime", []any{e.This(), e.Arg("format"), e.Arg("zone")}, "(", ")", true)
		},

		// Methods of ClickHouseGenerator.
		exp.KindGroupConcat: (*Generator).clickHouseGroupConcatSQL,
		exp.KindOffset:      (*Generator).clickHouseOffsetSQL,
		exp.KindStrToDate:   (*Generator).clickHouseStrToDateSQL,
		exp.KindCast:        (*Generator).clickHouseCastSQL,
		exp.KindTryCast:     (*Generator).clickHouseTryCastSQL,
		exp.KindLikeProperty: func(g *Generator, e exp.Expression) string {
			return "AS " + g.sqlKey(e, "this")
		},
		exp.KindEQ: func(g *Generator, e exp.Expression) string {
			return g.clickHouseAnyToHas(e, (*Generator).eqSQL, "")
		},
		exp.KindNEQ: func(g *Generator, e exp.Expression) string {
			return g.clickHouseAnyToHas(e, (*Generator).neqSQL, "NOT ")
		},
		exp.KindRegexpILike: func(g *Generator, e exp.Expression) string {
			regex := g.funcCall("CONCAT", []any{"'(?i)'", e.Expr()}, "(", ")", true)
			return g.funcCall("match", []any{e.This(), regex}, "(", ")", true)
		},
		exp.KindDataType: (*Generator).clickHouseDataTypeSQL,
		exp.KindCTE: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("scalar")) {
				return g.sqlKey(e, "this") + " AS " + g.sqlKey(e, "alias")
			}
			return g.cteSQL(e)
		},
		exp.KindPlaceholder: func(g *Generator, e exp.Expression) string {
			return "{" + e.Name() + ": " + g.sqlKey(e, "kind") + "}"
		},
		exp.KindOnCluster: func(g *Generator, e exp.Expression) string {
			return "ON CLUSTER " + g.sqlKey(e, "this")
		},
		exp.KindAutoRefreshProperty: (*Generator).clickHouseAutoRefreshPropertySQL,
		exp.KindCreate:              (*Generator).clickHouseCreateSQL,
		exp.KindPreWhere: func(g *Generator, e exp.Expression) string {
			return g.seg("PREWHERE") + g.sep() + g.indent(g.sqlKey(e, "this"), 0, nil, false, false)
		},
		exp.KindIndexColumnConstraint: (*Generator).clickHouseIndexColumnConstraintSQL,
		exp.KindPartition: func(g *Generator, e exp.Expression) string {
			return "PARTITION " + g.expressions(exprsOptions{expression: e, flat: true})
		},
		exp.KindPartitionId: func(g *Generator, e exp.Expression) string {
			return "ID " + g.gen(e.This())
		},
		exp.KindReplacePartition: func(g *Generator, e exp.Expression) string {
			return "REPLACE " + g.gen(e.Expr()) + " FROM " + g.sqlKey(e, "source")
		},
		exp.KindProjectionDef: func(g *Generator, e exp.Expression) string {
			return "PROJECTION " + g.gen(e.This()) + " " + g.wrap(e.Expr())
		},
		exp.KindNestedJSONSelect: func(g *Generator, e exp.Expression) string {
			return g.sqlKey(e, "this") + ".^" + g.sqlKey(e, "expression")
		},
		exp.KindIs: func(g *Generator, e exp.Expression) string {
			isSQL := g.isSQL(e)
			if parent := e.Parent(); parent != nil && parent.Kind() == exp.KindNot {
				// value IS NOT NULL -> NOT (value IS NULL)
				isSQL = g.wrap(isSQL)
			}
			return isSQL
		},
		exp.KindIn: func(g *Generator, e exp.Expression) string {
			inSQL := g.inSQL(e)
			if parent := e.Parent(); parent != nil && parent.Kind() == exp.KindNot && truthy(e.Arg("is_global")) {
				inSQL = strings.Replace(inSQL, "GLOBAL IN", "GLOBAL NOT IN", 1)
			}
			return inSQL
		},
		exp.KindNot: func(g *Generator, e exp.Expression) string {
			if this := e.This(); this != nil && this.Kind() == exp.KindIn {
				if truthy(this.Arg("is_global")) {
					// let `GLOBAL IN` child interpose `NOT`
					return g.sqlKey(e, "this")
				}
				e.Set("this", exp.Paren(exp.Args{"this": this}))
			}
			return g.notSQL(e)
		},
		exp.KindValues: func(g *Generator, e exp.Expression) string {
			// If the VALUES clause contains tuples of expressions, we need to treat it
			// as a table since Clickhouse will automatically alias it as such.
			valuesAsTable := true
			if alias := asExpression(e.Arg("alias")); alias != nil && truthy(alias.Arg("columns")) && len(e.Expressions()) > 0 {
				valuesAsTable = false
				for _, value := range e.Expressions()[0].Expressions() {
					if value.Kind() == exp.KindTuple {
						valuesAsTable = true
						break
					}
				}
			}
			return g.valuesSQLWith(e, valuesAsTable)
		},
		exp.KindTimestampTrunc: (*Generator).clickHouseTimestampTruncSQL,
		exp.KindDateTrunc:      (*Generator).clickHouseTimestampTruncSQL,
	})
}

// argMaxOrMinNoCount ports arg_max_or_min_no_count (dialect.py:1976-1981).
func argMaxOrMinNoCount(name string) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		g.unsupportedArgs(e, "count")
		return g.funcCall(name, []any{e.This(), e.Expr()}, "(", ")", true)
	}
}

// sha256SQL ports sha256_sql / sha2_digest_sql (dialect.py:2388-2393).
func sha256SQL(g *Generator, e exp.Expression) string {
	length := e.Text("length")
	if length == "" {
		length = "256"
	}
	return g.funcCall("SHA"+length, []any{e.This()}, "(", ")", true)
}

// removeFromArrayUsingFilterSQL ports remove_from_array_using_filter (dialect.py:2319-2335).
func (g *Generator) removeFromArrayUsingFilterSQL(e exp.Expression) string {
	lambdaID := exp.ToIdentifier("_u")
	cond := exp.NEQ(exp.Args{"this": lambdaID, "expression": e.Expr()})
	filterSQL := g.gen(exp.New(exp.KindArrayFilter, exp.Args{
		"this":       e.This(),
		"expression": exp.New(exp.KindLambda, exp.Args{"this": cond, "expressions": []exp.Expression{lambdaID.Copy()}}),
	}))
	if truthy(e.Arg("null_propagation")) {
		return g.gen(exp.Case(exp.Args{
			"ifs": []exp.Expression{exp.If(exp.Args{
				"this": exp.Is(exp.Args{"this": e.Expr().Copy(), "expression": exp.Null()}),
				"true": exp.Null(),
			})},
			"default": filterSQL,
		}))
	}
	return filterSQL
}

// clickHouseDatetimeDeltaSQL ports _datetime_delta_sql (generators/clickhouse.py:65-78).
func clickHouseDatetimeDeltaSQL(name string) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		if e.Arg("unit") == nil {
			return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
		}
		return g.funcCall(name, []any{unitToVar(e, "DAY"), e.Expr(), e.This(), e.Arg("zone")}, "(", ")", true)
	}
}

// unitToVar ports unit_to_var (dialect.py:2086-2093).
func unitToVar(e exp.Expression, defaultUnit string) exp.Expression {
	unit := asExpression(e.Arg("unit"))
	if unit != nil {
		switch unit.Kind() {
		case exp.KindVar, exp.KindPlaceholder, exp.KindWeekStart, exp.KindColumn:
			return unit
		}
	}
	value := defaultUnit
	if unit != nil {
		value = unit.Name()
	}
	if value == "" {
		return nil
	}
	return exp.Var(exp.Args{"this": value})
}

// clickHouseQuantileSQL ports _quantile_sql (generators/clickhouse.py:55-63).
func (g *Generator) clickHouseQuantileSQL(e exp.Expression) string {
	quantile := asExpression(e.Arg("quantile"))
	args := "(" + g.sqlKey(e, "this") + ")"
	var fn string
	if quantile != nil && quantile.Kind() == exp.KindArray {
		items := []any{}
		for _, q := range quantile.Expressions() {
			items = append(items, q)
		}
		fn = g.funcCall("quantiles", items, "(", ")", true)
	} else {
		fn = g.funcCall("quantile", []any{quantile}, "(", ")", true)
	}
	return fn + args
}

// clickHouseUnixToTimeSQL ports _unix_to_time_sql (generators/clickhouse.py:31-47).
func (g *Generator) clickHouseUnixToTimeSQL(e exp.Expression) string {
	scale := asExpression(e.Arg("scale"))
	timestamp := e.This()
	bigint := func(v exp.Expression) exp.Expression {
		return exp.Cast(exp.Args{"this": v, "to": exp.DTypeBigInt.IntoExpr(nil)})
	}
	scaleName := ""
	if scale != nil {
		scaleName = scale.Name()
	}
	switch {
	case scale == nil || scaleName == "0":
		return g.funcCall("fromUnixTimestamp", []any{bigint(timestamp)}, "(", ")", true)
	case scaleName == "3":
		return g.funcCall("fromUnixTimestamp64Milli", []any{bigint(timestamp)}, "(", ")", true)
	case scaleName == "6":
		return g.funcCall("fromUnixTimestamp64Micro", []any{bigint(timestamp)}, "(", ")", true)
	case scaleName == "9":
		return g.funcCall("fromUnixTimestamp64Nano", []any{bigint(timestamp)}, "(", ")", true)
	}
	pow := exp.Anonymous(exp.Args{"this": "POW", "expressions": []exp.Expression{exp.LiteralNumber(10), scale}})
	return g.funcCall("fromUnixTimestamp", []any{bigint(exp.Div(exp.Args{"this": timestamp, "expression": pow}))}, "(", ")", true)
}

// clickHouseTimeStrToTimeSQL ports _timestrtotime_sql (generators/clickhouse.py:81-133).
func (g *Generator) clickHouseTimeStrToTimeSQL(e exp.Expression) string {
	ts := e.This()
	tz := asExpression(e.Arg("zone"))
	if tz != nil && ts != nil && ts.Kind() == exp.KindLiteral {
		// Clickhouse will not accept timestamps that include a UTC offset, so we must remove
		// them; the timezone is encoded as a data type parameter instead.
		if parsed, ok := parseISOTimestamp(strings.TrimSpace(ts.Name())); ok {
			ts = exp.LiteralString(parsed)
		}
	}
	params := []exp.Expression{exp.New(exp.KindDataTypeParam, exp.Args{"this": exp.LiteralNumber(6)})}
	if tz != nil {
		params = append(params, exp.New(exp.KindDataTypeParam, exp.Args{"this": tz}))
	}
	// Non-nullable DateTime64 with microsecond precision
	datatype := exp.DTypeDatetime64.IntoExpr(exp.Args{"expressions": params, "nullable": false})
	return g.gen(exp.Cast(exp.Args{"this": ts, "to": datatype}))
}

// parseISOTimestamp is datetime.fromisoformat(...).replace(tzinfo=None).isoformat(sep=" ")
// for the timestamp shapes _timestrtotime_sql handles: the result drops any UTC offset and
// keeps microseconds only when they are non-zero.
func parseISOTimestamp(s string) (string, bool) {
	layouts := []string{
		"2006-01-02 15:04:05.999999999Z07:00", "2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05Z07:00", "2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05", "2006-01-02T15:04:05",
		"2006-01-02 15:04Z07:00", "2006-01-02 15:04", "2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			out := t.Format("2006-01-02 15:04:05")
			if micros := t.Nanosecond() / 1000; micros != 0 {
				out += "." + strconv.FormatInt(int64(1000000+micros), 10)[1:]
			}
			return out, true
		}
	}
	return "", false
}

// clickHouseMapSQL ports _map_sql (generators/clickhouse.py:136-155).
func (g *Generator) clickHouseMapSQL(e exp.Expression) string {
	if parent := e.Parent(); parent == nil || parent.ArgKey() != "settings" {
		return lowerFunc(g.hiveVarMapSQL(e))
	}
	keys, values := asExpression(e.Arg("keys")), asExpression(e.Arg("values"))
	if keys == nil || values == nil || keys.Kind() != exp.KindArray || values.Kind() != exp.KindArray {
		g.unsupported("Cannot convert array columns into map.")
		return ""
	}
	ks, vs := keys.Expressions(), values.Expressions()
	args := []string{}
	for i := 0; i < len(ks) && i < len(vs); i++ {
		args = append(args, g.gen(ks[i])+": "+g.gen(vs[i]))
	}
	return "{" + strings.Join(args, ", ") + "}"
}

// lowerFunc ports _lower_func (generators/clickhouse.py:50-52).
func lowerFunc(sql string) string {
	index := strings.Index(sql, "(")
	if index < 0 {
		return sql
	}
	return strings.ToLower(sql[:index]) + sql[index:]
}

// clickHouseJSONCastSQL ports _json_cast_sql (generators/clickhouse.py:158-166).
func (g *Generator) clickHouseJSONCastSQL(e exp.Expression) string {
	this := g.sqlKey(e, "this")
	to := asExpression(e.Arg("to"))
	toSQL := g.gen(to)
	if to != nil && len(to.Expressions()) > 0 {
		toSQL = g.gen(exp.ToIdentifier(toSQL))
	}
	return this + ".:" + toSQL
}

// clickHouseGroupConcatSQL ports groupconcat_sql (generators/clickhouse.py:398-424).
func (g *Generator) clickHouseGroupConcatSQL(e exp.Expression) string {
	this := e.This()
	separator := asExpression(e.Arg("separator"))
	if this != nil && this.Kind() == exp.KindLimit && this.This() != nil {
		limit := this
		this = limit.This()
		limit.Set("this", nil)
		return g.gen(exp.New(exp.KindParameterizedAgg, exp.Args{
			"this":        "groupConcat",
			"params":      []exp.Expression{this},
			"expressions": []exp.Expression{separator, limit.Expr()},
		}))
	}
	if separator != nil {
		return g.gen(exp.New(exp.KindParameterizedAgg, exp.Args{
			"this":        "groupConcat",
			"params":      []exp.Expression{this},
			"expressions": []exp.Expression{separator},
		}))
	}
	return g.funcCall("groupConcat", []any{this}, "(", ")", true)
}

// clickHouseOffsetSQL ports offset_sql (generators/clickhouse.py:426-435): OFFSET ... FETCH
// syntax requires a "ROW" or "ROWS" keyword.
func (g *Generator) clickHouseOffsetSQL(e exp.Expression) string {
	offset := g.offsetSQL(e)
	if parent := e.Parent(); parent != nil && parent.Kind() == exp.KindSelect {
		if limit := asExpression(parent.Arg("limit")); limit != nil && limit.Kind() == exp.KindFetch {
			offset += " ROWS"
		}
	}
	return offset
}

// clickHouseStrToDateSQL ports strtodate_sql (generators/clickhouse.py:437-445).
func (g *Generator) clickHouseStrToDateSQL(e exp.Expression) string {
	strToDateSQL := g.functionFallbackSQL(e)
	if parent := e.Parent(); parent == nil || parent.Kind() != exp.KindCast {
		// StrToDate returns DATEs in other dialects (eg. postgres), so this branch aims to
		// improve the transpilation to clickhouse
		return g.castSQL(exp.Cast(exp.Args{"this": e, "to": exp.DTypeDate.IntoExpr(nil)}))
	}
	return strToDateSQL
}

// clickHouseCastSQL ports cast_sql (generators/clickhouse.py:447-453).
func (g *Generator) clickHouseCastSQL(e exp.Expression) string {
	this := e.This()
	if this != nil && this.Kind() == exp.KindStrToDate {
		if to := asExpression(e.Arg("to")); to != nil && to.Equal(exp.DTypeDatetime.IntoExpr(nil)) {
			return g.gen(this)
		}
	}
	return g.castSQL(e)
}

// clickHouseNonNullableTypes ports NON_NULLABLE_TYPES (generators/clickhouse.py:382-393).
var clickHouseNonNullableTypes = []exp.DType{
	exp.DTypeArray, exp.DTypeMap, exp.DTypeStruct, exp.DTypePoint, exp.DTypeRing,
	exp.DTypeLineString, exp.DTypeMultiLineString, exp.DTypePolygon, exp.DTypeMultiPolygon,
}

// isTypeCheckNullable is DataType.is_type(dtype, check_nullable=True) (datatypes.py) for a
// bare DType: a nullable-marked type never matches the unmarked one.
func isTypeCheckNullable(e exp.Expression, t exp.DType) bool {
	return exp.IsType(e, t) && !truthy(e.Arg("nullable"))
}

func clickHouseIsNonNullableType(dtype exp.Expression) bool {
	for _, t := range clickHouseNonNullableTypes {
		if isTypeCheckNullable(dtype, t) {
			return true
		}
	}
	return false
}

// clickHouseTryCastSQL ports trycast_sql (generators/clickhouse.py:455-461).
func (g *Generator) clickHouseTryCastSQL(e exp.Expression) string {
	if dtype := asExpression(e.Arg("to")); dtype != nil && !clickHouseIsNonNullableType(dtype) {
		// Casting x into Nullable(T) appears to behave similarly to TRY_CAST(x AS T)
		dtype.Set("nullable", true)
	}
	return g.castSQL(e)
}

// clickHouseAnyToHas ports _any_to_has (generators/clickhouse.py:477-493).
func (g *Generator) clickHouseAnyToHas(e exp.Expression, fallback func(*Generator, exp.Expression) string, prefix string) string {
	left, right := e.This(), e.Expr()
	var arr, this exp.Expression
	switch {
	case left != nil && left.Kind() == exp.KindAny:
		arr, this = left, right
	case right != nil && right.Kind() == exp.KindAny:
		arr, this = right, left
	default:
		return fallback(g, e)
	}
	inner := arr.This()
	for inner != nil && inner.Kind() == exp.KindParen {
		inner = inner.This()
	}
	return prefix + g.funcCall("has", []any{inner, this}, "(", ")", true)
}

// clickHouseDataTypeSQL ports datatype_sql (generators/clickhouse.py:503-532).
func (g *Generator) clickHouseDataTypeSQL(e exp.Expression) string {
	// String is the standard ClickHouse type, every other variant is just an alias.
	// Additionally, any supplied length parameter will be ignored.
	var dtype string
	if t, ok := e.Arg("this").(exp.DType); ok && clickHouseStringTypes[t] {
		dtype = "String"
	} else {
		dtype = g.dataTypeSQL(e)
	}
	// Wrap in Nullable(...) when the type is marked nullable, or unmarked and neither the key
	// type of a Map nor a composite type.
	parent := e.Parent()
	nullable, isSet := e.Arg("nullable").(bool)
	isMapKey := parent != nil && parent.Kind() == exp.KindDataType && isTypeCheckNullable(parent, exp.DTypeMap) && e.Index() <= 0
	if (isSet && nullable) || (!isSet && !isMapKey && !clickHouseIsNonNullableType(e)) {
		dtype = "Nullable(" + dtype + ")"
	}
	return dtype
}

// clickHouseStringTypes ports STRING_TYPE_MAPPING's keys (generators/clickhouse.py:189-201).
var clickHouseStringTypes = map[exp.DType]bool{
	exp.DTypeBlob: true, exp.DTypeChar: true, exp.DTypeLongBlob: true, exp.DTypeLongText: true,
	exp.DTypeMediumBlob: true, exp.DTypeMediumText: true, exp.DTypeTinyBlob: true,
	exp.DTypeTinyText: true, exp.DTypeText: true, exp.DTypeVarBinary: true, exp.DTypeVarchar: true,
}

// clickHouseAutoRefreshPropertySQL ports autorefreshproperty_sql (generators/clickhouse.py:566-582).
func (g *Generator) clickHouseAutoRefreshPropertySQL(e exp.Expression) string {
	cadence := g.sqlKey(e, "cadence")
	interval := asExpression(e.Arg("this"))
	schedule := ""
	if cadence != "" && interval != nil {
		schedule = " " + cadence + " " + g.clickHouseRefreshIntervalSQL(interval)
	}
	offset := ""
	if o := asExpression(e.Arg("offset")); o != nil {
		offset = " OFFSET " + g.clickHouseRefreshIntervalSQL(o)
	}
	randomize := ""
	if r := asExpression(e.Arg("randomize")); r != nil {
		randomize = " RANDOMIZE FOR " + g.clickHouseRefreshIntervalSQL(r)
	}
	dependencies := g.expressions(exprsOptions{expression: e, flat: true})
	if dependencies != "" {
		dependencies = " DEPENDS ON " + dependencies
	}
	settings := g.sqlKey(e, "settings")
	if settings != "" {
		settings = " " + settings
	}
	appendSQL := ""
	if truthy(e.Arg("append")) {
		appendSQL = " APPEND"
	}
	return "REFRESH" + schedule + offset + randomize + dependencies + settings + appendSQL
}

// clickHouseRefreshIntervalSQL ports _refresh_interval_sql (generators/clickhouse.py:561-564).
func (g *Generator) clickHouseRefreshIntervalSQL(e exp.Expression) string {
	if e.Kind() == exp.KindAdd {
		return g.clickHouseRefreshIntervalSQL(e.This()) + " " + g.clickHouseRefreshIntervalSQL(e.Expr())
	}
	if e.Kind() != exp.KindInterval {
		panic(&exp.ArgError{Msg: "Expected Interval, got " + exp.ClassName(e.Kind())})
	}
	return g.gen(e)
}

// clickHouseOnClusterTargets ports ON_CLUSTER_TARGETS (generators/clickhouse.py:369-379).
var clickHouseOnClusterTargets = map[string]bool{
	"SCHEMA": true, "DATABASE": true, "TABLE": true, "VIEW": true, "DICTIONARY": true,
	"INDEX": true, "FUNCTION": true, "NAMED COLLECTION": true,
}

// clickHouseCreateableSQL ports createable_sql (generators/clickhouse.py:584-600).
func (g *Generator) clickHouseCreateableSQL(e exp.Expression, locations map[propertyLocation][]exp.Expression) string {
	if clickHouseOnClusterTargets[e.Text("kind")] && len(locations[propertyLocationPostName]) > 0 {
		this := e.This()
		var thisName string
		if this != nil && this.Kind() == exp.KindSchema {
			thisName = g.sqlKey(this, "this")
		} else {
			thisName = g.sqlKey(e, "this")
		}
		props := []string{}
		for _, prop := range locations[propertyLocationPostName] {
			props = append(props, g.gen(prop))
		}
		thisSchema := ""
		if this != nil {
			thisSchema = g.schemaColumnsSQL(this)
		}
		if thisSchema != "" {
			thisSchema = g.sep() + thisSchema
		}
		return thisName + g.sep() + strings.Join(props, " ") + thisSchema
	}
	return g.sqlKey(e, "this")
}

// clickHouseCreateSQL ports create_sql (generators/clickhouse.py:602-618): the comment
// property comes last in CTAS statements, i.e. after the query.
func (g *Generator) clickHouseCreateSQL(e exp.Expression) string {
	query := e.Expr()
	var commentProp exp.Expression
	if query != nil && query.Is(exp.TraitQuery) {
		if commentProp = e.Find(exp.KindSchemaCommentProperty); commentProp != nil {
			commentProp.Pop()
			query.Replace(exp.Paren(exp.Args{"this": query.Copy()}))
		}
	}
	createSQL := g.createSQL(e)
	commentSQL := g.gen(commentProp)
	if commentSQL != "" {
		commentSQL = " " + commentSQL
	}
	return createSQL + commentSQL
}

// clickHouseIndexColumnConstraintSQL ports indexcolumnconstraint_sql (generators/clickhouse.py:624-635).
func (g *Generator) clickHouseIndexColumnConstraintSQL(e exp.Expression) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	expr := g.sqlKey(e, "expression")
	if expr != "" {
		expr = " " + expr
	}
	indexType := g.sqlKey(e, "index_type")
	if indexType != "" {
		indexType = " TYPE " + indexType
	}
	granularity := g.sqlKey(e, "granularity")
	if granularity != "" {
		granularity = " GRANULARITY " + granularity
	}
	return "INDEX" + this + expr + indexType + granularity
}

// clickHouseTimestampTruncSQL ports timestamptrunc_sql (generators/clickhouse.py:711-718).
func (g *Generator) clickHouseTimestampTruncSQL(e exp.Expression) string {
	unit := weekstartUnitToStr(e)
	// https://clickhouse.com/docs/whats-new/changelog/2023#improvement
	if g.dialect.VersionLess(23, 12) && unit != nil && unit.IsString() {
		unit = exp.LiteralString(strings.ToLower(unit.Name()))
	}
	return g.funcCall("dateTrunc", []any{unit, e.This(), e.Arg("zone")}, "(", ")", true)
}
