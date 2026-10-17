package generator

import (
	"strconv"
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// postgresDateDiffFactor ports DATE_DIFF_FACTOR (generators/postgres.py:45-52).
var postgresDateDiffFactor = map[string]string{
	"MICROSECOND": " * 1000000", "MILLISECOND": " * 1000", "SECOND": "", "MINUTE": " / 60", "HOUR": " / 3600", "DAY": " / 86400",
}

// Postgres overrides follow generators/postgres.py TRANSFORMS and *_sql methods; the shared
// tables already carry the postgres branches written before per-dialect dispatch existed.
func init() {
	castTo := func(e exp.Expression, t exp.DType) exp.Expression {
		return exp.New(exp.KindCast, exp.Args{"this": e, "to": t.IntoExpr(nil)})
	}
	rename := func(name string) func(*Generator, exp.Expression) string {
		return func(g *Generator, e exp.Expression) string {
			return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
		}
	}
	noNulls := func(message string) func(*Generator, exp.Expression) string {
		return func(g *Generator, e exp.Expression) string {
			g.unsupported(message)
			return g.sqlKey(e, "this")
		}
	}
	withinGroup := func(g *Generator, e exp.Expression) string {
		if out := transforms.AddWithinGroupForPercentiles(e); out.Kind() == exp.KindWithinGroup {
			return g.gen(out)
		}
		return g.functionFallbackSQL(e)
	}
	table := map[exp.Kind]func(*Generator, exp.Expression) string{
		// generators/postgres.py:348-356.
		exp.KindJSONExtract: func(g *Generator, e exp.Expression) string {
			return g.postgresJSONExtractSQL(e, "JSON_EXTRACT_PATH", "->")
		},
		exp.KindJSONExtractScalar: func(g *Generator, e exp.Expression) string {
			return g.postgresJSONExtractSQL(e, "JSON_EXTRACT_PATH_TEXT", "->>")
		},
		exp.KindJSONPathKey:       (*Generator).jsonPathKeyOnlyName,
		exp.KindJSONPathRoot:      func(*Generator, exp.Expression) string { return "" },
		exp.KindJSONPathSubscript: func(g *Generator, e exp.Expression) string { return g.jsonPathPart(e.Arg("this")) },
		// generators/postgres.py:335
		exp.KindExplode: rename("UNNEST"),
		// arraycontains_sql (generators/postgres.py:576-600).
		exp.KindArrayContains: func(g *Generator, e exp.Expression) string {
			value, array := e.Expr(), e.This()
			coalesce := exp.New(exp.KindCoalesce, exp.Args{
				"this":        exp.EQ(exp.Args{"this": value.Copy(), "expression": exp.Any(exp.Args{"this": exp.Paren(exp.Args{"this": array})})}),
				"expressions": []exp.Expression{exp.Boolean(exp.Args{"this": false})},
			})
			return g.gen(exp.Case(exp.Args{
				"ifs":     []exp.Expression{exp.If(exp.Args{"this": exp.Is(exp.Args{"this": value, "expression": exp.Null()}), "true": exp.Null()})},
				"default": coalesce,
			}))
		},
		// generators/postgres.py:408
		exp.KindUuid:   func(*Generator, exp.Expression) string { return "GEN_RANDOM_UUID()" },
		exp.KindGetbit: (*Generator).getbitSQL,
		exp.KindRound:  (*Generator).postgresRoundSQL,
		exp.KindDay:    (*Generator).postgresDayMonthYearSQL,
		exp.KindMonth:  (*Generator).postgresDayMonthYearSQL,
		exp.KindYear:   (*Generator).postgresDayMonthYearSQL,
		// generators/postgres.py:168-180 _unix_to_time_sql
		exp.KindUnixToTime: func(g *Generator, e exp.Expression) string {
			scale := asExpression(e.Arg("scale"))
			var timestamp exp.Expression = e.This()
			if scale != nil && !(scale.IsNumber() && scale.Name() == "0") {
				pow := exp.FunctionByName["POW"]([]exp.Expression{exp.LiteralNumber(10), scale})
				timestamp = exp.New(exp.KindDiv, exp.Args{"this": timestamp, "expression": pow})
			}
			return g.funcCall("TO_TIMESTAMP", []any{timestamp, g.formatTime(e)}, "(", ")", true)
		},
		// generators/postgres.py:397 timestamptrunc_sql(zone=True) (dialect.py:1700-1709)
		exp.KindTimestampTrunc: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_TRUNC", []any{unitToStr(e), e.This(), e.Arg("zone")}, "(", ")", true)
		},
		exp.KindArrayFilter:        (*Generator).filterArrayUsingUnnest,
		exp.KindArrayAppend:        arrayAppendSQLFunc("ARRAY_APPEND", false),
		exp.KindArrayPrepend:       arrayAppendSQLFunc("ARRAY_PREPEND", true),
		exp.KindArrayConcat:        arrayConcatSQL("ARRAY_CAT", false),
		exp.KindTimestampFromParts: prestoRename("MAKE_TIMESTAMP"),
		exp.KindTimeFromParts:      prestoRename("MAKE_TIME"),
		exp.KindBitwiseAndAgg:      prestoRename("BIT_AND"),
		exp.KindBitwiseOrAgg:       prestoRename("BIT_OR"),
		exp.KindBitwiseXorAgg:      prestoRename("BIT_XOR"),
		exp.KindBitwiseXor:         func(g *Generator, e exp.Expression) string { return g.binary(e, "#") },
		exp.KindPivot:              noPivotSQL,
		exp.KindLastDay: func(g *Generator, e exp.Expression) string {
			// no_last_day_sql (dialects/dialect.py:2161-2166)
			truncCurrDate := exp.FunctionByName["DATE_TRUNC"]([]exp.Expression{exp.LiteralString("month"), e.This()})
			plusOneMonth := exp.FunctionByName["DATE_ADD"]([]exp.Expression{truncCurrDate, exp.LiteralNumber(1), exp.LiteralString("month")})
			minusOneDay := exp.FunctionByName["DATE_SUB"]([]exp.Expression{plusOneMonth, exp.LiteralNumber(1), exp.LiteralString("day")})
			return g.gen(exp.CastTo(minusOneDay, exp.DTypeDate))
		},
		exp.KindBracket: func(g *Generator, e exp.Expression) string {
			// generators/postgres.py:480-485: ARRAY[1, 2, 3][3] is not allowed; wrap the array.
			if this := e.This(); this != nil && this.Kind() == exp.KindArray {
				e.Set("this", exp.Paren(exp.Args{"this": this}))
			}
			return g.bracketSQL(e)
		},
		exp.KindCountIf: func(g *Generator, e exp.Expression) string {
			// dialect.py:1812-1819 count_if_to_sum
			cond := e.This()
			if cond != nil && cond.Kind() == exp.KindDistinct {
				cond = cond.Expressions()[0]
				g.unsupported("DISTINCT is not supported when converting COUNT_IF to SUM")
			}
			return g.funcCall("SUM", []any{exp.If(exp.Args{"this": cond, "true": exp.LiteralNumber(1), "false": exp.LiteralNumber(0)})}, "(", ")", true)
		},
		exp.KindDateDiff:   (*Generator).postgresDateDiffSQL,
		exp.KindTsOrDsDiff: (*Generator).postgresDateDiffSQL,
		exp.KindDateSub:    func(g *Generator, e exp.Expression) string { return g.postgresDateAddSQL("-", e) },
		exp.KindTsOrDsAdd:  func(g *Generator, e exp.Expression) string { return g.postgresDateAddSQL("+", e) },
		exp.KindGenerateSeries: func(g *Generator, e exp.Expression) string {
			// dialect.py:1420-1436 generate_series_sql("GENERATE_SERIES")
			end := asExpression(e.Arg("end"))
			if boolValue(e.Arg("is_end_exclusive")) && end != nil {
				end = exp.Sub(exp.Args{"this": end, "expression": exp.LiteralNumber(1)})
			}
			return g.funcCall("GENERATE_SERIES", []any{e.Arg("start"), end, e.Arg("step")}, "(", ")", true)
		},
		exp.KindIgnoreNulls:  noNulls("PostgreSQL does not support IGNORE NULLS."),
		exp.KindRespectNulls: noNulls("PostgreSQL does not support RESPECT NULLS."),
		exp.KindInterval: func(g *Generator, e exp.Expression) string {
			// generators/postgres.py:559-567: quarters become months.
			if this := e.This(); this != nil && this.Kind() == exp.KindLiteral && strings.HasPrefix(strings.ToLower(e.Text("unit")), "quarter") {
				if n, err := strconv.Atoi(this.Name()); err == nil {
					this.Replace(exp.LiteralString(strconv.Itoa(n * 3)))
					asExpression(e.Arg("unit")).Replace(exp.Var(exp.Args{"this": "MONTH"}))
				}
			}
			return g.intervalSQL(e)
		},
		exp.KindIsAscii: func(g *Generator, e exp.Expression) string {
			return "(" + g.sqlKey(e, "this") + " ~ '^[[:ascii:]]*$')"
		},
		exp.KindLevenshtein: func(g *Generator, e exp.Expression) string {
			name := "LEVENSHTEIN"
			if e.Arg("max_dist") != nil {
				name = "LEVENSHTEIN_LESS_EQUAL"
			}
			return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
		},
		exp.KindMapFromEntries: func(g *Generator, e exp.Expression) string {
			g.unsupported("MAP_FROM_ENTRIES unsupported")
			return ""
		},
		exp.KindPercentileCont: withinGroup,
		exp.KindPercentileDisc: withinGroup,
		exp.KindIntDiv:         prestoRename("DIV"),
		exp.KindRand:           rename("RANDOM"),
		exp.KindSelect: func(g *Generator, e exp.Expression) string {
			// generators/postgres.py:382-387 Select preprocess chain.
			out := transforms.EliminateSemiAndAntiJoins(e)
			out = transforms.EliminateQualify(out)
			if out.Kind() != exp.KindSelect {
				return g.gen(out)
			}
			return g.selectSQL(out)
		},
		exp.KindStrPosition: func(g *Generator, e exp.Expression) string {
			return g.strPositionSQLWith(e, "POSITION", false, false)
		},
		exp.KindStrToDate: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_DATE", []any{e.This(), g.formatTime(e)}, "(", ")", true)
		},
		exp.KindStrToTime: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_TIMESTAMP", []any{e.This(), g.formatTime(e)}, "(", ")", true)
		},
		exp.KindTimeToStr: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_CHAR", []any{e.This(), g.formatTime(e)}, "(", ")", true)
		},
		exp.KindTimeToUnix: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_PART", []any{exp.LiteralString("epoch"), e.This()}, "(", ")", true)
		},
		exp.KindXor: func(g *Generator, e exp.Expression) string {
			// dialect.py:1952-1955 bool_xor_sql
			a, b := g.sqlKey(e, "this"), g.sqlKey(e, "expression")
			return "(" + a + " AND (NOT " + b + ")) OR ((NOT " + a + ") AND " + b + ")"
		},
		exp.KindUnnest: func(g *Generator, e exp.Expression) string {
			// generators/postgres.py:448-477
			exprs := e.Expressions()
			if len(exprs) != 1 {
				return g.unnestSQL(e)
			}
			arg := exprs[0]
			if arg.Kind() != exp.KindGenerateDateArray {
				this := g.annotateTypes(arg)
				if this.IsType("array<json>") {
					for this.Kind() == exp.KindCast {
						this = this.This()
					}

					argAsJSON := g.gen(exp.CastTo(this, exp.DTypeJSON))
					alias := g.sqlKey(e, "alias")
					if alias != "" {
						alias = " AS " + alias
					}

					if truthy(e.Arg("offset")) {
						g.unsupported("Unsupported JSON_ARRAY_ELEMENTS with offset")
					}

					return "JSON_ARRAY_ELEMENTS(" + argAsJSON + ")" + alias
				}
				return g.unnestSQL(e)
			}
			series := exp.New(exp.KindGenerateSeries, exp.Args{"start": arg.Arg("start"), "end": arg.Arg("end"), "step": arg.Arg("step")})
			if p := e.Parent(); p != nil && (p.Kind() == exp.KindFrom || p.Kind() == exp.KindJoin) {
				alias := asExpression(e.Arg("alias"))
				if alias == nil {
					alias = exp.TableAlias(exp.Args{"this": exp.ToIdentifier("_unnested_generate_series")})
				}
				tableAlias := exp.TableAlias(exp.Args{"this": exp.ToIdentifier("_t"), "columns": []exp.Expression{exp.ToIdentifier("value")}})
				sel := exp.Select(exp.Args{
					"expressions": []exp.Expression{castTo(exp.Column(exp.Args{"this": exp.ToIdentifier("value")}), exp.DTypeDate)},
					"from_":       exp.From(exp.Args{"this": exp.Table(exp.Args{"this": series, "alias": tableAlias})}),
				})
				return g.gen(exp.Subquery(exp.Args{"this": sel, "alias": alias}))
			}
			return g.gen(series)
		},
	}
	registerDialectDispatch("postgres", table)
	// generators/postgres.py:283-287.
	registerJSONPathParts("postgres", exp.KindJSONPathKey, exp.KindJSONPathRoot, exp.KindJSONPathSubscript)
}

