package dialects

import (
	"maps"
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// Snowflake ports dialects/snowflake.py (a direct Dialect subclass) plus the flags and the
// FUNCTIONS overlay of parsers/snowflake.py and the flags of generators/snowflake.py. Generator
// deltas live in generator/dialect_snowflake.go, parser callbacks in
// parser/dialect_snowflake_overrides.go, typing/snowflake.py (EXPRESSION_METADATA) in
// optimizer/typing_snowflake.go.
func Snowflake() *Dialect {
	d := Base()
	d.Name = "snowflake"
	// generators/snowflake.py:394.
	d.SupportsConvertTimezone = true
	// dialects/snowflake.py:17-35.
	d.NormalizationStrategy = Uppercase
	d.NullOrdering = "nulls_are_large"
	d.TimeFormat = "YYYY-MM-DD HH24:MI:SS"
	d.SupportsUserDefinedTypes = false
	d.PreferCTEAliasColumn = true
	d.TablesampleSizeIsPercent = true
	d.CopyParamsAreCsv = false
	d.ArrayAggIncludesNulls = false
	d.ArrayFuncsPropagatesNulls = true
	d.AlterTableAddRequiredForEachColumn = false
	d.TryCastRequiresString = &trueValue
	d.SupportsAliasRefsInJoinConditions = true
	d.UUIDIsStringType = true
	d.InitcapDefaultDelimiterChars = " \t\n\r\f\v!?@\"^#$&~_,.:;+\\-*%/|\\[\\](){}<>"
	d.FormatTime = SnowflakeFormatTime
	d.DatePartMapping = baseDatePartMapping()
	d.DatePartMapping["ISOWEEK"] = "WEEKISO"
	d.DatePartMapping["EPOCH_SECOND"] = "EPOCH_SECOND"
	d.DatePartMapping["EPOCH_SECONDS"] = "EPOCH_SECOND"
	d.ValidIntervalUnits = validIntervalUnits(d.DatePartMapping)
	d.Pseudocolumns = map[string]bool{"LEVEL": true}
	// dialects/snowflake.py:149-151: the JSONPath tokenizer does not treat `$` as the root.
	d.JSONPathNoRootToken = true

	// parsers/snowflake.py:325-329.
	d.IdentifyPivotStrings = true
	d.TypedLambdaArgs = true
	d.DefaultSamplingMethod = "BERNOULLI"
	d.ColonIsVariantExtract = true
	d.JSONExtractRequiresJSONExpression = true

	// generators/snowflake.py:375-401 flags.
	d.SelectKinds = map[string]bool{}
	d.ParameterToken = "$"
	d.SingleStringInterval = true
	d.JoinHints = false
	d.TableHints = false
	d.QueryHints = false
	d.StructDelimiter = [2]string{"(", ")"}
	d.CopyParamsAreWrapped = false
	d.CopyParamsEqRequired = true
	d.ExceptIntersectSupportAllClause = false
	d.SupportsMedian = true
	d.ArraySizeName = "ARRAY_SIZE"
	d.TryUnsupported = true
	d.InsertOverwrite = " OVERWRITE INTO"
	d.StarExcept = "EXCLUDE"
	d.DirectedJoins = true
	d.UescapeUnsupported = true
	d.CollateIsFunc = true
	d.JSONKeyValuePairSep = ","
	d.AutoIncrementTokenName = "AUTOINCREMENT"
	d.StringEscape = "\\"

	cfg := tokens.BaseConfig()
	// dialects/snowflake.py:153-205.
	cfg.StringEscapes = map[rune]bool{'\\': true, '\'': true}
	cfg.ByteStringEscapes = map[rune]bool{'\\': true, '\'': true}
	cfg.FormatStrings["x'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["X'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.HasHexStrings = true
	d.HexStart, d.HexEnd = "x'", "'"
	cfg.FormatStrings["$$"] = tokens.FormatString{End: "$$", TokenType: tokens.RAW_STRING}
	cfg.Comments = map[string]string{"--": "", "//": "", "/*": "*/"}
	cfg.NestedComments = false
	cfg.UnescapedSequences = map[string]string{`\a`: "a", `\v`: "v"}
	for keyword, tokenType := range map[string]tokens.TokenType{
		"BYTEINT":             tokens.INT,
		"FILE://":             tokens.URI_START,
		"FILE FORMAT":         tokens.FILE_FORMAT,
		"GET":                 tokens.GET,
		"INTEGRATION":         tokens.INTEGRATION,
		"MATCH_CONDITION":     tokens.MATCH_CONDITION,
		"MATCH_RECOGNIZE":     tokens.MATCH_RECOGNIZE,
		"MINUS":               tokens.EXCEPT,
		"NCHAR VARYING":       tokens.VARCHAR,
		"PACKAGE":             tokens.PACKAGE,
		"POLICY":              tokens.POLICY,
		"POOL":                tokens.POOL,
		"PUT":                 tokens.PUT,
		"UNDROP":              tokens.UNDROP,
		"REMOVE":              tokens.COMMAND,
		"RM":                  tokens.COMMAND,
		"ROLE":                tokens.ROLE,
		"RULE":                tokens.RULE,
		"SAMPLE":              tokens.TABLE_SAMPLE,
		"SEMANTIC VIEW":       tokens.SEMANTIC_VIEW,
		"SQL_DOUBLE":          tokens.DOUBLE,
		"SQL_VARCHAR":         tokens.VARCHAR,
		"STAGE":               tokens.STAGE,
		"STORAGE INTEGRATION": tokens.STORAGE_INTEGRATION,
		"STREAMLIT":           tokens.STREAMLIT,
		"TAG":                 tokens.TAG,
		"TIMESTAMP_TZ":        tokens.TIMESTAMPTZ,
		"TOP":                 tokens.TOP,
		"VOLUME":              tokens.VOLUME,
		"WAREHOUSE":           tokens.WAREHOUSE,
		"FLOAT":               tokens.DOUBLE,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	delete(cfg.Keywords, "/*+")
	cfg.SingleTokens['$'] = tokens.PARAMETER
	cfg.SingleTokens['!'] = tokens.EXCLAMATION
	cfg.VarSingleTokens = map[rune]bool{'$': true}
	cfg.Commands = maps.Clone(cfg.Commands)
	delete(cfg.Commands, tokens.SHOW)
	d.TokenizerConfig = tokens.CompileConfig(cfg)

	snowflakeFunctions(d)
	return d
}

// snowflakeFunctions ports the FUNCTIONS overlay of parsers/snowflake.py:397-748.
func snowflakeFunctions(d *Dialect) {
	seq := hiveSeqGet
	newArgs := func(kind exp.Kind, keys ...string) func([]exp.Expression) exp.Expression {
		return func(args []exp.Expression) exp.Expression {
			m := exp.Args{}
			for i, key := range keys {
				if v := seq(args, i); v != nil {
					m[key] = v
				}
			}
			return exp.New(kind, m)
		}
	}
	with := func(kind exp.Kind, flags exp.Args, keys ...string) func([]exp.Expression) exp.Expression {
		build := newArgs(kind, keys...)
		return func(args []exp.Expression) exp.Expression {
			node := build(args)
			for key, value := range flags {
				node.Set(key, value)
			}
			return node
		}
	}
	datePart := func(kind exp.Kind, extra exp.Args) func([]exp.Expression) exp.Expression {
		return func(args []exp.Expression) exp.Expression {
			m := exp.Args{"this": seq(args, 2), "expression": seq(args, 1), "unit": MapDatePart(seq(args, 0), nil)}
			for key, value := range extra {
				m[key] = value
			}
			return exp.New(kind, m)
		}
	}
	toNumber := func(safe bool) func([]exp.Expression) exp.Expression {
		return func(args []exp.Expression) exp.Expression { return snowflakeBuildToNumber(args, safe) }
	}
	toChar := func(args []exp.Expression) exp.Expression { return snowflakeBuildTimeToStrOrToChar(d, args) }
	trunc := func(args []exp.Expression) exp.Expression {
		return BuildTrunc(args, d, true, "", false, true)
	}
	geo := func(name string, dtype exp.DType) func([]exp.Expression) exp.Expression {
		return func(args []exp.Expression) exp.Expression {
			if len(args) == 1 {
				return exp.CastTo(args[0], dtype)
			}
			return exp.Anonymous(exp.Args{"this": name, "expressions": args})
		}
	}
	functions := map[string]func([]exp.Expression) exp.Expression{
		"CHARINDEX": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindStrPosition, exp.Args{"this": seq(args, 1), "substr": seq(args, 0), "position": seq(args, 2), "clamp_position": true})
		},
		"ADD_MONTHS":        with(exp.KindAddMonths, exp.Args{"preserve_end_of_month": true}, "this", "expression"),
		"APPROX_PERCENTILE": exp.FromArgListFunc(exp.KindApproxQuantile),
		"CURRENT_TIME":      newArgs(exp.KindLocaltime, "this"),
		"APPROX_TOP_K": func(args []exp.Expression) exp.Expression {
			if len(args) == 1 {
				args = append(args, exp.LiteralNumber(1))
			}
			return exp.FromArgList(exp.KindApproxTopK, args)
		},
		"ARRAY_CONSTRUCT": func(args []exp.Expression) exp.Expression {
			return exp.Array(exp.Args{"expressions": args})
		},
		"ARRAY_CONTAINS": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindArrayContains, exp.Args{"this": seq(args, 1), "expression": seq(args, 0), "ensure_variant": false, "check_null": true})
		},
		"ARRAY_DISTINCT": with(exp.KindArrayDistinct, exp.Args{"check_null": true}, "this"),
		"ARRAY_GENERATE_RANGE": func(args []exp.Expression) exp.Expression {
			return SnowflakeArrayGenerateRange(args)
		},
		"ARRAY_EXCEPT": with(exp.KindArrayExcept, exp.Args{"is_multiset": true}, "this", "expression"),
		"ARRAY_INTERSECTION": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindArrayIntersect, exp.Args{"expressions": args, "is_multiset": true})
		},
		"ARRAY_POSITION": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindArrayPosition, exp.Args{"this": seq(args, 1), "expression": seq(args, 0), "zero_based": true})
		},
		"ARRAY_SLICE": with(exp.KindArraySlice, exp.Args{"zero_based": true}, "this", "start", "end"),
		"ARRAY_SORT": func(args []exp.Expression) exp.Expression {
			asc, nullsFirst := seq(args, 1), seq(args, 2)
			if nullsFirst == nil && asc != nil && asc.Kind() == exp.KindBoolean {
				nullsFirst = exp.Boolean(exp.Args{"this": !boolArgValue(asc.Arg("this"))})
			}
			m := exp.Args{"this": seq(args, 0)}
			if asc != nil {
				m["asc"] = asc
			}
			if nullsFirst != nil {
				m["nulls_first"] = nullsFirst
			}
			return exp.New(exp.KindSortArray, m)
		},
		"ARRAY_FLATTEN":   exp.FromArgListFunc(exp.KindFlatten),
		"ARRAY_TO_STRING": with(exp.KindArrayToString, exp.Args{"null_is_empty": true, "null_delim_is_null": true}, "this", "expression"),
		"ARRAYS_OVERLAP":  with(exp.KindArrayOverlaps, exp.Args{"null_safe": true}, "this", "expression"),
		"BITAND":          snowflakeBuildBitwise(exp.KindBitwiseAnd, "BITAND"),
		"BIT_AND":         snowflakeBuildBitwise(exp.KindBitwiseAnd, "BITAND"),
		"BITNOT":          newArgs(exp.KindBitwiseNot, "this"),
		"BIT_NOT":         newArgs(exp.KindBitwiseNot, "this"),
		"BITXOR":          snowflakeBuildBitwise(exp.KindBitwiseXor, "BITXOR"),
		"BIT_XOR":         snowflakeBuildBitwise(exp.KindBitwiseXor, "BITXOR"),
		"BITOR":           snowflakeBuildBitwise(exp.KindBitwiseOr, "BITOR"),
		"BIT_OR":          snowflakeBuildBitwise(exp.KindBitwiseOr, "BITOR"),
		"BITSHIFTLEFT":    snowflakeBuildBitwise(exp.KindBitwiseLeftShift, "BITSHIFTLEFT"),
		"BIT_SHIFTLEFT":   snowflakeBuildBitwise(exp.KindBitwiseLeftShift, "BIT_SHIFTLEFT"),
		"BITSHIFTRIGHT":   snowflakeBuildBitwise(exp.KindBitwiseRightShift, "BITSHIFTRIGHT"),
		"BIT_SHIFTRIGHT":  snowflakeBuildBitwise(exp.KindBitwiseRightShift, "BIT_SHIFTRIGHT"),
		"BITANDAGG":       exp.FromArgListFunc(exp.KindBitwiseAndAgg),
		"BITAND_AGG":      exp.FromArgListFunc(exp.KindBitwiseAndAgg),
		"BIT_AND_AGG":     exp.FromArgListFunc(exp.KindBitwiseAndAgg),
		"BIT_ANDAGG":      exp.FromArgListFunc(exp.KindBitwiseAndAgg),
		"BITORAGG":        exp.FromArgListFunc(exp.KindBitwiseOrAgg),
		"BITOR_AGG":       exp.FromArgListFunc(exp.KindBitwiseOrAgg),
		"BIT_OR_AGG":      exp.FromArgListFunc(exp.KindBitwiseOrAgg),
		"BIT_ORAGG":       exp.FromArgListFunc(exp.KindBitwiseOrAgg),
		"BITXORAGG":       exp.FromArgListFunc(exp.KindBitwiseXorAgg),
		"BITXOR_AGG":      exp.FromArgListFunc(exp.KindBitwiseXorAgg),
		"BIT_XOR_AGG":     exp.FromArgListFunc(exp.KindBitwiseXorAgg),
		"BIT_XORAGG":      exp.FromArgListFunc(exp.KindBitwiseXorAgg),
		"BITMAP_OR_AGG":   exp.FromArgListFunc(exp.KindBitmapOrAgg),
		"BOOLAND":         with(exp.KindBooland, exp.Args{"round_input": true}, "this", "expression"),
		"BOOLOR":          with(exp.KindBoolor, exp.Args{"round_input": true}, "this", "expression"),
		"BOOLNOT":         with(exp.KindBoolnot, exp.Args{"round_input": true}, "this"),
		"BOOLXOR":         with(exp.KindXor, exp.Args{"round_input": true}, "this", "expression"),
		"CORR":            with(exp.KindCorr, exp.Args{"null_on_zero_variance": true}, "this", "expression"),
		"COUNT_IF":        with(exp.KindCountIf, exp.Args{"zero_on_all_null": true}, "this"),
		"DATE":            snowflakeBuildDatetime(d, "DATE", exp.DTypeDate, false),
		"DATEFROMPARTS":   with(exp.KindDateFromParts, exp.Args{"allow_overflow": true}, "year", "month", "day"),
		"DATE_FROM_PARTS": with(exp.KindDateFromParts, exp.Args{"allow_overflow": true}, "year", "month", "day"),
		"DATE_TRUNC":      snowflakeDateTruncToTime,
		"DATEADD":         datePart(exp.KindDateAdd, nil),
		"DATEDIFF":        datePart(exp.KindDateDiff, exp.Args{"date_part_boundary": true}),
		"DAYNAME":         with(exp.KindDayname, exp.Args{"abbreviated": true}, "this"),
		"DAYOFWEEKISO":    exp.FromArgListFunc(exp.KindDayOfWeekIso),
		"DIV0":            snowflakeBuildIfFromDiv0,
		"DIV0NULL":        snowflakeBuildIfFromDiv0Null,
		"EDITDISTANCE":    newArgs(exp.KindLevenshtein, "this", "expression", "max_dist"),
		"FLATTEN":         exp.FromArgListFunc(exp.KindExplode),
		"GENERATOR":       snowflakeBuildGenerator,
		"GET":             exp.FromArgListFunc(exp.KindGetExtract),
		"GETDATE":         exp.FromArgListFunc(exp.KindCurrentTimestamp),
		"GET_PATH": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindJSONExtract, exp.Args{"this": seq(args, 0), "expression": d.ToJSONPath(seq(args, 1)), "requires_json": true})
		},
		"GREATEST": func(args []exp.Expression) exp.Expression {
			return snowflakeGreatestLeast(exp.KindGreatest, args, false)
		},
		"LEAST": func(args []exp.Expression) exp.Expression {
			return snowflakeGreatestLeast(exp.KindLeast, args, false)
		},
		"GREATEST_IGNORE_NULLS": func(args []exp.Expression) exp.Expression {
			return snowflakeGreatestLeast(exp.KindGreatest, args, true)
		},
		"LEAST_IGNORE_NULLS": func(args []exp.Expression) exp.Expression {
			return snowflakeGreatestLeast(exp.KindLeast, args, true)
		},
		"LEFT":                   with(exp.KindLeft, exp.Args{"negative_length_returns_empty": true}, "this", "expression"),
		"HEX_DECODE_BINARY":      exp.FromArgListFunc(exp.KindUnhex),
		"HEX_ENCODE":             exp.FromArgListFunc(exp.KindHex),
		"IFF":                    exp.FromArgListFunc(exp.KindIf),
		"JAROWINKLER_SIMILARITY": with(exp.KindJarowinklerSimilarity, exp.Args{"case_insensitive": true, "integer_scale": true}, "this", "expression"),
		"MD5_HEX":                exp.FromArgListFunc(exp.KindMD5),
		"MD5_BINARY":             exp.FromArgListFunc(exp.KindMD5Digest),
		"MD5_NUMBER_LOWER64":     exp.FromArgListFunc(exp.KindMD5NumberLower64),
		"MD5_NUMBER_UPPER64":     exp.FromArgListFunc(exp.KindMD5NumberUpper64),
		"MONTHNAME":              with(exp.KindMonthname, exp.Args{"abbreviated": true}, "this"),
		"LAST_DAY": func(args []exp.Expression) exp.Expression {
			m := exp.Args{"this": seq(args, 0)}
			if unit := MapDatePart(seq(args, 1), nil); unit != nil {
				m["unit"] = unit
			}
			return exp.New(exp.KindLastDay, m)
		},
		"LEN":            with(exp.KindLength, exp.Args{"binary": true}, "this"),
		"LENGTH":         with(exp.KindLength, exp.Args{"binary": true}, "this"),
		"LOCALTIMESTAMP": exp.FromArgListFunc(exp.KindCurrentTimestamp),
		"NULLIFZERO":     snowflakeBuildIfFromNullIfZero,
		"OBJECT_CONSTRUCT": func(args []exp.Expression) exp.Expression {
			return SnowflakeBuildObjectConstruct(args)
		},
		"OBJECT_KEYS":        exp.FromArgListFunc(exp.KindJSONKeys),
		"OCTET_LENGTH":       exp.FromArgListFunc(exp.KindByteLength),
		"PARSE_URL":          newArgs(exp.KindParseUrl, "this", "permissive"),
		"REGEXP_EXTRACT_ALL": snowflakeBuildRegexpExtract(d, exp.KindRegexpExtractAll),
		"REGEXP_LIKE":        snowflakeBuildRegexpLike,
		"REGEXP_REPLACE": func(args []exp.Expression) exp.Expression {
			node := exp.FromArgList(exp.KindRegexpReplace, args)
			if node.Arg("replacement") == nil {
				node.Set("replacement", exp.LiteralString(""))
			}
			return node
		},
		"REGEXP_SUBSTR":     snowflakeBuildRegexpExtract(d, exp.KindRegexpExtract),
		"REGEXP_SUBSTR_ALL": snowflakeBuildRegexpExtract(d, exp.KindRegexpExtractAll),
		"RANDOM": func(args []exp.Expression) exp.Expression {
			// -2^63 and 2^63-1 as Python floats; Literal.number re-renders the negative one through
			// Decimal (core.py:1755-1764), hence the upper-case exponent.
			m := exp.Args{
				"lower": exp.Neg(exp.Args{"this": exp.Literal(exp.Args{"this": "9.223372036854776E+18", "is_string": false})}),
				"upper": exp.Literal(exp.Args{"this": "9.223372036854776e+18", "is_string": false}),
			}
			if this := seq(args, 0); this != nil {
				m["this"] = this
			}
			return exp.New(exp.KindRand, m)
		},
		"REPLACE": func(args []exp.Expression) exp.Expression {
			replacement := seq(args, 2)
			if replacement == nil {
				replacement = exp.LiteralString("")
			}
			return exp.New(exp.KindReplace, exp.Args{"this": seq(args, 0), "expression": seq(args, 1), "replacement": replacement})
		},
		"RIGHT":       with(exp.KindRight, exp.Args{"negative_length_returns_empty": true}, "this", "expression"),
		"RLIKE":       snowflakeBuildRegexpLike,
		"ROUND":       snowflakeBuildRound,
		"SHA1_BINARY": exp.FromArgListFunc(exp.KindSHA1Digest),
		"SHA1_HEX":    exp.FromArgListFunc(exp.KindSHA),
		"SHA2_BINARY": exp.FromArgListFunc(exp.KindSHA2Digest),
		"SHA2_HEX":    exp.FromArgListFunc(exp.KindSHA2),
		"SPLIT":       with(exp.KindSplit, exp.Args{"null_returns_null": true, "empty_delimiter_returns_whole": true}, "this", "expression"),
		"SQUARE": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindPow, exp.Args{"this": seq(args, 0), "expression": exp.LiteralNumber(2)})
		},
		"STDDEV_SAMP":              exp.FromArgListFunc(exp.KindStddev),
		"SYSDATE":                  with(exp.KindCurrentTimestamp, exp.Args{"sysdate": true}, "this"),
		"TABLE":                    newArgs(exp.KindTableFromRows, "this"),
		"TIMEADD":                  datePart(exp.KindTimeAdd, nil),
		"TIMEDIFF":                 datePart(exp.KindDateDiff, exp.Args{"date_part_boundary": true}),
		"TIME_FROM_PARTS":          with(exp.KindTimeFromParts, exp.Args{"overflow": true}, "hour", "min", "sec", "nano"),
		"TIMESTAMPADD":             datePart(exp.KindDateAdd, nil),
		"TIMESTAMPDIFF":            datePart(exp.KindDateDiff, exp.Args{"date_part_boundary": true}),
		"TIMESTAMPFROMPARTS":       snowflakeBuildTimestampFromParts,
		"TIMESTAMP_FROM_PARTS":     snowflakeBuildTimestampFromParts,
		"TIMESTAMPNTZFROMPARTS":    snowflakeBuildTimestampFromParts,
		"TIMESTAMP_NTZ_FROM_PARTS": snowflakeBuildTimestampFromParts,
		"TRUNC":                    trunc,
		"TRUNCATE":                 trunc,
		"TRY_DECRYPT":              with(exp.KindDecrypt, exp.Args{"safe": true}, "this", "passphrase", "aad", "encryption_method"),
		"TRY_DECRYPT_RAW":          with(exp.KindDecryptRaw, exp.Args{"safe": true}, "this", "key", "iv", "aad", "encryption_method", "aead"),
		"TRY_PARSE_JSON":           with(exp.KindParseJSON, exp.Args{"safe": true}, "this"),
		"TRY_TO_BINARY":            with(exp.KindToBinary, exp.Args{"safe": true}, "this", "format"),
		"TRY_TO_BOOLEAN":           with(exp.KindToBoolean, exp.Args{"safe": true}, "this"),
		"TRY_TO_DATE":              snowflakeBuildDatetime(d, "TRY_TO_DATE", exp.DTypeDate, true),
		"TRY_TO_DECIMAL":           toNumber(true),
		"TRY_TO_NUMBER":            toNumber(true),
		"TRY_TO_NUMERIC":           toNumber(true),
		"TRY_TO_DOUBLE":            with(exp.KindToDouble, exp.Args{"safe": true}, "this", "format"),
		"TRY_TO_FILE":              with(exp.KindToFile, exp.Args{"safe": true}, "this", "path"),
		"TRY_TO_TIME":              snowflakeBuildDatetime(d, "TRY_TO_TIME", exp.DTypeTime, true),
		"TRY_TO_TIMESTAMP":         snowflakeBuildDatetime(d, "TRY_TO_TIMESTAMP", exp.DTypeTimestamp, true),
		"TRY_TO_TIMESTAMP_LTZ":     snowflakeBuildDatetime(d, "TRY_TO_TIMESTAMP_LTZ", exp.DTypeTimestampLtz, true),
		"TRY_TO_TIMESTAMP_NTZ":     snowflakeBuildDatetime(d, "TRY_TO_TIMESTAMP_NTZ", exp.DTypeTimestampNtz, true),
		"TRY_TO_TIMESTAMP_TZ":      snowflakeBuildDatetime(d, "TRY_TO_TIMESTAMP_TZ", exp.DTypeTimestampTz, true),
		"TO_CHAR":                  toChar,
		"TO_DATE":                  snowflakeBuildDatetime(d, "TO_DATE", exp.DTypeDate, false),
		"TO_DECIMAL":               toNumber(false),
		"TO_NUMBER":                toNumber(false),
		"TO_NUMERIC":               toNumber(false),
		"TO_TIME":                  snowflakeBuildDatetime(d, "TO_TIME", exp.DTypeTime, false),
		"TO_TIMESTAMP":             snowflakeBuildDatetime(d, "TO_TIMESTAMP", exp.DTypeTimestamp, false),
		"TO_TIMESTAMP_LTZ":         snowflakeBuildDatetime(d, "TO_TIMESTAMP_LTZ", exp.DTypeTimestampLtz, false),
		"TO_TIMESTAMP_NTZ":         snowflakeBuildDatetime(d, "TO_TIMESTAMP_NTZ", exp.DTypeTimestampNtz, false),
		"TO_TIMESTAMP_TZ":          snowflakeBuildDatetime(d, "TO_TIMESTAMP_TZ", exp.DTypeTimestampTz, false),
		"TO_GEOGRAPHY":             geo("TO_GEOGRAPHY", exp.DTypeGeography),
		"TO_GEOMETRY":              geo("TO_GEOMETRY", exp.DTypeGeometry),
		"TO_VARCHAR":               toChar,
		"TO_JSON":                  exp.FromArgListFunc(exp.KindJSONFormat),
		"VECTOR_COSINE_SIMILARITY": exp.FromArgListFunc(exp.KindCosineDistance),
		"VECTOR_INNER_PRODUCT":     exp.FromArgListFunc(exp.KindDotProduct),
		"VECTOR_L1_DISTANCE":       exp.FromArgListFunc(exp.KindManhattanDistance),
		"VECTOR_L2_DISTANCE":       exp.FromArgListFunc(exp.KindEuclideanDistance),
		"ZEROIFNULL":               snowflakeBuildIfFromZeroIfNull,
		"LIKE":                     buildLike(exp.KindLike, false),
		"ILIKE":                    buildLike(exp.KindILike, false),
		"SEARCH":                   snowflakeBuildSearch,
		"SKEW":                     exp.FromArgListFunc(exp.KindSkewness),
		"SPLIT_PART":               with(exp.KindSplitPart, exp.Args{"part_index_zero_as_one": true, "empty_delimiter_returns_whole": true}, "this", "delimiter", "part_index"),
		"STRTOK": func(args []exp.Expression) exp.Expression {
			delimiter, partIndex := seq(args, 1), seq(args, 2)
			if delimiter == nil {
				delimiter = exp.LiteralString(" ")
			}
			if partIndex == nil {
				partIndex = exp.LiteralNumber(1)
			}
			return exp.New(exp.KindStrtok, exp.Args{"this": seq(args, 0), "delimiter": delimiter, "part_index": partIndex})
		},
		"STRTOK_TO_ARRAY": func(args []exp.Expression) exp.Expression {
			expression := seq(args, 1)
			if expression == nil {
				expression = exp.LiteralString(" ")
			}
			return exp.New(exp.KindStrtokToArray, exp.Args{"this": seq(args, 0), "expression": expression})
		},
		"SYSTIMESTAMP": exp.FromArgListFunc(exp.KindCurrentTimestamp),
		"IDENTIFIER":   exp.FromArgListFunc(exp.KindDynamicIdentifier),
		"UNICODE":      with(exp.KindUnicode, exp.Args{"empty_is_zero": true}, "this"),
		"WEEKISO":      exp.FromArgListFunc(exp.KindWeekOfYear),
		"WEEKOFYEAR":   exp.FromArgListFunc(exp.KindWeek),
	}
	for name, builder := range functions {
		d.Functions[name] = builder
	}
	// parser.py:193-252: the array builders read ARRAY_FUNCS_PROPAGATES_NULLS from the dialect.
	for _, name := range []string{"ARRAY_APPEND", "ARRAY_PREPEND", "ARRAY_CONCAT", "ARRAY_CAT"} {
		if base := exp.FunctionByName[name]; base != nil {
			d.Functions[name] = sparkWithNullPropagation(base)
		}
	}
	// parser.py:411-473: the Uuid builders read UUID_IS_STRING_TYPE (dialects/snowflake.py:33).
	uuid := func(args []exp.Expression) exp.Expression {
		return exp.New(exp.KindUuid, exp.Args{"is_string": true})
	}
	d.Functions["UUID"] = uuid
	d.Functions["GENERATE_UUID"] = uuid
	d.Functions["UUID_STRING"] = func(args []exp.Expression) exp.Expression {
		return exp.New(exp.KindUuid, exp.Args{"this": seq(args, 0), "name": seq(args, 1), "is_string": true})
	}
	// FUNCTION_PARSERS drops TRIM (parsers/snowflake.py:744), so TRIM reaches the base
	// FUNCTIONS entry Trim.from_arg_list.
	d.Functions["TRIM"] = exp.FromArgListFunc(exp.KindTrim)
	// parsers/snowflake.py:749.
	d.DisabledFunctions = map[string]bool{"PREDICT": true}
}

