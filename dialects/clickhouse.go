package dialects

import (
	"strconv"
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// ClickHouse ports dialects/clickhouse.py (a direct Dialect subclass) plus the flags and the
// FUNCTIONS overlay of parsers/clickhouse.py and the flags of generators/clickhouse.py.
// Generator deltas live in generator/dialect_clickhouse.go, parser callbacks in
// parser/dialect_clickhouse_overrides.go. typing/clickhouse.py (EXPRESSION_METADATA) is out of
// scope until annotate_types.
func ClickHouse() *Dialect {
	d := Base()
	d.Name = "clickhouse"
	// dialects/clickhouse.py:15-27.
	d.IndexOffset = 1
	d.NormalizeFunctions = ""
	d.NullOrdering = "nulls_are_last"
	d.SupportsUserDefinedTypes = false
	d.LogBaseFirst = nil
	d.ForceEarlyAliasRefExpansion = true
	d.PreserveOriginalNames = true
	d.HexStringIsIntegerType = true
	d.NormalizationStrategy = CaseSensitive
	// TIME_MAPPING is empty, so INVERSE_TIME_MAPPING only holds the strict->lax degradation of
	// _with_strict_time_inverse (dialect.py:151-160).
	d.FormatTime = clickHouseStrictTimeReplacer.Replace
	// dialects/clickhouse.py:35 CREATABLE_KIND_MAPPING.
	d.CreatableKindMapping = map[string]string{"DATABASE": "SCHEMA"}
	// dialects/clickhouse.py:37-41 SET_OP_DISTINCT_BY_DEFAULT: EXCEPT/INTERSECT are ALL by
	// default, a bare UNION is ambiguous (None).
	d.SetOpDistinctByDefault = map[exp.Kind]*bool{exp.KindExcept: &falseValue, exp.KindIntersect: &falseValue, exp.KindUnion: nil}

	// parsers/clickhouse.py flags.
	d.ModifiersAttachedToSetOp = false
	d.IntervalSpans = false
	d.OptionalAliasTokenCTE = false
	d.JoinsHaveEqualPrecedence = true
	d.LogDefaultsToLn = true
	// parsers/clickhouse.py:251-255: TRANSFORM and APPROX_TOP_SUM are not base builders here.
	d.DisabledFunctions = map[string]bool{"TRANSFORM": true, "APPROX_TOP_SUM": true}

	// generators/clickhouse.py:162-187 flags.
	d.TryUnsupported = true
	d.DecodeCaseUnsupported = true
	d.QueryHints = false
	d.StructDelimiter = [2]string{"(", ")"}
	d.Nvl2Supported = false
	d.AlterSetType = "TYPE"
	d.SupportsAlterColumnIfExists = true
	d.TablesampleRequiresParens = false
	d.TablesampleSizeIsRows = false
	d.TablesampleKeywords = "SAMPLE"
	d.LastDaySupportsDatePart = false
	d.CanImplementArrayAny = true
	d.SupportsToNumber = false
	d.JoinHints = false
	d.TableHints = false
	d.GroupingsSep = ""
	d.SetOpModifiers = false
	d.ArraySizeName = "LENGTH"
	d.WrapDerivedValues = false
	d.AutoRefreshBareIntervals = true
	d.SelectKinds = map[string]bool{}

	cfg := tokens.BaseConfig()
	// dialects/clickhouse.py:72-79.
	cfg.Comments = map[string]string{"--": "", "#": "", "#!": "", "/*": "*/"}
	cfg.Identifiers = map[rune]string{'"': `"`, '`': "`"}
	cfg.IdentifierEscapes = map[rune]bool{'\\': true}
	cfg.StringEscapes = map[rune]bool{'\'': true, '\\': true}
	// BYTE_STRING_ESCAPES defaults to a copy of STRING_ESCAPES (tokens.py:96-97).
	cfg.ByteStringEscapes = map[rune]bool{'\'': true, '\\': true}
	// BIT_STRINGS=[("0b","")] and HEX_STRINGS=[("0x",""),("0X","")]: the prefixes contain no
	// SINGLE_TOKENS character, so they only switch on the number scanner's 0b/0x forms.
	cfg.HasBitStrings = true
	cfg.HasHexStrings = true
	d.BitStart, d.BitEnd = "0b", ""
	d.HexStart, d.HexEnd = "0x", ""
	cfg.FormatStrings["$"] = tokens.FormatString{End: "$", TokenType: tokens.HEREDOC_STRING}
	cfg.SingleTokens['$'] = tokens.HEREDOC_STRING
	// dialects/clickhouse.py:27-28, 32-34.
	cfg.NumbersCanBeUnderscoreSeparated = true
	cfg.IdentifiersCanStartWithDigit = true
	cfg.UnescapedSequences = map[string]string{`\0`: "\x00"}

	for keyword, tokenType := range map[string]tokens.TokenType{
		".:":                      tokens.DOTCOLON,
		".^":                      tokens.DOTCARET,
		"ATTACH":                  tokens.COMMAND,
		"DATE32":                  tokens.DATE32,
		"DETACH":                  tokens.DETACH,
		"DATETIME64":              tokens.DATETIME64,
		"DICTIONARY":              tokens.DICTIONARY,
		"DYNAMIC":                 tokens.DYNAMIC,
		"ENUM8":                   tokens.ENUM8,
		"ENUM16":                  tokens.ENUM16,
		"EXCHANGE":                tokens.COMMAND,
		"FINAL":                   tokens.FINAL,
		"FIXEDSTRING":             tokens.FIXEDSTRING,
		"FLOAT32":                 tokens.FLOAT,
		"FLOAT64":                 tokens.DOUBLE,
		"GLOBAL":                  tokens.GLOBAL,
		"LOWCARDINALITY":          tokens.LOWCARDINALITY,
		"MAP":                     tokens.MAP,
		"NESTED":                  tokens.NESTED,
		"NOTHING":                 tokens.NOTHING,
		"SAMPLE":                  tokens.TABLE_SAMPLE,
		"TUPLE":                   tokens.STRUCT,
		"UINT16":                  tokens.USMALLINT,
		"UINT32":                  tokens.UINT,
		"UINT64":                  tokens.UBIGINT,
		"UINT8":                   tokens.UTINYINT,
		"IPV4":                    tokens.IPV4,
		"IPV6":                    tokens.IPV6,
		"POINT":                   tokens.POINT,
		"PROJECTION":              tokens.PROJECTION,
		"RING":                    tokens.RING,
		"LINESTRING":              tokens.LINESTRING,
		"MULTILINESTRING":         tokens.MULTILINESTRING,
		"POLYGON":                 tokens.POLYGON,
		"MULTIPOLYGON":            tokens.MULTIPOLYGON,
		"AGGREGATEFUNCTION":       tokens.AGGREGATEFUNCTION,
		"SIMPLEAGGREGATEFUNCTION": tokens.SIMPLEAGGREGATEFUNCTION,
		"SYSTEM":                  tokens.COMMAND,
		"PREWHERE":                tokens.PREWHERE,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	delete(cfg.Keywords, "/*+")
	d.TokenizerConfig = tokens.CompileConfig(cfg)

	clickHouseFunctions(d)

	return d
}

var falseValue = false

var clickHouseStrictTimeReplacer = func() *strings.Replacer {
	pairs := make([]string, 0, 2*len(strictTimeFormats))
	for _, kv := range strictTimeFormats {
		pairs = append(pairs, kv[0], kv[1])
	}
	return strings.NewReplacer(pairs...)
}()

// clickHouseTimestampTruncUnits ports TIMESTAMP_TRUNC_UNITS (parsers/clickhouse.py:76-88); WEEK
// is skipped since toStartOfWeek takes an extra mode argument.
var clickHouseTimestampTruncUnits = []string{"MICROSECOND", "MILLISECOND", "SECOND", "MINUTE", "HOUR", "DAY", "MONTH", "QUARTER", "YEAR"}

// clickHouseFunctions ports the FUNCTIONS overlay of parsers/clickhouse.py:250-325.
func clickHouseFunctions(d *Dialect) {
	regexpExtract := func(args []exp.Expression) exp.Expression {
		return exp.New(exp.KindRegexpExtract, exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1), "group": hiveSeqGet(args, 2)})
	}
	split := func(kind exp.Kind) func([]exp.Expression) exp.Expression {
		return func(args []exp.Expression) exp.Expression {
			return exp.New(kind, exp.Args{"this": hiveSeqGet(args, 1), "expression": hiveSeqGet(args, 0), "limit": hiveSeqGet(args, 2)})
		}
	}
	sha2 := func(length int) func([]exp.Expression) exp.Expression {
		return func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindSHA2, exp.Args{"this": hiveSeqGet(args, 0), "length": exp.LiteralNumber(length)})
		}
	}
	functions := map[string]func([]exp.Expression) exp.Expression{
		"REGEXPEXTRACT":  regexpExtract,
		"REGEXP_EXTRACT": regexpExtract,
		"REGEXP_SUBSTR":  regexpExtract,
		"ANY":            exp.FromArgListFunc(exp.KindAnyValue),
		"ARRAYCOMPACT":   exp.FromArgListFunc(exp.KindArrayCompact),
		"ARRAYCONCAT":    exp.FromArgListFunc(exp.KindArrayConcat),
		"ARRAYDISTINCT":  exp.FromArgListFunc(exp.KindArrayDistinct),
		"ARRAYEXCEPT":    exp.FromArgListFunc(exp.KindArrayExcept),
		"ARRAYSUM":       exp.FromArgListFunc(exp.KindArraySum),
		"ARRAYMAX":       exp.FromArgListFunc(exp.KindArrayMax),
		"ARRAYMIN":       exp.FromArgListFunc(exp.KindArrayMin),
		"ARRAYREVERSE":   exp.FromArgListFunc(exp.KindArrayReverse),
		"ARRAYSLICE":     exp.FromArgListFunc(exp.KindArraySlice),
		"ARRAYFILTER": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindArrayFilter, exp.Args{"this": hiveSeqGet(args, 1), "expression": hiveSeqGet(args, 0)})
		},
		"ARRAYMAP": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindTransform, exp.Args{"this": hiveSeqGet(args, 1), "expression": hiveSeqGet(args, 0)})
		},
		"CURRENTDATABASE":   exp.FromArgListFunc(exp.KindCurrentDatabase),
		"CURRENTSCHEMAS":    exp.FromArgListFunc(exp.KindCurrentSchemas),
		"COUNTIF":           clickHouseBuildCountIf,
		"CITYHASH64":        exp.FromArgListFunc(exp.KindCityHash64),
		"COSINEDISTANCE":    exp.FromArgListFunc(exp.KindCosineDistance),
		"VERSION":           exp.FromArgListFunc(exp.KindCurrentVersion),
		"DATE_ADD":          buildDateDelta(exp.KindDateAdd, "", false),
		"DATEADD":           buildDateDelta(exp.KindDateAdd, "", false),
		"DATE_DIFF":         buildDateDelta(exp.KindDateDiff, "", true),
		"DATEDIFF":          buildDateDelta(exp.KindDateDiff, "", true),
		"DATE_FORMAT":       clickHouseBuildDatetimeFormat(exp.KindTimeToStr),
		"DATE_SUB":          buildDateDelta(exp.KindDateSub, "", false),
		"DATESUB":           buildDateDelta(exp.KindDateSub, "", false),
		"DATETRUNC":         exp.FromArgListFunc(exp.KindDateTrunc),
		"FORMATDATETIME":    clickHouseBuildDatetimeFormat(exp.KindTimeToStr),
		"HAS":               exp.FromArgListFunc(exp.KindArrayContains),
		"ILIKE":             buildLike(exp.KindILike, false),
		"JSONEXTRACTSTRING": buildJSONExtractPathWith(exp.KindJSONExtractScalar, false, false),
		"LENGTH": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindLength, exp.Args{"this": hiveSeqGet(args, 0), "binary": true})
		},
		"LIKE":                  buildLike(exp.KindLike, false),
		"L2Distance":            exp.FromArgListFunc(exp.KindEuclideanDistance),
		"MAP":                   hiveBuildVarMap,
		"MATCH":                 exp.FromArgListFunc(exp.KindRegexpLike),
		"NOTLIKE":               buildLike(exp.KindLike, true),
		"PARSEDATETIME":         clickHouseBuildDatetimeFormat(exp.KindParseDatetime),
		"RANDCANONICAL":         exp.FromArgListFunc(exp.KindRand),
		"STR_TO_DATE":           clickHouseBuildStrToDate,
		"TIMESTAMP_SUB":         buildDateDelta(exp.KindTimestampSub, "", false),
		"TIMESTAMPSUB":          buildDateDelta(exp.KindTimestampSub, "", false),
		"TIMESTAMP_ADD":         buildDateDelta(exp.KindTimestampAdd, "", false),
		"TIMESTAMPADD":          buildDateDelta(exp.KindTimestampAdd, "", false),
		"TOMONDAY":              clickHouseBuildTimestampTrunc("WEEK"),
		"UNIQ":                  exp.FromArgListFunc(exp.KindApproxDistinct),
		"MD5":                   exp.FromArgListFunc(exp.KindMD5Digest),
		"SHA256":                sha2(256),
		"SHA512":                sha2(512),
		"SPLITBYCHAR":           clickHouseBuildSplitByChar,
		"SPLITBYREGEXP":         split(exp.KindRegexpSplit),
		"SPLITBYSTRING":         split(exp.KindSplit),
		"SUBSTRINGINDEX":        exp.FromArgListFunc(exp.KindSubstringIndex),
		"TOTYPENAME":            exp.FromArgListFunc(exp.KindTypeof),
		"EDITDISTANCE":          exp.FromArgListFunc(exp.KindLevenshtein),
		"JAROWINKLERSIMILARITY": exp.FromArgListFunc(exp.KindJarowinklerSimilarity),
		"LEVENSHTEINDISTANCE":   exp.FromArgListFunc(exp.KindLevenshtein),
		"UTCTIMESTAMP":          exp.FromArgListFunc(exp.KindUtcTimestamp),
		// parser.py:443 "LOG": build_logarithm, reading LOG_BASE_FIRST/LOG_DEFAULTS_TO_LN.
		"LOG": buildLogarithm(d),
	}
	for _, unit := range clickHouseTimestampTruncUnits {
		functions["TOSTARTOF"+unit] = clickHouseBuildTimestampTrunc(unit)
	}
	for name, builder := range functions {
		d.Functions[name] = builder
	}
}

