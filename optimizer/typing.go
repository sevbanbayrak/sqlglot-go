package optimizer

import (
	"sync"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
)

// ExprMetadataSpec ports one EXPRESSION_METADATA entry (typing/__init__.py): an annotator, or
// the type the expression returns (an exp.DType or a DataType expression).
type ExprMetadataSpec struct {
	Annotator func(a *TypeAnnotator, e exp.Expression) exp.Expression
	Returns   any
}

// ExprMetadata ports ExprMetadataType: expression Kind -> annotation spec. Upstream keys it by
// class; the Go node model keys it by Kind.
type ExprMetadata map[exp.Kind]ExprMetadataSpec

func (m ExprMetadata) Copy() ExprMetadata {
	out := make(ExprMetadata, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (m ExprMetadata) returns(t any, kinds ...exp.Kind) {
	for _, k := range kinds {
		m[k] = ExprMetadataSpec{Returns: t}
	}
}

func (m ExprMetadata) annotate(f func(a *TypeAnnotator, e exp.Expression) exp.Expression, kinds ...exp.Kind) {
	for _, k := range kinds {
		m[k] = ExprMetadataSpec{Annotator: f}
	}
}

// byArgs is `lambda self, e: self._annotate_by_args(e, *args)`.
func byArgs(args ...any) func(a *TypeAnnotator, e exp.Expression) exp.Expression {
	return func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.AnnotateByArgs(e, args, false, false)
	}
}

// setTypeFromStr is `lambda self, e: self._set_type(e, exp.DataType.from_str(dtype, dialect=dialect))`.
func setTypeFromStr(dtype string, dialect string) func(a *TypeAnnotator, e exp.Expression) exp.Expression {
	return func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.SetType(e, dataTypeFromStr(dtype, dialect))
	}
}

func dataTypeFromStr(dtype string, dialect string) exp.Expression {
	t, err := exp.DataTypeFromStr(dtype, dialect, false, nil)
	if err != nil {
		panic(err)
	}
	return t
}

// subclasses is helper.subclasses(exp.__name__, ...) over the Go traits and extra Kinds.
func subclasses(trait exp.Trait, extra ...exp.Kind) []exp.Kind {
	out := append([]exp.Kind{}, extra...)
	for _, k := range exp.AllKinds() {
		if exp.New(k, nil).Is(trait) {
			out = append(out, k)
		}
	}
	return out
}

// TIMESTAMP_EXPRESSIONS (typing/__init__.py:9-16).
var timestampExpressions = []exp.Kind{
	exp.KindCurrentTimestamp,
	exp.KindStrToTime,
	exp.KindTimeStrToTime,
	exp.KindTimestampAdd,
	exp.KindTimestampSub,
	exp.KindUnixToTime,
}