func boolArgValue(v any) bool {
	b, _ := v.(bool)
	return b
}

// snowflakeGreatestLeast ports the GREATEST/LEAST base builders (parser.py:415-424) under
// LEAST_GREATEST_IGNORES_NULLS = False, and the *_IGNORE_NULLS overlay entries.
func snowflakeGreatestLeast(kind exp.Kind, args []exp.Expression, ignoreNulls bool) exp.Expression {
	m := exp.Args{"ignore_nulls": ignoreNulls}
	if len(args) > 0 {
		m["this"] = args[0]
	}
	m["expressions"] = []exp.Expression{}
	if len(args) > 1 {
		m["expressions"] = args[1:]
	}
	return exp.New(kind, m)
}

// snowflakeBuildToNumber ports _build_to_number (parsers/snowflake.py:44-62).
func snowflakeBuildToNumber(args []exp.Expression, safe bool) exp.Expression {
	second := hiveSeqGet(args, 1)
	var format, precision, scale exp.Expression
	if second != nil && second.IsNumber() {
		precision = second
		scale = hiveSeqGet(args, 2)
		if scale == nil {
			scale = exp.LiteralNumber(0)
		}
	} else {
		format = second
		precision = hiveSeqGet(args, 2)
		if precision == nil {
			precision = exp.LiteralNumber(38)
		}
		scale = hiveSeqGet(args, 3)
		if scale == nil {
			scale = exp.LiteralNumber(0)
		}
	}
	m := exp.Args{"this": hiveSeqGet(args, 0), "precision": precision, "scale": scale, "safe": safe}
	if format != nil {
		m["format"] = format
	}
	return exp.New(exp.KindToNumber, m)
}