// getbitSQL ports getbit_sql (dialect.py:2626-2644): an integer-typed Getbit whose zero index is
// the least-significant bit becomes bitwise operations.
func (g *Generator) getbitSQL(e exp.Expression) string {
	value, position := e.This(), e.Expr()

	if !truthy(e.Arg("zero_is_msb")) && e.IsType(append(typeArgs(exp.SignedIntegerTypes), typeArgs(exp.UnsignedIntegerTypes)...)...) {
		// Use bitwise operations: (value >> position) & 1
		shifted := exp.New(exp.KindBitwiseRightShift, exp.Args{"this": value, "expression": position})
		return g.gen(exp.New(exp.KindBitwiseAnd, exp.Args{"this": shifted, "expression": exp.LiteralNumber(1)}))
	}

	return g.funcCall("GET_BIT", []any{value, position}, "(", ")", true)
}

// postgresRoundSQL ports _round_sql (generators/postgres.py:230-248).
func (g *Generator) postgresRoundSQL(e exp.Expression) string {
	this := g.sqlKey(e, "this")
	decimals := g.sqlKey(e, "decimals")

	if decimals == "" {
		return g.funcCall("ROUND", []any{this}, "(", ")", true)
	}

	if e.Type() == nil {
		e = g.annotateTypes(e)
	}

	// ROUND(double precision, integer) is not permitted in Postgres
	// so it's necessary to cast to decimal before rounding.
	if e.This().IsType(exp.DTypeDouble) {
		decimalType := exp.DTypeDecimal.IntoExpr(exp.Args{"expressions": e.Expressions()})
		this = g.gen(exp.Cast(exp.Args{"this": exp.Var(exp.Args{"this": this}), "to": decimalType}))
	}

	return g.funcCall("ROUND", []any{this, decimals}, "(", ")", true)
}