// clickHouseBuildDatetimeFormat ports _build_datetime_format (parsers/clickhouse.py:23-34): the
// base build_formatted_time (identity TIME_MAPPING here) plus an optional third zone argument.
func clickHouseBuildDatetimeFormat(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		expr := exp.New(kind, exp.Args{"this": hiveSeqGet(args, 0), "format": hiveSeqGet(args, 1)})
		if zone := hiveSeqGet(args, 2); zone != nil {
			expr.Set("zone", zone)
		}
		return expr
	}
}

// clickHouseBuildCountIf ports _build_count_if (parsers/clickhouse.py:37-41).
func clickHouseBuildCountIf(args []exp.Expression) exp.Expression {
	if len(args) == 1 {
		return exp.New(exp.KindCountIf, exp.Args{"this": hiveSeqGet(args, 0)})
	}
	return exp.New(exp.KindCombinedAggFunc, exp.Args{"this": "countIf", "expressions": args})
}

// clickHouseBuildStrToDate ports _build_str_to_date (parsers/clickhouse.py:44-49).
func clickHouseBuildStrToDate(args []exp.Expression) exp.Expression {
	if len(args) == 3 {
		return exp.Anonymous(exp.Args{"this": "STR_TO_DATE", "expressions": args})
	}
	strToDate := exp.FromArgList(exp.KindStrToDate, args)
	return exp.Cast(exp.Args{"this": strToDate, "to": exp.DTypeDatetime.IntoExpr(nil)})
}