// snowflakeTimestampTypes ports TIMESTAMP_TYPES (parsers/snowflake.py:75-80).
var snowflakeTimestampTypes = map[exp.DType]string{
	exp.DTypeTimestamp:    "TO_TIMESTAMP",
	exp.DTypeTimestampLtz: "TO_TIMESTAMP_LTZ",
	exp.DTypeTimestampNtz: "TO_TIMESTAMP_NTZ",
	exp.DTypeTimestampTz:  "TO_TIMESTAMP_TZ",
}

// SnowflakeTimestampFunc returns the TIMESTAMP_TYPES name of dtype.
func SnowflakeTimestampFunc(dtype exp.DType) (string, bool) {
	name, ok := snowflakeTimestampTypes[dtype]
	return name, ok
}

// isPyInt ports helper.is_int: int(text) succeeds.
func isPyInt(text string) bool {
	_, ok := tokens.ParseIntPython(text, 10)
	return ok
}

// snowflakeBuildDatetime ports _build_datetime (parsers/snowflake.py:83-131).
func snowflakeBuildDatetime(d *Dialect, name string, kind exp.DType, safe bool) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		value := hiveSeqGet(args, 0)
		scaleOrFmt := hiveSeqGet(args, 1)

		intValue := value != nil && isPyInt(value.Name())
		intScaleOrFmt := scaleOrFmt != nil && scaleOrFmt.IsInt()

		if value != nil && (value.Kind() == exp.KindLiteral || value.Kind() == exp.KindNeg || scaleOrFmt != nil) {
			// Converts calls like `TO_TIME('01:02:03')` into casts
			if len(args) == 1 && value.IsString() && !intValue {
				if safe {
					cast := exp.New(exp.KindTryCast, exp.Args{"this": value, "to": kind.IntoExpr(nil), "requires_string": true})
					if kind == exp.DTypeDate {
						cast.Set("probe_date_format", true)
					}
					return cast
				}
				return exp.CastTo(value, kind)
			}

			// Handles `TO_TIMESTAMP(str, fmt)` and `TO_TIMESTAMP(num, scale)` as special
			// cases so we can transpile them, since they're relatively common
			if _, ok := snowflakeTimestampTypes[kind]; ok {
				if !safe && (intScaleOrFmt || (intValue && scaleOrFmt == nil)) {
					m := exp.Args{"this": value}
					if scaleOrFmt != nil {
						m["scale"] = scaleOrFmt
					}
					unix := exp.New(exp.KindUnixToTime, m)
					unix.Set("target_type", kind.IntoExpr(nil))
					return unix
				}
				if scaleOrFmt != nil && !intScaleOrFmt {
					strToTime := exp.New(exp.KindStrToTime, exp.Args{"this": value, "format": snowflakeParseTime(scaleOrFmt)})
					strToTime.Set("safe", safe)
					strToTime.Set("target_type", kind.IntoExpr(nil))
					return strToTime
				}
			}
		}

		// Handle DATE/TIME with format strings - allow int_value if a format string is provided
		hasFormatString := scaleOrFmt != nil && !intScaleOrFmt
		if (kind == exp.DTypeDate || kind == exp.DTypeTime) && (!intValue || hasFormatString) {
			klass := exp.KindTsOrDsToDate
			if kind == exp.DTypeTime {
				klass = exp.KindTsOrDsToTime
			}
			m := exp.Args{"this": value}
			if format := snowflakeParseTime(scaleOrFmt); format != nil {
				m["format"] = format
			}
			formatted := exp.New(klass, m)
			formatted.Set("safe", safe)
			return formatted
		}

		return exp.Anonymous(exp.Args{"this": name, "expressions": args})
	}
}

