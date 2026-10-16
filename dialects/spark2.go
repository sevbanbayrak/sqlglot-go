package dialects

import (
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// Spark2 ports dialects/spark2.py and parsers/spark2.py on top of Hive. Generator deltas live in
// generator/dialect_spark2.go. EXPRESSION_METADATA (typing) is out of scope until annotate_types.
func Spark2() *Dialect {
	d := Hive()
	d.Name = "spark2"
	// dialects/spark2.py:11,29.
	d.AlterTableSupportsCascade = false
	d.InitcapDefaultDelimiterChars = " "
	// generators/spark2.py:130-134,222-223.
	d.Nvl2Supported = true
	d.CanImplementArrayAny = true
	d.AlterSetType = "TYPE"
	d.WrapDerivedValues = false
	// parsers/spark2.py:21-26.
	d.TrimPatternFirst = true
	d.LenientTimeMapping = true
	d.ChangeColumnAlterSyntax = true
	d.FuncTokens = map[tokens.TokenType]bool{tokens.AND: true, tokens.OR: true}

	// parsers/spark2.py:28-84 FUNCTIONS overlay.
	for name, builder := range map[string]func([]exp.Expression) exp.Expression{
		"AGGREGATE":          exp.FromArgListFunc(exp.KindReduce),
		"BOOLEAN":            spark2BuildAsCast(exp.DTypeBoolean),
		"DATE":               spark2BuildAsCast(exp.DTypeDate),
		"DATE_TRUNC":         spark2BuildDateTrunc,
		"DAYOFMONTH":         spark2BuildTsOrDsToDateWrapper(exp.KindDayOfMonth),
		"DAYOFWEEK":          spark2BuildTsOrDsToDateWrapper(exp.KindDayOfWeek),
		"DAYOFYEAR":          spark2BuildTsOrDsToDateWrapper(exp.KindDayOfYear),
		"DOUBLE":             spark2BuildAsCast(exp.DTypeDouble),
		"ELEMENT_AT":         spark2BuildElementAt,
		"FLOAT":              spark2BuildAsCast(exp.DTypeFloat),
		"FORMAT_STRING":      exp.FromArgListFunc(exp.KindFormat),
		"FROM_UTC_TIMESTAMP": spark2BuildZoneCast(exp.KindAtTimeZone),
		"LTRIM":              spark2BuildTrim("LEADING"),
		"INT":                spark2BuildAsCast(exp.DTypeInt),
		"MAP_FROM_ARRAYS":    exp.FromArgListFunc(exp.KindMap),
		"RLIKE":              exp.FromArgListFunc(exp.KindRegexpLike),
		"RTRIM":              spark2BuildTrim("TRAILING"),
		"SHIFTLEFT":          spark2BuildBinary(exp.KindBitwiseLeftShift),
		"SHIFTRIGHT":         spark2BuildBinary(exp.KindBitwiseRightShift),
		"STRING":             spark2BuildAsCast(exp.DTypeText),
		"SLICE":              exp.FromArgListFunc(exp.KindArraySlice),
		"TIMESTAMP":          spark2BuildAsCast(exp.DTypeTimestamp),
		"TO_TIMESTAMP":       spark2BuildToTimestamp,
		"TO_UNIX_TIMESTAMP":  exp.FromArgListFunc(exp.KindStrToUnix),
		"TO_UTC_TIMESTAMP":   spark2BuildZoneCast(exp.KindFromTimeZone),
		"TRUNC":              spark2BuildDateTrunc_,
		"WEEKOFYEAR":         spark2BuildTsOrDsToDateWrapper(exp.KindWeekOfYear),
		// dialects/spark2.py:16-24: the lenient TIME_MAPPING replaces Hive's strict one in the
		// inherited formatted-time builders.
		"DATE_FORMAT": func(args []exp.Expression) exp.Expression {
			this := exp.New(exp.KindTimeStrToTime, exp.Args{"this": hiveSeqGet(args, 0)})
			return hiveBuildFormattedTimeWith(spark2TimeTable, exp.KindTimeToStr, this, hiveSeqGet(args, 1), false)
		},
		"FROM_UNIXTIME": func(args []exp.Expression) exp.Expression {
			return hiveBuildFormattedTimeWith(spark2TimeTable, exp.KindUnixToStr, hiveSeqGet(args, 0), hiveSeqGet(args, 1), true)
		},
		"TO_DATE": func(args []exp.Expression) exp.Expression {
			result := hiveBuildFormattedTimeWith(spark2TimeTable, exp.KindTsOrDsToDate, hiveSeqGet(args, 0), hiveSeqGet(args, 1), false)
			result.Set("safe", true)
			return result
		},
		"UNIX_TIMESTAMP": func(args []exp.Expression) exp.Expression {
			if len(args) == 0 {
				args = []exp.Expression{exp.New(exp.KindCurrentTimestamp, nil)}
			}
			return hiveBuildFormattedTimeWith(spark2TimeTable, exp.KindStrToUnix, hiveSeqGet(args, 0), hiveSeqGet(args, 1), true)
		},
	} {
		d.Functions[name] = builder
	}

	cfg := d.TokenizerConfig
	// dialects/spark2.py:31-37.
	cfg.FormatStrings["x'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["X'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.HasHexStrings = true
	d.HexStart, d.HexEnd = "X'", "'"
	cfg.Keywords["TIMESTAMP"] = tokens.TIMESTAMPTZ
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}

// spark2TimeTable overlays dialects/spark2.py:16-24 on Hive's TIME_MAPPING.
var spark2TimeTable = func() hiveTimeTable {
	mapping := make(map[string]string, len(hiveTimeMapping)+6)
	for k, v := range hiveTimeMapping {
		mapping[k] = v
	}
	for k, v := range map[string]string{"MM": "%m", "dd": "%d", "HH": "%H", "hh": "%I", "mm": "%M", "ss": "%S"} {
		mapping[k] = v
	}
	return hiveTimeTable{mapping: mapping, trie: hiveBuildTimeTrie(mapping)}
}()

func spark2BuildAsCast(to exp.DType) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		return exp.Cast(exp.Args{"this": hiveSeqGet(args, 0), "to": to.IntoExpr(nil)})
	}
}

