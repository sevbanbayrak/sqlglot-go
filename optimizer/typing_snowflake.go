package optimizer

import (
	"fmt"
	"strconv"
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
)

var snowflakeDateParts = map[string]bool{"DAY": true, "WEEK": true, "MONTH": true, "QUARTER": true, "YEAR": true}

const (
	snowflakeMaxPrecision = 38
	snowflakeMaxScale     = 37
)

func snowflakeAnnotateReverse(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	expression = a.AnnotateByArgs(expression, []any{"this"}, false, false)
	if expression.IsType(exp.DTypeNull) {
		// Snowflake treats REVERSE(NULL) as a VARCHAR
		a.SetType(expression, exp.DTypeVarchar)
	}

	return expression
}

// snowflakeAnnotateTimestampFromParts: TIMESTAMP_FROM_PARTS with time_zone -> TIMESTAMPTZ,
// without -> TIMESTAMP (defaults to TIMESTAMP_NTZ).
func snowflakeAnnotateTimestampFromParts(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	if expression.Arg("zone") != nil {
		a.SetType(expression, exp.DTypeTimestampTz)
	} else {
		a.SetType(expression, exp.DTypeTimestamp)
	}

	return expression
}

func snowflakeAnnotateDateOrTimeAdd(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	if expression.This().IsType(exp.DTypeDate) && !snowflakeDateParts[strings.ToUpper(expression.Text("unit"))] {
		a.SetType(expression, exp.DTypeTimestampNtz)
	} else {
		a.AnnotateByArgs(expression, []any{"this"}, false, false)
	}
	return expression
}

// snowflakeAnnotateDecodeCase types DECODE(expr, val1, ret1, val2, ret2, ..., default) from its
// return values only, not the comparison values or the expression being compared.
func snowflakeAnnotateDecodeCase(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	expressions := expression.Expressions()

	// Return values are at indices 2, 4, 6, ... and the last element (if even length)
	var returnTypes []any
	for i := 2; i < len(expressions); i += 2 {
		returnTypes = append(returnTypes, typeOrNil(expressions[i].Type()))
	}

	// If the total number of expressions is even, the last one is the default
	if len(expressions)%2 == 0 && len(expressions) > 0 {
		returnTypes = append(returnTypes, typeOrNil(expressions[len(expressions)-1].Type()))
	}

	// Determine the common type from all return values
	var lastType any
	for _, retType := range returnTypes {
		if lastType == nil {
			lastType = retType
		}
		lastType = a.MaybeCoerce(lastType, retType)
	}

	a.SetType(expression, lastType)
	return expression
}

func snowflakeAnnotateArgMaxMin(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	if truthy(expression.Arg("count")) {
		a.SetType(expression, exp.DTypeArray)
	} else {
		a.SetType(expression, typeOrNil(expression.This().Type()))
	}
	return expression
}

// snowflakeAnnotateWithinGroup re-annotates PercentileDisc/PercentileCont to the ordered
// expression's type.
func snowflakeAnnotateWithinGroup(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	this := expression.This()
	order := expression.Expr()
	if this != nil && (this.Kind() == exp.KindPercentileDisc || this.Kind() == exp.KindPercentileCont) &&
		order != nil && order.Kind() == exp.KindOrder && len(order.Expressions()) == 1 &&
		order.Expressions()[0].Kind() == exp.KindOrdered {
		a.SetType(expression, typeOrNil(order.Expressions()[0].This().Type()))
	} else {
		a.SetType(expression, typeOrNil(this.Type()))
	}

	return expression
}

// typeParamInt is `param.this.to_py()` for a DataType size parameter.
func typeParamInt(param exp.Expression) int {
	if param == nil {
		return 0
	}
	this := param
	if param.Kind() == exp.KindDataTypeParam {
		this = param.This()
	}
	n, err := strconv.Atoi(this.Name())
	if err != nil {
		panic(fmt.Errorf("non-integer type parameter %q", this.Name()))
	}
	return n
}

