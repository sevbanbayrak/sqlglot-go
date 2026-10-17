package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// prestoExpressionMetadata ports typing/presto.py.
func prestoExpressionMetadata(base ExprMetadata) ExprMetadata {
	m := base.Copy()
	m.returns(exp.DTypeBigInt,
		exp.KindBitwiseAnd,
		exp.KindBitwiseNot,
		exp.KindBitwiseOr,
		exp.KindBitwiseXor,
		exp.KindLength,
		exp.KindLevenshtein,
		exp.KindStrPosition,
		exp.KindWidthBucket,
	)
	m.annotate(byArgs("this"),
		exp.KindCeil,
		exp.KindFloor,
		exp.KindRound,
		exp.KindSign,
	)
	m.annotate(byArgs("this", "expression"), exp.KindMod)
	m.annotate(func(a *TypeAnnotator, e exp.Expression) exp.Expression {
		if e.This() != nil {
			return a.AnnotateByArgs(e, []any{"this"}, false, false)
		}
		return a.SetType(e, exp.DTypeDouble)
	}, exp.KindRand)
	m.returns(exp.DTypeVarBinary, exp.KindMD5Digest)
	return m
}