// postgresDayMonthYearSQL ports _day_month_year_sql (generators/postgres.py:80-89).
func (g *Generator) postgresDayMonthYearSQL(e exp.Expression) string {
	this := e.This()
	value := this
	if this != nil && this.Kind() == exp.KindTsOrDsToDate {
		value = this.This()
	}

	if value != nil && value.IsType(typeArgs(exp.IntegerTypes)...) {
		if defaultDate := asExpression(this.Arg("default_date")); defaultDate != nil {
			this = exp.BinOp(exp.KindAdd, exp.CastTo(defaultDate, exp.DTypeDate), value)
		}
	}

	name := map[exp.Kind]string{exp.KindDay: "DAY", exp.KindMonth: "MONTH", exp.KindYear: "YEAR"}[e.Kind()]
	return g.gen(exp.Extract(exp.Args{"this": exp.Var(exp.Args{"this": name}), "expression": this}))
}

// postgresDateDiffSQL ports _date_diff_sql (generators/postgres.py:94-125).
func (g *Generator) postgresDateDiffSQL(e exp.Expression) string {
	unit := strings.ToUpper(e.Text("unit"))
	if unit == "" {
		unit = "DAY"
	}
	if unit == "DAY" && boolValue(e.Arg("date_part_boundary")) {
		this := exp.New(exp.KindCast, exp.Args{"this": e.This(), "to": exp.DTypeDate.IntoExpr(nil)})
		expr := exp.New(exp.KindCast, exp.Args{"this": e.Expr(), "to": exp.DTypeDate.IntoExpr(nil)})
		return g.gen(exp.Paren(exp.Args{"this": exp.Sub(exp.Args{"this": this, "expression": expr})}))
	}
	end := "CAST(" + g.sqlKey(e, "this") + " AS TIMESTAMP)"
	start := "CAST(" + g.sqlKey(e, "expression") + " AS TIMESTAMP)"
	if factor, ok := postgresDateDiffFactor[unit]; ok {
		return "CAST(EXTRACT(epoch FROM " + end + " - " + start + ")" + factor + " AS BIGINT)"
	}
	age := "AGE(" + end + ", " + start + ")"
	switch unit {
	case "WEEK":
		unit = "EXTRACT(days FROM (" + end + " - " + start + ")) / 7"
	case "MONTH":
		unit = "EXTRACT(year FROM " + age + ") * 12 + EXTRACT(month FROM " + age + ")"
	case "QUARTER":
		unit = "EXTRACT(year FROM " + age + ") * 4 + EXTRACT(month FROM " + age + ") / 3"
	case "YEAR":
		unit = "EXTRACT(year FROM " + age + ")"
	default:
		unit = age
	}
	return "CAST(" + unit + " AS BIGINT)"
}