// clickHouseBuildTimestampTrunc ports _build_timestamp_trunc (parsers/clickhouse.py:52-55).
func clickHouseBuildTimestampTrunc(unit string) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		return exp.New(exp.KindTimestampTrunc, exp.Args{"this": hiveSeqGet(args, 0), "unit": exp.Var(exp.Args{"this": unit}), "zone": hiveSeqGet(args, 1)})
	}
}

// clickHouseBuildSplitByChar ports _build_split_by_char (parsers/clickhouse.py:58-65): only a
// single-byte separator literal maps onto Split.
func clickHouseBuildSplitByChar(args []exp.Expression) exp.Expression {
	if sep := hiveSeqGet(args, 0); sep != nil && sep.Kind() == exp.KindLiteral && sep.IsString() && len(sep.Name()) == 1 {
		return exp.New(exp.KindSplit, exp.Args{"this": hiveSeqGet(args, 1), "expression": sep, "limit": hiveSeqGet(args, 2)})
	}
	return exp.Anonymous(exp.Args{"this": "splitByChar", "expressions": args})
}

// buildDateDelta ports build_date_delta (dialect.py:1636-1655) without unit_mapping; an empty
// defaultUnit is upstream's default_unit=None.
func buildDateDelta(kind exp.Kind, defaultUnit string, supportsTimezone bool) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		unitBased := len(args) >= 3
		this := hiveSeqGet(args, 0)
		if unitBased {
			this = args[2]
		}
		var unit exp.Expression
		if unitBased {
			unit = args[0]
		} else if defaultUnit != "" {
			unit = exp.LiteralString(defaultUnit)
		}
		// The TimeUnit constructor (expressions/core.py:2055-2068) normalizes the unit into a Var.
		expression := exp.New(kind, exp.Args{"this": this, "expression": hiveSeqGet(args, 1), "unit": exp.TimeUnitVar(unit)})
		if supportsTimezone && len(args) == 4 {
			expression.Set("zone", args[3])
		}
		return expression
	}
}

