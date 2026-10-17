package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// clickhouseExpressionMetadata ports typing/clickhouse.py.
func clickhouseExpressionMetadata(base ExprMetadata) ExprMetadata {
	m := base.Copy()
	m.returns(exp.DTypeUBigInt,
		exp.KindCountIf,
	)
	m.annotate(setTypeFromStr("FixedString(16)", "clickhouse"), exp.KindMD5Digest)
	m.annotate(setTypeFromStr("Float64", "clickhouse"), exp.KindCorr)
	return m
}