// snowflakeBuildBitwise ports _build_bitwise (parsers/snowflake.py:134-153).
func snowflakeBuildBitwise(kind exp.Kind, name string) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		if len(args) == 3 {
			// Special handling for bitwise operations with padside argument
			if kind == exp.KindBitwiseAnd || kind == exp.KindBitwiseOr || kind == exp.KindBitwiseXor {
				return exp.New(kind, exp.Args{"this": args[0], "expression": args[1], "padside": args[2]})
			}
			return exp.Anonymous(exp.Args{"this": name, "expressions": args})
		}
		result := exp.New(kind, exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1)})
		// Snowflake specifies INT128 for bitwise shifts
		if kind == exp.KindBitwiseLeftShift || kind == exp.KindBitwiseRightShift {
			result.Set("requires_int128", true)
		}
		return result
	}
}

// snowflakeBuildIfFromDiv0 ports _build_if_from_div0 (parsers/snowflake.py:157-166).
func snowflakeBuildIfFromDiv0(args []exp.Expression) exp.Expression {
	lhs := exp.Wrap(hiveSeqGet(args, 0), exp.TraitBinary)
	rhs := exp.Wrap(hiveSeqGet(args, 1), exp.TraitBinary)
	cond := exp.And_(
		exp.EQ(exp.Args{"this": rhs, "expression": exp.LiteralNumber(0)}),
		exp.Not_(exp.Is(exp.Args{"this": lhs, "expression": exp.Null()}), false),
	)
	return exp.If(exp.Args{"this": cond, "true": exp.LiteralNumber(0), "false": exp.Div(exp.Args{"this": lhs, "expression": rhs})})
}

