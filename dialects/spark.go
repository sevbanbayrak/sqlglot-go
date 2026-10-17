package dialects

import (
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// Spark ports dialects/spark.py and parsers/spark.py on top of Spark2. Generator deltas live in
// generator/dialect_spark.go. EXPRESSION_METADATA (typing) is out of scope until annotate_types.
func Spark() *Dialect {
	d := Spark2()
	d.Name = "spark"
	// dialects/spark.py:13-16.
	d.SupportsOrderByAll = true
	// generators/spark.py:68,70.
	d.SupportsToNumber = true
	d.SupportsConvertTimezone = true
	d.SupportsLimitAll = true
	d.ArrayFuncsPropagatesNulls = true
	// dialects/spark.py:19-28: strict again, so the Hive lenient rewrite applies.
	d.LenientTimeMapping = false
	// generators/spark.py:77-83.
	d.PadFillPatternIsRequired = false
	d.SupportsMedian = true
	d.SetAssignmentRequiresVariableKeyword = true
	d.DecodeCaseUnsupported = false

	for name, builder := range map[string]func([]exp.Expression) exp.Expression{
		"ANY_VALUE":        hiveBuildWithIgnoreNulls(exp.KindAnyValue),
		"ARRAY_INSERT":     sparkBuildArrayInsert,
		"BIT_AND":          exp.FromArgListFunc(exp.KindBitwiseAndAgg),
		"BIT_GET":          exp.FromArgListFunc(exp.KindGetbit),
		"BIT_OR":           exp.FromArgListFunc(exp.KindBitwiseOrAgg),
		"BIT_XOR":          exp.FromArgListFunc(exp.KindBitwiseXorAgg),
		"BIT_COUNT":        exp.FromArgListFunc(exp.KindBitwiseCount),
		"CURDATE":          exp.FromArgListFunc(exp.KindCurrentDate),
		"DATE_ADD":         sparkBuildDateAdd,
		"DATEADD":          sparkBuildDateAdd,
		"MAKE_TIMESTAMP":   exp.FromArgListFunc(exp.KindTimestampFromParts),
		"TIMESTAMPADD":     sparkBuildDateAdd,
		"TIMESTAMPDIFF":    sparkBuildDateDelta(exp.KindTimestampDiff),
		"TRY_ADD":          exp.FromArgListFunc(exp.KindSafeAdd),
		"TRY_DIVIDE":       exp.FromArgListFunc(exp.KindSafeDivide),
		"TRY_MULTIPLY":     exp.FromArgListFunc(exp.KindSafeMultiply),
		"TRY_SUBTRACT":     exp.FromArgListFunc(exp.KindSafeSubtract),
		"DATEDIFF":         sparkBuildDateDiff,
		"DATE_DIFF":        sparkBuildDateDiff,
		"JSON_OBJECT_KEYS": exp.FromArgListFunc(exp.KindJSONKeys),
		"LISTAGG":          exp.FromArgListFunc(exp.KindGroupConcat),
		"TIMESTAMP_LTZ":    spark2BuildAsCast(exp.DTypeTimestampLtz),
		"TIMESTAMP_NTZ":    spark2BuildAsCast(exp.DTypeTimestampNtz),
		"TRY_ELEMENT_AT": func(args []exp.Expression) exp.Expression {
			var exprs []exp.Expression
			if index := hiveSeqGet(args, 1); index != nil {
				exprs = []exp.Expression{index}
			}
			return exp.Bracket(exp.Args{"this": hiveSeqGet(args, 0), "expressions": exprs, "offset": 1, "safe": true})
		},
		"LIKE":  sparkBuildLike(exp.KindLike),
		"ILIKE": sparkBuildLike(exp.KindILike),
		// dialects/spark.py:19-28: strict TIME_MAPPING, so the Hive builders apply again.
		"DATE_FORMAT":    hiveBuildDateFormat,
		"FROM_UNIXTIME":  hiveBuildFromUnixTime,
		"TO_DATE":        hiveBuildToDate,
		"UNIX_TIMESTAMP": hiveBuildUnixTimestamp,
		"TO_TIMESTAMP": func(args []exp.Expression) exp.Expression {
			if len(args) == 1 {
				return spark2BuildAsCast(exp.DTypeTimestamp)(args)
			}
			return hiveBuildFormattedTime(exp.KindStrToTime, hiveSeqGet(args, 0), hiveSeqGet(args, 1), false)
		},
	} {
		d.Functions[name] = builder
	}
	// parser.py:193-275: the array builders read ARRAY_FUNCS_PROPAGATES_NULLS from the dialect.
	for _, name := range []string{"ARRAY_APPEND", "ARRAY_PREPEND", "ARRAY_CONCAT", "ARRAY_CAT", "ARRAY_REMOVE"} {
		if base := exp.FunctionByName[name]; base != nil {
			d.Functions[name] = sparkWithNullPropagation(base)
		}
	}

	cfg := d.TokenizerConfig
	// dialects/spark.py:30-41: raw strings r'...' / R"..." without escapes, DECLARE keyword.
	cfg.StringEscapesAllowedInRawStrings = false
	for quote := range cfg.Quotes {
		cfg.FormatStrings["r"+quote] = tokens.FormatString{End: quote, TokenType: tokens.RAW_STRING}
		cfg.FormatStrings["R"+quote] = tokens.FormatString{End: quote, TokenType: tokens.RAW_STRING}
	}
	cfg.Keywords["DECLARE"] = tokens.DECLARE
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}

// sparkBuildArrayInsert ports parsers/spark.py:65-70.
func sparkBuildArrayInsert(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindArrayInsert, exp.Args{"this": hiveSeqGet(args, 0), "position": hiveSeqGet(args, 1), "expression": hiveSeqGet(args, 2), "offset": 1})
}