// baseExpressionMetadata ports typing/__init__.py EXPRESSION_METADATA, entry by entry in the
// dict's order (a later entry overrides an earlier one, as in the dict literal).
func baseExpressionMetadata() ExprMetadata {
	m := ExprMetadata{}
	// Pow subclasses Binary upstream (math.py) but carries no Binary trait in the Go node model.
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.AnnotateBinary(e) },
		subclasses(exp.TraitBinary, exp.KindPow)...)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateUnary(e) },
		subclasses(exp.TraitUnary, exp.KindAlias, exp.KindPivotAlias, exp.KindIgnoreNulls, exp.KindRespectNulls)...)
	m.returns(exp.DTypeBigInt,
		exp.KindApproxDistinct,
		exp.KindArraySize,
		exp.KindCountIf,
		exp.KindDenseRank,
		exp.KindInt64,
		exp.KindNtile,
		exp.KindRank,
		exp.KindRowNumber,
		exp.KindUnixSeconds,
		exp.KindUnixMicros,
		exp.KindUnixMillis,
	)
	m.returns(exp.DTypeBinary,
		exp.KindFromBase32,
		exp.KindFromBase64,
	)
	m.returns(exp.DTypeBoolean,
		exp.KindAll,
		exp.KindAny,
		exp.KindBetween,
		exp.KindBoolean,
		exp.KindContains,
		exp.KindEndsWith,
		exp.KindExists,
		exp.KindIn,
		exp.KindIsInf,
		exp.KindIsNan,
		exp.KindLogicalAnd,
		exp.KindLogicalOr,
		exp.KindStartsWith,
	)
	m.returns(exp.DTypeDate,
		exp.KindCurrentDate,
		exp.KindDate,
		exp.KindDateFromParts,
		exp.KindDateStrToDate,
		exp.KindDiToDate,
		exp.KindLastDay,
		exp.KindStrToDate,
		exp.KindTimeStrToDate,
		exp.KindTsOrDsToDate,
	)
	m.returns(exp.DTypeDatetime,
		exp.KindCurrentDatetime,
		exp.KindDatetime,
		exp.KindDatetimeAdd,
		exp.KindDatetimeSub,
	)
	m.returns(exp.DTypeDouble,
		exp.KindAsin,
		exp.KindAsinh,
		exp.KindAcos,
		exp.KindCovarPop,
		exp.KindCovarSamp,
		exp.KindAcosh,
		exp.KindApproxQuantile,
		exp.KindAtan,
		exp.KindAtanh,
		exp.KindAvg,
		exp.KindCbrt,
		exp.KindCos,
		exp.KindCosh,
		exp.KindCot,
		exp.KindDegrees,
		exp.KindExp,
		exp.KindKurtosis,
		exp.KindLn,
		exp.KindLog,
		exp.KindPi,
		exp.KindPow,
		exp.KindPercentileCont,
		exp.KindQuantile,
		exp.KindRadians,
		exp.KindRound,
		exp.KindSafeDivide,
		exp.KindSin,
		exp.KindSinh,
		exp.KindSqrt,
		exp.KindStddev,
		exp.KindStddevPop,
		exp.KindStddevSamp,
		exp.KindRand,
		exp.KindTan,
		exp.KindTanh,
		exp.KindToDouble,
		exp.KindCumeDist,
		exp.KindPercentRank,
		exp.KindVariance,
		exp.KindVariancePop,
		exp.KindSkewness,
	)
	m.returns(exp.DTypeInt,
		exp.KindAscii,
		exp.KindBitLength,
		exp.KindCeil,
		exp.KindDatetimeDiff,
		exp.KindDayOfMonth,
		exp.KindDayOfWeek,
		exp.KindDayOfYear,
		exp.KindFloor,
		exp.KindGetbit,
		exp.KindHour,
		exp.KindTimestampDiff,
		exp.KindTimeDiff,
		exp.KindUnicode,
		exp.KindDateToDi,
		exp.KindLevenshtein,
		exp.KindLength,
		exp.KindSign,
		exp.KindStrPosition,
		exp.KindTsOrDiToDi,
		exp.KindQuarter,
		exp.KindUnixDate,
	)
	m.returns(exp.DTypeInterval,
		exp.KindInterval,
		exp.KindJustifyDays,
		exp.KindJustifyHours,
		exp.KindJustifyInterval,
		exp.KindMakeInterval,
	)
	m.returns(exp.DTypeJSON,
		exp.KindParseJSON,
	)
	m.returns(exp.DTypeTime,
		exp.KindCurrentTime,
		exp.KindLocaltime,
		exp.KindTime,
		exp.KindTimeAdd,
		exp.KindTimeSub,
	)
	m.returns(exp.DTypeTimestampLtz,
		exp.KindTimestampLtzFromParts,
	)
	m.returns(exp.DTypeTimestampTz,
		exp.KindCurrentTimestampLTZ,
		exp.KindTimestampTzFromParts,
	)
	m.returns(exp.DTypeTimestamp, timestampExpressions...)
	m.returns(exp.DTypeTinyInt,
		exp.KindDay,
		exp.KindDayOfWeekIso,
		exp.KindMonth,
		exp.KindWeek,
		exp.KindWeekOfYear,
		exp.KindYear,
		exp.KindYearOfWeek,
		exp.KindYearOfWeekIso,
	)
	m.returns(exp.DTypeVarchar,
		exp.KindArrayToString,
		exp.KindConcat,
		exp.KindConcatWs,
		exp.KindChr,
		exp.KindCurrentCatalog,
		exp.KindCurrentRole,
		exp.KindCurrentSchema,
		exp.KindCurrentVersion,
		exp.KindCurrentUser,
		exp.KindDayname,
		exp.KindDateToDateStr,
		exp.KindDPipe,
		exp.KindGroupConcat,
		exp.KindInitcap,
		exp.KindLower,
		exp.KindMD5,
		exp.KindMonthname,
		exp.KindRawString,
		exp.KindRepeat,
		exp.KindSHA,
		exp.KindSHA2,
		exp.KindSessionUser,
		exp.KindSpace,
		exp.KindString,
		exp.KindSubstring,
		exp.KindTimeToStr,
		exp.KindTimeToTimeStr,
		exp.KindTrim,
		exp.KindToBase32,
		exp.KindToBase64,
		exp.KindTranslate,
		exp.KindTsOrDsToDateStr,
		exp.KindTypeof,
		exp.KindUnixToStr,
		exp.KindUnixToTimeStr,
		exp.KindUpper,
	)
	// exp.HavingMax is listed here upstream; the port has no HavingMax node (parser.go
	// _parse_having_max is not ported), so there is nothing to annotate.
	m.annotate(byArgs("this"),
		exp.KindAbs,
		exp.KindAnyValue,
		exp.KindArrayConcatAgg,
		exp.KindArrayReverse,
		exp.KindArraySlice,
		exp.KindFilter,
		exp.KindFirstValue,
		exp.KindLastValue,
		exp.KindLimit,
		exp.KindNthValue,
		exp.KindOrder,
		exp.KindSortArray,
		exp.KindWindow,
	)
	m.annotate(byArgs("this", "expressions"),
		exp.KindArrayConcat,
		exp.KindCoalesce,
		exp.KindGreatest,
		exp.KindLeast,
		exp.KindMax,
		exp.KindMin,
	)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateByArrayElement(e) },
		exp.KindArrayFirst,
		exp.KindArrayLast,
	)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		t, err := a.schema.GetUDFType(e, nil, nil)
		if err != nil {
			panic(err)
		}
		return a.SetType(e, t)
	}, exp.KindAnonymous)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateTimeunit(e) },
		exp.KindDateAdd,
		exp.KindDateSub,
		exp.KindDateTrunc,
	)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.SetType(e, asExpression(e.Arg("to")))
	},
		exp.KindCast,
		exp.KindTryCast,
	)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateMap(e) },
		exp.KindMap,
		exp.KindVarMap,
	)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.AnnotateByArgs(e, []any{"expressions"}, false, true)
	}, exp.KindArray)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.AnnotateByArgs(e, []any{"this"}, false, true)
	}, exp.KindArrayAgg)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateBracket(e) }, exp.KindBracket)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		var args []any
		for _, ifExpr := range expressionsFor(e, "ifs") {
			args = append(args, asExpression(ifExpr.Arg("true")))
		}
		return a.AnnotateByArgs(e, append(args, "default"), false, false)
	}, exp.KindCase)
	bigIntOrInt := func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		if truthy(e.Arg("big_int")) {
			return a.SetType(e, exp.DTypeBigInt)
		}
		return a.SetType(e, exp.DTypeInt)
	}
	m.annotate(bigIntOrInt, exp.KindCount)
	m.annotate(bigIntOrInt, exp.KindDateDiff)
	m.annotate(func(_ *TypeAnnotator, e exp.Expression) exp.Expression { return e }, exp.KindDataType)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.AnnotateDiv(e) }, exp.KindDiv)
	m.annotate(byArgs("expressions"), exp.KindDistinct)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateDot(e) }, exp.KindDot)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateExplode(e) }, exp.KindExplode)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateExtract(e) }, exp.KindExtract)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		if truthy(e.Arg("is_integer")) {
			return a.SetType(e, exp.DTypeBigInt)
		}
		return a.SetType(e, exp.DTypeBinary)
	}, exp.KindHexString)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.AnnotateByArgs(e, []any{"start", "end", "step"}, false, true)
	}, exp.KindGenerateSeries)
	m.annotate(setTypeFromStr("ARRAY<DATE>", ""), exp.KindGenerateDateArray)
	m.annotate(setTypeFromStr("ARRAY<TIMESTAMP>", ""), exp.KindGenerateTimestampArray)
	m.annotate(byArgs("true", "false"), exp.KindIf)
	m.annotate(byArgs("this", "default"), exp.KindLag)
	m.annotate(byArgs("this", "default"), exp.KindLead)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateLiteral(e) }, exp.KindLiteral)
	m.returns(exp.DTypeNull, exp.KindNull)
	m.annotate(byArgs("this", "expression"), exp.KindNullif)
	m.annotate(byArgs("expression"), exp.KindPropertyEQ)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateStruct(e) }, exp.KindStruct)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.AnnotateByArgs(e, []any{"this", "expressions"}, true, false)
	}, exp.KindSum)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		if truthy(e.Arg("with_tz")) {
			return a.SetType(e, exp.DTypeTimestampTz)
		}
		return a.SetType(e, exp.DTypeTimestamp)
	}, exp.KindTimestamp)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateToMap(e) }, exp.KindToMap)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateUnnest(e) }, exp.KindUnnest)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateWithinGroup(e) }, exp.KindWithinGroup)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression { return a.annotateSubquery(e) }, exp.KindSubquery)
	return m
}