// snowflakeBuildIfFromDiv0Null ports _build_if_from_div0null (parsers/snowflake.py:170-180).
func snowflakeBuildIfFromDiv0Null(args []exp.Expression) exp.Expression {
	lhs := exp.Wrap(hiveSeqGet(args, 0), exp.TraitBinary)
	rhs := exp.Wrap(hiveSeqGet(args, 1), exp.TraitBinary)
	// Returns 0 when divisor is 0 OR NULL
	cond := exp.Or_(
		exp.EQ(exp.Args{"this": rhs, "expression": exp.LiteralNumber(0)}),
		exp.Is(exp.Args{"this": rhs, "expression": exp.Null()}),
	)
	return exp.If(exp.Args{"this": cond, "true": exp.LiteralNumber(0), "false": exp.Div(exp.Args{"this": lhs, "expression": rhs})})
}

// snowflakeBuildIfFromZeroIfNull ports _build_if_from_zeroifnull (parsers/snowflake.py:184-186).
func snowflakeBuildIfFromZeroIfNull(args []exp.Expression) exp.Expression {
	cond := exp.Is(exp.Args{"this": hiveSeqGet(args, 0), "expression": exp.Null()})
	return exp.If(exp.Args{"this": cond, "true": exp.LiteralNumber(0), "false": hiveSeqGet(args, 0)})
}

