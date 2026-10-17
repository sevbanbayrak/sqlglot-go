package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

var compressLongblobTypes = []any{
	exp.DTypeText,
	exp.DTypeMediumText,
	exp.DTypeLongText,
	exp.DTypeBlob,
	exp.DTypeMediumBlob,
	exp.DTypeLongBlob,
	exp.DTypeJSON,
}

var compressVarbinaryTypes = []any{
	exp.DTypeChar,
	exp.DTypeVarchar,
	exp.DTypeBinary,
	exp.DTypeVarBinary,
	exp.DTypeTinyBlob,
	exp.DTypeEnum,
	exp.DTypeInt,
	exp.DTypeBigInt,
	exp.DTypeDecimal,
	exp.DTypeDouble,
	exp.DTypeDate,
	exp.DTypeDatetime,
}

// typeSet turns one of the exp.*Types sets into is_type arguments.
func typeSet(types map[exp.DType]bool) []any {
	out := make([]any, 0, len(types))
	for t := range types {
		out = append(out, t)
	}
	return out
}

func mysqlAnnotateReverse(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	if expression.This().IsType(exp.DTypeBinary, exp.DTypeVarBinary, exp.DTypeUnknown) {
		a.AnnotateByArgs(expression, []any{"this"}, false, false)
	} else {
		a.SetType(expression, exp.DTypeVarchar)
	}

	return expression
}

func mysqlAnnotateTruncate(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	if expression.This().IsType(typeSet(exp.TextTypes)...) {
		return a.SetType(expression, exp.DTypeDouble)
	}

	return a.AnnotateByArgs(expression, []any{"this"}, false, false)
}

func mysqlAnnotateRegexpReplace(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	args := []exp.Expression{expression.This(), expression.Expr(), asExpression(expression.Arg("replacement"))}

	hasBinary := false
	for _, arg := range args {
		if arg != nil {
			if arg.IsType(exp.DTypeUnknown) {
				return a.SetType(expression, exp.DTypeUnknown)
			}
			hasBinary = hasBinary || arg.IsType(typeSet(exp.BinaryTypes)...)
		}
	}

	if hasBinary {
		return a.SetType(expression, exp.DTypeLongBlob)
	}
	return a.SetType(expression, exp.DTypeLongText)
}

func mysqlAnnotateCompress(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	this := expression.This()

	if this.IsType(compressVarbinaryTypes...) {
		return a.SetType(expression, exp.DTypeVarBinary)
	}

	if this.IsType(compressLongblobTypes...) {
		return a.SetType(expression, exp.DTypeLongBlob)
	}

	if this.IsType(exp.DTypeTinyText) {
		return a.SetType(expression, exp.DTypeBlob)
	}

	return a.SetType(expression, exp.DTypeUnknown)
}

func mysqlAnnotateBitFunc(a *TypeAnnotator, expression exp.Expression) exp.Expression {
	this := expression.This()

	if this.IsType(exp.DTypeUnknown) {
		return a.SetType(expression, exp.DTypeUnknown)
	}

	if this.IsType(typeSet(exp.BinaryTypes)...) {
		return a.SetType(expression, exp.DTypeVarBinary)
	}

	return a.SetType(expression, exp.DTypeUBigInt)
}

// mysqlExpressionMetadata ports typing/mysql.py.
func mysqlExpressionMetadata(base ExprMetadata) ExprMetadata {
	m := base.Copy()
	m.returns(exp.DTypeDouble,
		exp.KindAtan2,
		exp.KindMatchAgainst,
		exp.KindStDistance,
	)
	m.returns(exp.DTypeDatetime,
		exp.KindCurrentTimestamp,
		exp.KindConvertTimezone,
		exp.KindLocaltime,
		exp.KindLocaltimestamp,
		exp.KindUtcTimestamp,
	)
	m.returns(exp.DTypeDate,
		exp.KindUtcDate,
	)
	m.returns(exp.DTypeVarchar,
		exp.KindElt,
		exp.KindHex,
		exp.KindNumberToStr, // format()
		exp.KindReplace,
		exp.KindStuff, // insert function
		exp.KindSubstringIndex,
		exp.KindRegexpSubstr,
		exp.KindCollation,
		exp.KindJSONType,
		exp.KindUuid,
	)
	m.returns(exp.DTypeInt,
		exp.KindMonth,
		exp.KindSecond,
		exp.KindWeek,
		exp.KindMinute,
	)
	m.returns(exp.DTypeBigInt,
		exp.KindRegexpInstr,
		exp.KindGrouping,
	)
	m.returns(exp.DTypeTime,
		exp.KindTimeFromParts,
		exp.KindUtcTime,
	)
	m.returns(exp.DTypeVarBinary,
		exp.KindUnhex,
	)
	m.returns(exp.DTypeJSON,
		exp.KindJSONObjectAgg,
		exp.KindJSONObject,
		exp.KindJSONExtract,
		exp.KindJSONKeys,
		exp.KindJSONArrayAppend,
		exp.KindJSONArrayInsert,
		exp.KindJSONRemove,
		exp.KindJSONSet,
	)
	m.returns(exp.DTypeLongText,
		exp.KindCurrentRole,
	)
	m.annotate(byArgs("this"),
		exp.KindPad,
		exp.KindLeft,
		exp.KindRight,
		exp.KindLead,
		exp.KindLag,
	)
	m.annotate(mysqlAnnotateBitFunc,
		exp.KindBitwiseAndAgg,
		exp.KindBitwiseXorAgg,
		exp.KindBitwiseOrAgg,
	)
	m.annotate(mysqlAnnotateReverse, exp.KindReverse)
	m.annotate(mysqlAnnotateTruncate, exp.KindTrunc)
	m.annotate(mysqlAnnotateRegexpReplace, exp.KindRegexpReplace)
	m.annotate(mysqlAnnotateCompress, exp.KindCompress)
	return m
}