// sparkBuildDateDiff ports _build_datediff (parsers/spark.py:15-38).
func sparkBuildDateDiff(args []exp.Expression) exp.Expression {
	var unit exp.Expression
	this, expression := hiveSeqGet(args, 0), hiveSeqGet(args, 1)
	if len(args) == 3 {
		unit = exp.Var(exp.Args{"this": this.Name()})
		this = args[2]
	}
	return exp.FromArgList(exp.KindDateDiff, []exp.Expression{
		exp.New(exp.KindTsOrDsToDate, exp.Args{"this": this}),
		exp.New(exp.KindTsOrDsToDate, exp.Args{"this": expression}),
		unit,
	})
}

// sparkBuildDateAdd ports _build_dateadd (parsers/spark.py:41-52).
func sparkBuildDateAdd(args []exp.Expression) exp.Expression {
	expression := hiveSeqGet(args, 1)
	if len(args) == 2 {
		return exp.FromArgList(exp.KindTsOrDsAdd, []exp.Expression{hiveSeqGet(args, 0), expression, exp.LiteralString("DAY")})
	}
	return exp.FromArgList(exp.KindTimestampAdd, []exp.Expression{hiveSeqGet(args, 2), expression, hiveSeqGet(args, 0)})
}

// sparkBuildDateDelta ports build_date_delta (dialect.py:1636-1655) with default_unit DAY.
func sparkBuildDateDelta(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		unitBased := len(args) >= 3
		this := hiveSeqGet(args, 0)
		unit := exp.Expression(exp.LiteralString("DAY"))
		if unitBased {
			this = args[2]
			unit = args[0]
		}
		return exp.FromArgList(kind, []exp.Expression{this, hiveSeqGet(args, 1), unit})
	}
}

// sparkBuildLike ports build_like (dialect.py:2444-2456).
func sparkBuildLike(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		like := exp.New(kind, exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1)})
		if escape := hiveSeqGet(args, 2); escape != nil {
			return exp.Escape(exp.Args{"this": like, "expression": escape})
		}
		return like
	}
}

func sparkWithNullPropagation(base func([]exp.Expression) exp.Expression) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		out := base(args)
		if out != nil {
			out.Set("null_propagation", true)
		}
		return out
	}
}