func spark2BuildTsOrDsToDateWrapper(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		return exp.New(kind, exp.Args{"this": exp.New(exp.KindTsOrDsToDate, exp.Args{"this": hiveSeqGet(args, 0)})})
	}
}

func spark2BuildBinary(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		return exp.New(kind, exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1)})
	}
}

// spark2BuildTrim ports parser.build_trim with reverse_args=True (parsers/spark2.py:59,66).
func spark2BuildTrim(position string) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		this, expression := hiveSeqGet(args, 0), hiveSeqGet(args, 1)
		if expression != nil {
			this, expression = expression, this
		}
		return exp.Trim(exp.Args{"this": this, "expression": expression, "position": position})
	}
}

// spark2BuildZoneCast ports FROM_UTC_TIMESTAMP / TO_UTC_TIMESTAMP (parsers/spark2.py:51-58,75-82).
func spark2BuildZoneCast(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		this := hiveSeqGet(args, 0)
		if this == nil {
			this = exp.Var(exp.Args{"this": ""})
		}
		// exp.cast skips a re-cast when the existing cast type maps to the same name in the
		// dialect (builders.py:488-499); Hive and Spark map every timestamp flavour to TIMESTAMP.
		if this.Kind() != exp.KindCast || !spark2IsTimestampCast(this) {
			this = exp.Cast(exp.Args{"this": this, "to": exp.DTypeTimestamp.IntoExpr(nil)})
		}
		return exp.New(kind, exp.Args{
			"this": this,
			"zone": hiveSeqGet(args, 1),
		})
	}
}

// DATE_TRUNC(unit, this) -> TimestampTrunc (parsers/spark2.py:32-34).
func spark2BuildDateTrunc(args []exp.Expression) exp.Expression {
	unit := hiveSeqGet(args, 0)
	if unit != nil {
		unit = exp.Var(exp.Args{"this": unit.Name()})
	}
	return exp.FromArgList(exp.KindTimestampTrunc, []exp.Expression{hiveSeqGet(args, 1), unit})
}

// TRUNC(this, unit) -> DateTrunc (parsers/spark2.py:83); TimeUnit upper-cases a literal unit.
func spark2BuildDateTrunc_(args []exp.Expression) exp.Expression {
	unit := hiveSeqGet(args, 1)
	if unit != nil && unit.Kind() == exp.KindLiteral && unit.IsString() {
		unit = exp.LiteralString(strings.ToUpper(unit.Name()))
	} else if unit != nil && unit.Kind() == exp.KindVar {
		unit = exp.TimeUnitVar(unit)
	}
	return exp.New(exp.KindDateTrunc, exp.Args{"this": hiveSeqGet(args, 0), "unit": unit})
}

func spark2IsTimestampCast(cast exp.Expression) bool {
	to := cast.Arg("to").(exp.Expression)
	for _, t := range []exp.DType{exp.DTypeTimestamp, exp.DTypeTimestampTz, exp.DTypeTimestampNtz, exp.DTypeTimestampLtz, exp.DTypeDatetime} {
		if exp.IsType(to, t) {
			return true
		}
	}
	return false
}

// ELEMENT_AT(x, i) -> one-based unsafe Bracket (parsers/spark2.py:42-47).
func spark2BuildElementAt(args []exp.Expression) exp.Expression {
	var exprs []exp.Expression
	if index := hiveSeqGet(args, 1); index != nil {
		exprs = []exp.Expression{index}
	}
	return exp.Bracket(exp.Args{"this": hiveSeqGet(args, 0), "expressions": exprs, "offset": 1, "safe": false})
}

// TO_TIMESTAMP(x) -> CAST(x AS TIMESTAMP); TO_TIMESTAMP(x, fmt) -> StrToTime (parsers/spark2.py:69-73).
func spark2BuildToTimestamp(args []exp.Expression) exp.Expression {
	if len(args) == 1 {
		return spark2BuildAsCast(exp.DTypeTimestamp)(args)
	}
	return hiveBuildFormattedTimeWith(spark2TimeTable, exp.KindStrToTime, hiveSeqGet(args, 0), hiveSeqGet(args, 1), false)
}
