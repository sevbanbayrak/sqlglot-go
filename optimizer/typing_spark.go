package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// sparkExpressionMetadata ports typing/spark.py.
func sparkExpressionMetadata(spark2 ExprMetadata) ExprMetadata {
	m := spark2.Copy()
	m.returns(exp.DTypeBinary,
		exp.KindBitmapConstructAgg,
		exp.KindToBinary,
	)
	m.returns(exp.DTypeDate,
		exp.KindDateFromUnixDate,
		// 2-arg `date_add(startDate, numDays)` / `date_sub` are routed to
		// TsOrDsAdd by Hive/Spark parsers; both return DATE per the Spark
		// and Databricks contracts.
		exp.KindTsOrDsAdd,
	)
	m.returns(exp.DTypeDouble,
		exp.KindSec,
	)
	m.returns(exp.DTypeVarchar,
		exp.KindCollation,
		exp.KindCurrentTimezone,
		exp.KindRandstr,
		exp.KindToChar,
	)
	m.annotate(byArgs("this"),
		exp.KindArrayCompact,
		exp.KindArrayInsert,
		exp.KindBitwiseAndAgg,
		exp.KindBitwiseOrAgg,
		exp.KindBitwiseXorAgg,
		exp.KindLeft,
		exp.KindOverlay,
	)
	m.returns(exp.DTypeBigInt, exp.KindBitmapCount)
	m.returns(exp.DTypeTinyInt, exp.KindGrouping)
	m.returns(exp.DTypeTimestampNtz, exp.KindLocaltimestamp)
	return m
}