// snowflakeBuildIfFromNullIfZero ports _build_if_from_nullifzero (parsers/snowflake.py:199-201).
func snowflakeBuildIfFromNullIfZero(args []exp.Expression) exp.Expression {
	cond := exp.EQ(exp.Args{"this": hiveSeqGet(args, 0), "expression": exp.LiteralNumber(0)})
	return exp.If(exp.Args{"this": cond, "true": exp.Null(), "false": hiveSeqGet(args, 0)})
}

// snowflakeBuildSearch ports _build_search (parsers/snowflake.py:189-195).
func snowflakeBuildSearch(args []exp.Expression) exp.Expression {
	m := exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1)}
	if len(args) > 2 {
		for _, arg := range args[2:] {
			if arg != nil && arg.Kind() == exp.KindKwarg {
				m[strings.ToLower(arg.Name())] = arg
			}
		}
	}
	return exp.New(exp.KindSearch, m)
}

// snowflakeBuildRegexpLike ports _build_regexp_like (parsers/snowflake.py:213-219).
func snowflakeBuildRegexpLike(args []exp.Expression) exp.Expression {
	m := exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1), "full_match": true}
	if flag := hiveSeqGet(args, 2); flag != nil {
		m["flag"] = flag
	}
	return exp.New(exp.KindRegexpLike, m)
}

