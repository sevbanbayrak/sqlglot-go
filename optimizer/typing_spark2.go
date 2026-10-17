package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// annotateBySimilarArgs ports _annotate_by_similar_args (typing/spark2.py:15-44): type inference
// for CONCAT-family expressions (CONCAT, LPAD, RPAD).
//
//   - All-BINARY → BINARY (the binary overload).
//   - Otherwise, if any arg has a known, non-array, non-binary type → STRING.
//   - Else → UNKNOWN.
func annotateBySimilarArgs(a *TypeAnnotator, expression exp.Expression, argKeys ...string) exp.Expression {
	var argExprs []exp.Expression
	for _, key := range argKeys {
		argExprs = append(argExprs, exprList(expression.Arg(key))...)
	}

	allBinary := len(argExprs) > 0
	for _, e := range argExprs {
		if !e.IsType(exp.DTypeBinary) {
			allBinary = false
			break
		}
	}
	anyKnown := false
	for _, e := range argExprs {
		if e.Type() != nil && !e.IsType(exp.DTypeUnknown, exp.DTypeArray, exp.DTypeBinary) {
			anyKnown = true
			break
		}
	}

	var result exp.DType
	switch {
	case allBinary:
		result = exp.DTypeBinary
	case anyKnown:
		result = exp.DTypeText
	default:
		result = exp.DTypeUnknown
	}

	a.SetType(expression, result)
	return expression
}

// spark2ExpressionMetadata ports typing/spark2.py.
func spark2ExpressionMetadata(hive ExprMetadata) ExprMetadata {
	m := hive.Copy()
	m.returns(exp.DTypeDouble,
		exp.KindAtan2,
		exp.KindRandn,
	)
	m.returns(exp.DTypeVarchar,
		exp.KindFormat,
		exp.KindRight,
	)
	m.annotate(byArgs("this"),
		exp.KindArrayFilter,
		exp.KindShuffle,
		exp.KindSubstring,
	)
	m.returns(exp.DTypeDouble,
		exp.KindNanvl,
	)
	m.returns(exp.DTypeDate, exp.KindAddMonths)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		quantile := asExpression(e.Arg("quantile"))
		return a.AnnotateByArgs(e, []any{"this"}, false, quantile != nil && quantile.IsType(exp.DTypeArray))
	}, exp.KindApproxQuantile)
	m.returns(exp.DTypeTimestamp, exp.KindAtTimeZone)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return annotateBySimilarArgs(a, e, "expressions")
	}, exp.KindConcat)
	m.returns(exp.DTypeDate, exp.KindNextDay)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		return annotateBySimilarArgs(a, e, "this", "fill_pattern")
	}, exp.KindPad)
	return m
}