// snowflakeAnnotateMedian: FLOAT/DOUBLE -> DOUBLE, NUMBER(p, s) -> NUMBER(min(p+3, 38), min(s+3, 37)).
func snowflakeAnnotateMedian(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	// First annotate the argument to get its type
	expression = a.AnnotateByArgs(expression, []any{"this"}, false, false)

	// Get the input type
	inputType := expression.This().Type()

	if inputType.IsType(exp.DTypeDouble) {
		// If input is FLOAT/DOUBLE, return DOUBLE (FLOAT is normalized to DOUBLE in Snowflake)
		a.SetType(expression, exp.DTypeDouble)
	} else {
		// If input is NUMBER(p, s), return NUMBER(min(p+3, 38), min(s+3, 37))
		exprs := inputType.Expressions()

		precision := snowflakeMaxPrecision
		if p, ok := seqGet(exprs, 0); ok {
			precision = typeParamInt(p)
		}

		scale := 0
		if s, ok := seqGet(exprs, 1); ok {
			scale = typeParamInt(s)
		}

		newPrecision := min(precision+3, snowflakeMaxPrecision)
		newScale := min(scale+3, snowflakeMaxScale)

		// Build the new NUMBER type
		a.SetType(expression, dataTypeFromStr(fmt.Sprintf("NUMBER(%d, %d)", newPrecision, newScale), "snowflake"))
	}

	return expression
}

// snowflakeAnnotateVariance types VAR_POP, VAR_SAMP, VARIANCE, VARIANCE_POP:
// DECFLOAT -> DECFLOAT(38), FLOAT/DOUBLE -> FLOAT, INT/NUMBER(p, 0) -> NUMBER(38, 6),
// NUMBER(p, s) -> NUMBER(38, max(12, s)).
func snowflakeAnnotateVariance(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	// First annotate the argument to get its type
	expression = a.AnnotateByArgs(expression, []any{"this"}, false, false)

	// Get the input type
	inputType := expression.This().Type()

	switch {
	// Special case: DECFLOAT -> DECFLOAT(38)
	case inputType.IsType(exp.DTypeDecFloat):
		a.SetType(expression, dataTypeFromStr("DECFLOAT", "snowflake"))
	// Special case: FLOAT/DOUBLE -> DOUBLE
	case inputType.IsType(exp.DTypeFloat, exp.DTypeDouble):
		a.SetType(expression, exp.DTypeDouble)
	// For NUMBER types: determine the scale
	default:
		scale := 0
		if s, ok := seqGet(inputType.Expressions(), 1); ok {
			scale = typeParamInt(s)
		}

		// If scale is 0 (INT, BIGINT, NUMBER(p,0)): return NUMBER(38, 6)
		// Otherwise, Snowflake appears to assign scale through the formula MAX(12, s)
		newScale := max(12, scale)
		if scale == 0 {
			newScale = 6
		}

		// Build the new NUMBER type
		a.SetType(expression, dataTypeFromStr(fmt.Sprintf("NUMBER(%d, %d)", snowflakeMaxPrecision, newScale), "snowflake"))
	}

	return expression
}

// snowflakeAnnotateKurtosis: DECFLOAT -> DECFLOAT, DOUBLE/FLOAT -> DOUBLE, other numerics ->
// NUMBER(38, 12).
func snowflakeAnnotateKurtosis(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	expression = a.AnnotateByArgs(expression, []any{"this"}, false, false)
	inputType := expression.This().Type()

	switch {
	case inputType.IsType(exp.DTypeDecFloat):
		a.SetType(expression, dataTypeFromStr("DECFLOAT", "snowflake"))
	case inputType.IsType(exp.DTypeFloat, exp.DTypeDouble):
		a.SetType(expression, exp.DTypeDouble)
	default:
		a.SetType(expression, dataTypeFromStr(fmt.Sprintf("NUMBER(%d, 12)", snowflakeMaxPrecision), "snowflake"))
	}

	return expression
}

// snowflakeAnnotateMathWithFloatDecfloat: math functions that preserve DECFLOAT but return
// DOUBLE for everything else.
func snowflakeAnnotateMathWithFloatDecfloat(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	expression = a.AnnotateByArgs(expression, []any{"this"}, false, false)

	// If input is DECFLOAT, preserve
	if expression.This().IsType(exp.DTypeDecFloat) {
		a.SetType(expression, expression.This().Type())
	} else {
		// For all other types (integers, decimals, etc.), return DOUBLE
		a.SetType(expression, exp.DTypeDouble)
	}

	return expression
}

func snowflakeAnnotateStrToTime(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	// target_type is stored as a DataType instance
	var targetType any = exp.DTypeTimestamp
	if t := asExpression(expression.Arg("target_type")); t != nil && t.Kind() == exp.KindDataType {
		targetType = t.Arg("this")
	}
	a.SetType(expression, targetType)
	return expression
}