// dialectTyping is a dialect's EXPRESSION_METADATA plus its COERCES_TO (empty means the
// TypeAnnotator default). Upstream keeps both on the Dialect class; they live here because the
// tables reference TypeAnnotator, which the dialects package cannot import.
type dialectTyping struct {
	metadata  ExprMetadata
	coercesTo CoercesTo
}

var (
	typingOnce   sync.Once
	typingTables map[string]*dialectTyping
)

func buildTypingTables() {
	base := &dialectTyping{metadata: baseExpressionMetadata()}
	hive := &dialectTyping{metadata: hiveExpressionMetadata(base.metadata), coercesTo: hiveCoercesTo()}
	spark2 := &dialectTyping{metadata: spark2ExpressionMetadata(hive.metadata), coercesTo: hive.coercesTo}
	spark := &dialectTyping{metadata: sparkExpressionMetadata(spark2.metadata), coercesTo: hive.coercesTo}
	presto := &dialectTyping{metadata: prestoExpressionMetadata(base.metadata)}
	typingTables = map[string]*dialectTyping{
		"base":       base,
		"athena":     base,
		"mysql":      {metadata: mysqlExpressionMetadata(base.metadata)},
		"postgres":   {metadata: postgresExpressionMetadata(base.metadata)},
		"presto":     presto,
		"trino":      presto,
		"hive":       hive,
		"spark2":     spark2,
		"spark":      spark,
		"databricks": {metadata: databricksExpressionMetadata(spark.metadata), coercesTo: databricksCoercesTo()},
		"clickhouse": {metadata: clickhouseExpressionMetadata(base.metadata)},
		"snowflake":  {metadata: snowflakeExpressionMetadata(base.metadata)},
	}
}

// typingFor resolves Dialect.EXPRESSION_METADATA / Dialect.COERCES_TO by dialect name, following
// the upstream class inheritance (Trino extends Presto, Spark extends Spark2 extends Hive, ...).
func typingFor(d *dialects.Dialect) *dialectTyping {
	typingOnce.Do(buildTypingTables)
	if t, ok := typingTables[d.Name]; ok {
		return t
	}
	return typingTables["base"]
}
