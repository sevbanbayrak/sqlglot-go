package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// databricksExpressionMetadata ports typing/databricks.py.
func databricksExpressionMetadata(spark ExprMetadata) ExprMetadata {
	m := spark.Copy()
	m.returns(exp.DTypeDouble,
		exp.KindRegrAvgx,
		exp.KindRegrAvgy,
		exp.KindRegrIntercept,
		exp.KindRegrR2,
		exp.KindRegrSlope,
		exp.KindRegrSxx,
		exp.KindRegrSxy,
		exp.KindRegrSyy,
		exp.KindRint,
	)
	m.returns(exp.DTypeInt,
		exp.KindRegexpCount,
		exp.KindRegexpInstr,
	)
	m.returns(exp.DTypeVarchar,
		exp.KindRegexpSubstr,
		exp.KindSecret,
	)
	m.returns(exp.DTypeBigInt, exp.KindRegrCount)
	m.returns(exp.DTypeBoolean, exp.KindSearch)
	m.annotate(setTypeFromStr("ARRAY<STRING>", "databricks"), exp.KindRegexpExtractAll)
	return m
}

// databricksCoercesTo ports Databricks.COERCES_TO (dialects/databricks.py:20-28).
func databricksCoercesTo() CoercesTo {
	c := copyCoercesTo(TypeAnnotatorCoercesTo)
	targets := map[exp.DType]bool{exp.DTypeBinary: true, exp.DTypeBoolean: true, exp.DTypeInterval: true}
	for t := range exp.NumericTypes {
		targets[t] = true
	}
	for t := range exp.TemporalTypes {
		targets[t] = true
	}
	for t := range exp.TextTypes {
		c.add(t, targets)
	}
	return c
}