// snowflakeExpressionMetadata ports typing/snowflake.py.
func snowflakeExpressionMetadata(base ExprMetadata) ExprMetadata {
	m := base.Copy()
	m.annotate(byArgs("this"),
		exp.KindAddMonths,
		exp.KindCeil,
		exp.KindDateTrunc,
		exp.KindFloor,
		exp.KindLeft,
		exp.KindMode,
		exp.KindPad,
		exp.KindRight,
		exp.KindRound,
		exp.KindStuff,
		exp.KindSubstring,
		exp.KindTimeSlice,
		exp.KindTimestampTrunc,
	)
	m.returns(exp.DTypeArray,
		exp.KindApproxTopK,
		exp.KindApproxTopKEstimate,
		exp.KindArray,
		exp.KindArrayAgg,
		exp.KindArrayAppend,
		exp.KindArrayCompact,
		exp.KindArrayConcat,
		exp.KindArrayConstructCompact,
		exp.KindArrayPrepend,
		exp.KindArrayRemove,
		exp.KindArraysZip,
		exp.KindArrayUniqueAgg,
		exp.KindArrayUnionAgg,
		exp.KindMapKeys,
		exp.KindRegexpExtractAll,
		exp.KindSplit,
		exp.KindStringToArray,
		exp.KindStrtokToArray,
	)
	m.returns(exp.DTypeBigInt,
		exp.KindBitmapBitPosition,
		exp.KindBitmapBucketNumber,
		exp.KindBitmapCount,
		exp.KindFactorial,
		exp.KindGroupingId,
		exp.KindMD5NumberLower64,
		exp.KindMD5NumberUpper64,
		exp.KindRand,
		exp.KindSeq8,
		exp.KindZipf,
	)
	m.returns(exp.DTypeBinary,
		exp.KindBase64DecodeBinary,
		exp.KindBitmapConstructAgg,
		exp.KindBitmapOrAgg,
		exp.KindCompress,
		exp.KindDecompressBinary,
		exp.KindDecrypt,
		exp.KindDecryptRaw,
		exp.KindEncrypt,
		exp.KindEncryptRaw,
		exp.KindHexString,
		exp.KindMD5Digest,
		exp.KindSHA1Digest,
		exp.KindSHA2Digest,
		exp.KindToBinary,
		exp.KindTryBase64DecodeBinary,
		exp.KindTryHexDecodeBinary,
		exp.KindUnhex,
	)
	m.returns(exp.DTypeBoolean,
		exp.KindBooland,
		exp.KindBoolnot,
		exp.KindBoolor,
		exp.KindBoolxorAgg,
		exp.KindEqualNull,
		exp.KindIsNullValue,
		exp.KindMapContainsKey,
		exp.KindSearch,
		exp.KindSearchIp,
		exp.KindToBoolean,
	)
	m.returns(exp.DTypeDate,
		exp.KindNextDay,
		exp.KindPreviousDay,
	)
	m.annotate(setTypeFromStr("NUMBER", "snowflake"),
		exp.KindBitwiseAndAgg,
		exp.KindBitwiseOrAgg,
		exp.KindBitwiseXorAgg,
		exp.KindRegexpCount,
		exp.KindRegexpInstr,
		exp.KindToNumber,
	)
	m.returns(exp.DTypeDouble,
		exp.KindApproxPercentileEstimate,
		exp.KindApproximateSimilarity,
		exp.KindCosineDistance,
		exp.KindDotProduct,
		exp.KindEuclideanDistance,
		exp.KindManhattanDistance,
		exp.KindMonthsBetween,
		exp.KindNormal,
	)
	m.annotate(snowflakeAnnotateKurtosis, exp.KindKurtosis)
	m.returns(exp.DTypeDecFloat,
		exp.KindToDecfloat,
		exp.KindTryToDecfloat,
	)
	m.annotate(snowflakeAnnotateMathWithFloatDecfloat,
		exp.KindAcos,
		exp.KindAsin,
		exp.KindAtan,
		exp.KindAtan2,
		exp.KindCbrt,
		exp.KindCos,
		exp.KindCot,
		exp.KindDegrees,
		exp.KindExp,
		exp.KindLn,
		exp.KindLog,
		exp.KindPow,
		exp.KindRadians,
		exp.KindRegrAvgx,
		exp.KindRegrAvgy,
		exp.KindRegrCount,
		exp.KindRegrIntercept,
		exp.KindRegrR2,
		exp.KindRegrSlope,
		exp.KindRegrSxx,
		exp.KindRegrSxy,
		exp.KindRegrSyy,
		exp.KindRegrValx,
		exp.KindRegrValy,
		exp.KindSin,
		exp.KindSqrt,
		exp.KindTan,
		exp.KindTanh,
	)
	m.returns(exp.DTypeInt,
		exp.KindByteLength,
		exp.KindDenseRank,
		exp.KindGrouping,
		exp.KindJarowinklerSimilarity,
		exp.KindMapSize,
		exp.KindMinute,
		exp.KindNtile,
		exp.KindRank,
		exp.KindRowNumber,
		exp.KindRtrimmedLength,
		exp.KindSecond,
		exp.KindSeq1,
		exp.KindSeq2,
		exp.KindSeq4,
		exp.KindWidthBucket,
	)
	m.returns(exp.DTypeObject,
		exp.KindApproxPercentileAccumulate,
		exp.KindApproxPercentileCombine,
		exp.KindApproxTopKAccumulate,
		exp.KindApproxTopKCombine,
		exp.KindObjectAgg,
		exp.KindParseIp,
		exp.KindParseUrl,
		exp.KindXMLGet,
	)
	m.returns(exp.DTypeMap,
		exp.KindMapCat,
		exp.KindMapDelete,
		exp.KindMapInsert,
		exp.KindMapPick,
	)
	m.returns(exp.DTypeFile,
		exp.KindToFile,
	)
	m.returns(exp.DTypeTime,
		exp.KindTimeFromParts,
		exp.KindTsOrDsToTime,
	)
	m.returns(exp.DTypeTimestampLtz,
		exp.KindCurrentTimestamp,
		exp.KindLocaltimestamp,
	)
	m.returns(exp.DTypeTinyInt,
		exp.KindDayOfMonth,
		exp.KindDayOfWeek,
		exp.KindDayOfYear,
		exp.KindQuarter,
	)
	m.returns(exp.DTypeVarchar,
		exp.KindAIAgg,
		exp.KindAIClassify,
		exp.KindAISummarizeAgg,
		exp.KindBase64DecodeString,
		exp.KindBase64Encode,
		exp.KindCheckJson,
		exp.KindCheckXml,
		exp.KindCollate,
		exp.KindCollation,
		exp.KindCurrentAccount,
		exp.KindCurrentAccountName,
		exp.KindCurrentAvailableRoles,
		exp.KindCurrentClient,
		exp.KindCurrentDatabase,
		exp.KindCurrentIpAddress,
		exp.KindCurrentSchemas,
		exp.KindCurrentSecondaryRoles,
		exp.KindCurrentSession,
		exp.KindCurrentStatement,
		exp.KindCurrentTransaction,
		exp.KindCurrentWarehouse,
		exp.KindCurrentOrganizationUser,
		exp.KindCurrentRegion,
		exp.KindCurrentRoleType,
		exp.KindCurrentOrganizationName,
		exp.KindDecompressString,
		exp.KindHexDecodeString,
		exp.KindHex,
		exp.KindRandstr,
		exp.KindRegexpExtract,
		exp.KindRegexpReplace,
		exp.KindReplace,
		exp.KindSoundex,
		exp.KindSoundexP123,
		exp.KindSplitPart,
		exp.KindStrtok,
		exp.KindTryBase64DecodeString,
		exp.KindTryHexDecodeString,
		exp.KindUuid,
	)
	m.returns(exp.DTypeVariant,
		exp.KindMinhash,
		exp.KindMinhashCombine,
	)
	m.annotate(snowflakeAnnotateVariance,
		exp.KindVariance,
		exp.KindVariancePop,
	)
	m.annotate(snowflakeAnnotateArgMaxMin, exp.KindArgMax)
	m.annotate(snowflakeAnnotateArgMaxMin, exp.KindArgMin)
	m.annotate(byArgs("expressions"), exp.KindConcatWs)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		if e.Arg("source_tz") != nil {
			return a.SetType(e, exp.DTypeTimestampNtz)
		}
		return a.SetType(e, exp.DTypeTimestampTz)
	}, exp.KindConvertTimezone)
	m.annotate(snowflakeAnnotateDateOrTimeAdd, exp.KindDateAdd)
	m.annotate(snowflakeAnnotateDecodeCase, exp.KindDecodeCase)
	m.annotate(setTypeFromStr("NUMBER(19, 0)", "snowflake"), exp.KindHashAgg)
	m.annotate(snowflakeAnnotateMedian, exp.KindMedian)
	m.annotate(snowflakeAnnotateReverse, exp.KindReverse)
	m.annotate(snowflakeAnnotateStrToTime, exp.KindStrToTime)
	m.annotate(snowflakeAnnotateDateOrTimeAdd, exp.KindTimeAdd)
	m.annotate(snowflakeAnnotateTimestampFromParts, exp.KindTimestampFromParts)
	m.annotate(snowflakeAnnotateWithinGroup, exp.KindWithinGroup)
	return m
}