// snowflakeDateTruncToTime ports _date_trunc_to_time (parsers/snowflake.py:222-232).
func snowflakeDateTruncToTime(args []exp.Expression) exp.Expression {
	trunc := DateTruncToTime(args)
	unit := MapDatePart(asExpr(trunc.Arg("unit")), nil)
	trunc.Set("unit", unit)
	this := trunc.This()
	isTimeInput := this != nil && this.IsType(exp.DTypeTime, exp.DTypeTimeTz)
	isDateUnit := IsDateUnit(unit)
	if (trunc.Kind() == exp.KindTimestampTrunc && isDateUnit) || isTimeInput || (trunc.Kind() == exp.KindDateTrunc && !isDateUnit) {
		trunc.Set("input_type_preserved", true)
	}
	return trunc
}

func asExpr(v any) exp.Expression {
	e, _ := v.(exp.Expression)
	return e
}

// snowflakeBuildRegexpExtract ports _build_regexp_extract (parsers/snowflake.py:235-252).
func snowflakeBuildRegexpExtract(d *Dialect, kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		group := hiveSeqGet(args, 5)
		if group == nil {
			group = exp.LiteralNumber(0)
		}
		m := exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1), "group": group}
		for i, key := range []string{"position", "occurrence", "parameters"} {
			if v := hiveSeqGet(args, i+2); v != nil {
				m[key] = v
			}
		}
		if kind == exp.KindRegexpExtract {
			m["null_if_pos_overflow"] = d.RegexpExtractPositionOverflowReturnsNull
		}
		return exp.New(kind, m)
	}
}

// snowflakeBuildTimestampFromParts ports _build_timestamp_from_parts (parsers/snowflake.py:255-264).
func snowflakeBuildTimestampFromParts(args []exp.Expression) exp.Expression {
	if len(args) == 2 {
		return exp.New(exp.KindTimestampFromParts, exp.Args{"this": args[0], "expression": args[1]})
	}
	return exp.FromArgList(exp.KindTimestampFromParts, args)
}

// snowflakeBuildRound ports _build_round (parsers/snowflake.py:267-295).
func snowflakeBuildRound(args []exp.Expression) exp.Expression {
	kwargMap := map[string]string{"EXPR": "this", "SCALE": "decimals", "ROUNDING_MODE": "truncate"}
	positionalKeys := []string{"this", "decimals", "truncate"}
	positionalIdx := 0
	m := exp.Args{}
	for _, arg := range args {
		if arg != nil && arg.Kind() == exp.KindKwarg {
			if key, ok := kwargMap[strings.ToUpper(arg.This().Name())]; ok {
				m[key] = arg.Expr()
			}
		} else if positionalIdx < len(positionalKeys) {
			m[positionalKeys[positionalIdx]] = arg
			positionalIdx++
		}
	}
	expression := exp.New(exp.KindRound, m)
	expression.Set("casts_non_integer_decimals", true)
	return expression
}

// snowflakeBuildGenerator ports _build_generator (parsers/snowflake.py:306-327).
func snowflakeBuildGenerator(args []exp.Expression) exp.Expression {
	kwargMap := map[string]string{"ROWCOUNT": "rowcount", "TIMELIMIT": "timelimit"}
	positionalKeys := []string{"rowcount", "timelimit"}
	m := exp.Args{}
	for i, arg := range args {
		if arg != nil && arg.Kind() == exp.KindKwarg {
			if key, ok := kwargMap[strings.ToUpper(arg.This().Name())]; ok {
				m[key] = arg.Expr()
			}
		} else if i < len(positionalKeys) {
			m[positionalKeys[i]] = arg
		}
	}
	return exp.New(exp.KindGenerator, m)
}

