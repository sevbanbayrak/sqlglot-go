package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// hiveExpressionMetadata ports typing/hive.py.
func hiveExpressionMetadata(base ExprMetadata) ExprMetadata {
	m := base.Copy()
	m.returns(exp.DTypeBinary,
		exp.KindEncode,
		exp.KindUnhex,
	)
	m.returns(exp.DTypeDouble,
		exp.KindCorr,
		exp.KindMonthsBetween,
		exp.KindSign,
	)
	m.returns(exp.DTypeVarchar,
		exp.KindAddMonths,
		exp.KindCurrentDatabase,
		exp.KindHex,
		exp.KindJSONExtractScalar,
		exp.KindJSONFormat,
		exp.KindNextDay,
		exp.KindRegexpExtract,
		exp.KindRegexpReplace,
		exp.KindReplace,
		exp.KindSoundex,
	)
	m.returns(exp.DTypeBigInt,
		exp.KindFactorial,
		exp.KindIntDiv,
		exp.KindStrToUnix,
	)
	m.returns(exp.DTypeInt,
		exp.KindArraySize,
		exp.KindDenseRank,
		exp.KindMonth,
		exp.KindNtile,
		exp.KindRank,
		exp.KindRowNumber,
		exp.KindSecond,
		exp.KindMinute,
	)
	m.annotate(byArgs("this"),
		exp.KindArrayDistinct,
		exp.KindArrayExcept,
		exp.KindFirst,
		exp.KindLast,
		exp.KindNegative,
		exp.KindReverse,
	)
	m.annotate(byArgs("expressions"), exp.KindArrayIntersect)
	m.annotate(byArgs("quantile"), exp.KindApproxQuantile)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.AnnotateByArgs(e, []any{"this", "expressions"}, true, false)
	}, exp.KindCoalesce)
	m.returns(exp.DTypeBigInt, exp.KindGrouping)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return a.AnnotateByArgs(e, []any{"true", "false"}, true, false)
	}, exp.KindIf)
	m.returns(exp.DTypeDouble, exp.KindPercentileDisc)
	m.annotate(byArgs("quantile"), exp.KindQuantile)
	m.returns(dataTypeFromStr("ARRAY<STRING>", ""), exp.KindRegexpSplit)
	m.returns(dataTypeFromStr("MAP<STRING, STRING>", ""), exp.KindStrToMap)
	m.annotate(byArgs("this"), exp.KindWithinGroup)
	return m
}

// hiveCoercesTo ports Hive.COERCES_TO (dialects/hive.py:37-44): only the non-ANSI mode
// (default for Hive, Spark2, Spark) is supported.
func hiveCoercesTo() CoercesTo {
	c := copyCoercesTo(TypeAnnotatorCoercesTo)
	targets := map[exp.DType]bool{exp.DTypeInterval: true}
	for t := range exp.NumericTypes {
		targets[t] = true
	}
	for t := range exp.TemporalTypes {
		targets[t] = true
	}
	for t := range targets {
		c.add(t, exp.TextTypes)
	}
	return c
}