// buildLike ports build_like (dialect.py:2444-2456).
func buildLike(kind exp.Kind, notLike bool) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		like := exp.New(kind, exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1)})
		if escape := hiveSeqGet(args, 2); escape != nil {
			like = exp.Escape(exp.Args{"this": like, "expression": escape})
		}
		if notLike {
			like = exp.Not(exp.Args{"this": like})
		}
		return like
	}
}

// clickHouseGenerateValuesAliases ports generate_values_aliases (dialects/clickhouse.py:43-68).
func clickHouseGenerateValuesAliases(values exp.Expression) []exp.Expression {
	rows := values.Expressions()
	if len(rows) == 0 || rows[0] == nil {
		return nil
	}
	first := rows[0].Expressions()
	if len(first) > 1 && first[0].IsString() && first[1].Kind() == exp.KindTuple {
		var aliases []exp.Expression
		for _, coldef := range strings.Split(first[0].Name(), ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(coldef), " ")
			aliases = append(aliases, exp.ToIdentifier(name))
		}
		return aliases
	}
	aliases := make([]exp.Expression, 0, len(first))
	for i := range first {
		aliases = append(aliases, exp.ToIdentifier("c"+strconv.Itoa(i+1)))
	}
	return aliases
}

// buildLogarithm ports build_logarithm (parser.py:74-84): LOG_BASE_FIRST orders the two-argument
// form (None/False swap it), LOG_DEFAULTS_TO_LN picks Ln for the one-argument form.
func buildLogarithm(d *Dialect) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		this, expression := hiveSeqGet(args, 0), hiveSeqGet(args, 1)
		if expression != nil {
			if d.LogBaseFirst == nil || !*d.LogBaseFirst {
				this, expression = expression, this
			}
			return exp.New(exp.KindLog, exp.Args{"this": this, "expression": expression})
		}
		if d.LogDefaultsToLn {
			return exp.New(exp.KindLn, exp.Args{"this": this})
		}
		return exp.New(exp.KindLog, exp.Args{"this": this})
	}
}