// SnowflakeArrayGenerateRange ports the ARRAY_GENERATE_RANGE builder (parsers/snowflake.py:
// 422-428): Snowflake has exclusive end semantics.
func SnowflakeArrayGenerateRange(args []exp.Expression) exp.Expression {
	m := exp.Args{"start": hiveSeqGet(args, 0), "end": hiveSeqGet(args, 1), "is_end_exclusive": true}
	if step := hiveSeqGet(args, 2); step != nil {
		m["step"] = step
	}
	return exp.New(exp.KindGenerateSeries, m)
}

// SnowflakeBuildObjectConstruct ports build_object_construct (parsers/snowflake.py:1433-1445).
func SnowflakeBuildObjectConstruct(args []exp.Expression) exp.Expression {
	expression := hiveBuildVarMap(args)
	if expression.Kind() == exp.KindStarMap {
		return expression
	}
	keys := asExpr(expression.Arg("keys")).Expressions()
	values := asExpr(expression.Arg("values")).Expressions()
	properties := make([]exp.Expression, 0, len(keys))
	for i := range keys {
		if i >= len(values) {
			break
		}
		properties = append(properties, exp.PropertyEQ(exp.Args{"this": keys[i], "expression": values[i]}))
	}
	return exp.New(exp.KindStruct, exp.Args{"expressions": properties})
}

// snowflakeBuildTimeToStrOrToChar ports build_timetostr_or_tochar (dialect.py:2586-2599).
func snowflakeBuildTimeToStrOrToChar(d *Dialect, args []exp.Expression) exp.Expression {
	if len(args) == 2 {
		this := args[0]
		if this != nil && this.Type() == nil {
			annotateTypes(this, d)
		}
		if this != nil && this.IsType(typeSet(exp.TemporalTypes)...) {
			format := snowflakeParseTime(args[1])
			return exp.New(exp.KindTimeToStr, exp.Args{"this": this, "format": format})
		}
	}
	return exp.FromArgList(exp.KindToChar, args)
}

// snowflakeTimeMapping ports TIME_MAPPING (dialects/snowflake.py:52-118) in declaration order, so
// the inverse is built last-wins like upstream's dict comprehension.
var snowflakeTimeMapping = [][2]string{
	{"YYYY", "%Y"}, {"yyyy", "%Y"}, {"YY", "%y"}, {"yy", "%y"}, {"MMMM", "%B"}, {"mmmm", "%B"},
	{"MON", "%b"}, {"mon", "%b"}, {"MM", "%m"}, {"mm", "%m"}, {"DD", "%d"}, {"dd", "%-d"},
	{"DY", "%a"}, {"dy", "%w"}, {"HH24", "%H"}, {"hh24", "%H"}, {"HH12", "%I"}, {"hh12", "%I"},
	{"MI", "%M"}, {"mi", "%M"}, {"SS", "%S"}, {"ss", "%S"}, {"FF", "%f_nine"}, {"ff", "%f_nine"},
	{"FF0", "%f_zero"}, {"ff0", "%f_zero"}, {"FF1", "%f_one"}, {"ff1", "%f_one"},
	{"FF2", "%f_two"}, {"ff2", "%f_two"}, {"FF3", "%f_three"}, {"ff3", "%f_three"},
	{"FF4", "%f_four"}, {"ff4", "%f_four"}, {"FF5", "%f_five"}, {"ff5", "%f_five"},
	{"FF6", "%f"}, {"ff6", "%f"}, {"FF7", "%f_seven"}, {"ff7", "%f_seven"},
	{"FF8", "%f_eight"}, {"ff8", "%f_eight"}, {"FF9", "%f_nine"}, {"ff9", "%f_nine"},
	{"TZHTZM", "%z"}, {"tzhtzm", "%z"}, {"TZH:TZM", "%:z"}, {"tzh:tzm", "%:z"},
	{"TZH", "%-z"}, {"tzh", "%-z"}, {`"T"`, "T"}, {"AM", "%p"}, {"am", "%p"}, {"PM", "%p"},
	{"pm", "%p"},
}

var snowflakeForwardTimeMapping = func() map[string]string {
	m := map[string]string{}
	for _, kv := range snowflakeTimeMapping {
		m[kv[0]] = kv[1]
	}
	return m
}()

// snowflakeInverseTimeMapping merges the class-defined INVERSE_TIME_MAPPING {"T": "T"} over the
// inverted TIME_MAPPING, then applies _with_strict_time_inverse (dialect.py:152-161, 281-284).
var snowflakeInverseTimeMapping = func() map[string]string {
	m := map[string]string{}
	for _, kv := range snowflakeTimeMapping {
		m[kv[1]] = kv[0]
	}
	m["T"] = "T"
	for _, kv := range strictTimeFormats {
		strict, lax := kv[0], kv[1]
		if v, ok := m[strict]; ok {
			if _, has := m[lax]; !has {
				m[lax] = v
			}
		} else if v, ok := m[lax]; ok {
			m[strict] = v
		} else {
			m[strict] = lax
		}
	}
	return m
}()

var snowflakeForwardTimeTrie = prestoTimeTrie(snowflakeForwardTimeMapping)
var snowflakeInverseTimeTrie = prestoTimeTrie(snowflakeInverseTimeMapping)

// SnowflakeFormatTime ports Generator.format_time under Snowflake's INVERSE_TIME_MAPPING.
func SnowflakeFormatTime(format string) string {
	converted, _ := prestoConvertTimeFormatWith(format, snowflakeInverseTimeMapping, snowflakeInverseTimeTrie)
	return converted
}

// snowflakeParseTime ports Dialect.format_time under Snowflake's TIME_MAPPING (dialect.py:
// 1110-1129): a string literal is converted, anything else passes through.
func snowflakeParseTime(format exp.Expression) exp.Expression {
	if format == nil || !format.IsString() {
		return format
	}
	converted, ok := prestoConvertTimeFormatWith(format.Name(), snowflakeForwardTimeMapping, snowflakeForwardTimeTrie)
	if !ok {
		converted = "None"
	}
	return exp.LiteralString(converted)
}
