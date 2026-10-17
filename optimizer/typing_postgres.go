package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// postgresExpressionMetadata ports typing/postgres.py.
func postgresExpressionMetadata(base ExprMetadata) ExprMetadata {
	m := base.Copy()
	m.returns(exp.DTypeInt,
		exp.KindNtile,
		exp.KindWidthBucket,
	)
	m.returns(exp.DTypeText,
		exp.KindEncode,
	)
	return m
}