// filterArrayUsingUnnest ports filter_array_using_unnest (dialect.py:2287-2305) for ArrayFilter.
func (g *Generator) filterArrayUsingUnnest(e exp.Expression) string {
	cond := e.Expr()
	var alias exp.Expression
	switch {
	case cond != nil && cond.Kind() == exp.KindLambda && len(cond.Expressions()) == 1:
		alias = cond.Expressions()[0]
		cond = cond.This()
	case cond != nil && cond.Is(exp.TraitPredicate):
		alias = exp.ToIdentifier("_u")
	default:
		g.unsupported("Unsupported filter condition")
		return ""
	}
	unnest := exp.Unnest(exp.Args{
		"expressions": []exp.Expression{e.This()},
		"alias":       exp.TableAlias(exp.Args{"columns": []exp.Expression{alias.Copy()}}),
	})
	filtered := exp.Select(exp.Args{
		"expressions": []exp.Expression{alias.Copy()},
		"from_":       exp.From(exp.Args{"this": unnest}),
		"where":       exp.Where(exp.Args{"this": cond}),
	})
	return g.gen(exp.New(exp.KindArray, exp.Args{"expressions": []exp.Expression{filtered}}))
}

// generators/postgres.py TRANSFORMS reached from Snowflake sources.
func init() {
	registerDialectDispatch("postgres", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindCurrentVersion: prestoRename("VERSION"),
		exp.KindLogicalAnd:     prestoRename("BOOL_AND"),
		exp.KindRegexpReplace: func(g *Generator, e exp.Expression) string {
			return g.funcCall("REGEXP_REPLACE", []any{e.This(), e.Expr(), e.Arg("replacement"), e.Arg("position"), e.Arg("occurrence"), regexpReplaceGlobalModifier(e)}, "(", ")", true)
		},
		exp.KindToChar: func(g *Generator, e exp.Expression) string {
			if e.Arg("format") != nil {
				return g.functionFallbackSQL(e)
			}
			return g.gen(exp.CastTo(e.This(), exp.DTypeText))
		},
	})
	registerDialectPropertyLocations("postgres", map[exp.Kind]propertyLocation{
		exp.KindTransientProperty: propertyLocationUnsupported,
	})
}

// regexpReplaceGlobalModifier ports regexp_replace_global_modifier (dialect.py:2610-2623).
func regexpReplaceGlobalModifier(e exp.Expression) exp.Expression {
	modifiers := asExpression(e.Arg("modifiers"))
	occurrence := asExpression(e.Arg("occurrence"))
	if !truthy(e.Arg("single_replace")) && (occurrence == nil || (occurrence.IsInt() && occurrence.Name() == "0")) {
		if modifiers == nil || modifiers.IsString() {
			// Append 'g' to the modifiers if they are not provided since
			// the semantics of REGEXP_REPLACE from the input dialect
			// is to replace all occurrences of the pattern.
			value := ""
			if modifiers != nil {
				value = modifiers.Name()
			}
			modifiers = exp.LiteralString(value + "g")
		}
	}
	return modifiers
}

// noPivotSQL ports no_pivot_sql (dialects/dialect.py:1296-1298).
func noPivotSQL(g *Generator, e exp.Expression) string {
	g.unsupported("PIVOT unsupported")
	return ""
}
