package generator

import (
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/transforms"
)

// Presto overrides continued from dialect_presto.go: the generators/presto.py TRANSFORMS entries
// that became reachable once the base parser builds typed function nodes.
func init() {
	castTo := func(e exp.Expression, t exp.DType) exp.Expression {
		return exp.New(exp.KindCast, exp.Args{"this": e, "to": t.IntoExpr(nil)})
	}
	table := map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindArrayAny:      prestoRename("ANY_MATCH"),
		exp.KindArrayToString: prestoRename("ARRAY_JOIN"),
		exp.KindArrayConcat:   prestoRename("CONCAT"),
		exp.KindDateStrToDate: func(g *Generator, e exp.Expression) string { return g.gen(castTo(e.This(), exp.DTypeDate)) },
		exp.KindDateToDi: func(g *Generator, e exp.Expression) string {
			return "CAST(DATE_FORMAT(" + g.sqlKey(e, "this") + ", '%Y%m%d') AS INT)"
		},
		exp.KindDiToDate: func(g *Generator, e exp.Expression) string {
			return "CAST(DATE_PARSE(CAST(" + g.sqlKey(e, "this") + " AS VARCHAR), '%Y%m%d') AS DATE)"
		},
		exp.KindTsOrDiToDi: func(g *Generator, e exp.Expression) string {
			return "CAST(SUBSTR(REPLACE(CAST(" + g.sqlKey(e, "this") + " AS VARCHAR), '-', ''), 1, 8) AS INT)"
		},
		exp.KindDateSub: func(g *Generator, e exp.Expression) string { return g.prestoDateDeltaSQL(e, true) },
		exp.KindLevenshtein: func(g *Generator, e exp.Expression) string {
			g.unsupportedArgs(e, "ins_cost", "del_cost", "sub_cost", "max_dist")
			return g.funcCall("LEVENSHTEIN_DISTANCE", []any{e.This(), e.Expr()}, "(", ")", true)
		},
		exp.KindILike: (*Generator).noILikeSQL,
		exp.KindLeft:  (*Generator).leftToSubstringSQL,
		exp.KindRight: (*Generator).rightToSubstringSQL,
		exp.KindSortArray: func(g *Generator, e exp.Expression) string {
			// presto.py:60-66 _no_sort_array
			var comparator any
			if asc := asExpression(e.Arg("asc")); asc != nil && asc.Kind() == exp.KindBoolean && !boolValue(asc.Arg("this")) {
				comparator = "(a, b) -> CASE WHEN a < b THEN 1 WHEN a > b THEN -1 ELSE 0 END"
			}
			return g.funcCall("ARRAY_SORT", []any{e.This(), comparator}, "(", ")", true)
		},
		exp.KindTimeStrToDate: func(g *Generator, e exp.Expression) string { return timeStrToTimeSQL(g, e, false) },
		exp.KindTimeStrToTime: func(g *Generator, e exp.Expression) string { return timeStrToTimeSQL(g, e, false) },
		exp.KindTimeStrToUnix: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_UNIXTIME", []any{g.funcCall("DATE_PARSE", []any{e.This(), exp.LiteralString(dialects.PrestoTimeFormat)}, "(", ")", true)}, "(", ")", true)
		},
		exp.KindUnixToStr: func(g *Generator, e exp.Expression) string {
			return "DATE_FORMAT(FROM_UNIXTIME(" + g.sqlKey(e, "this") + "), " + g.gen(prestoFormatTime(e)) + ")"
		},
		exp.KindUnixToTimeStr: func(g *Generator, e exp.Expression) string {
			return "CAST(FROM_UNIXTIME(" + g.sqlKey(e, "this") + ") AS VARCHAR)"
		},
		exp.KindUnixToTime: func(g *Generator, e exp.Expression) string {
			// presto.py:138-144 _unix_to_time_sql
			scale := asExpression(e.Arg("scale"))
			if scale == nil || (scale.IsNumber() && scale.Name() == "0") {
				return g.funcCall("FROM_UNIXTIME", []any{e.Arg("this"), e.Arg("zone"), e.Arg("hours"), e.Arg("minutes")}, "(", ")", true)
			}
			return "FROM_UNIXTIME(CAST(" + g.sqlKey(e, "this") + " AS DOUBLE) / POW(10, " + g.gen(scale) + "))"
		},
		exp.KindTsOrDsAdd: func(g *Generator, e exp.Expression) string {
			// presto.py:113-116 with ts_or_ds_add_cast (dialect.py:1984-1994)
			returnType := asExpression(e.Arg("return_type"))
			if returnType == nil {
				returnType = exp.DTypeDate.IntoExpr(nil)
			}
			this := e.This()
			if exp.IsType(returnType, exp.DTypeDate) {
				this = castTo(this, exp.DTypeTimestamp)
			}
			this = exp.New(exp.KindCast, exp.Args{"this": this, "to": returnType})
			return g.funcCall("DATE_ADD", []any{prestoUnit(e), e.Expr(), this}, "(", ")", true)
		},
		exp.KindTsOrDsDiff: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_DIFF", []any{prestoUnit(e), castTo(e.Expr(), exp.DTypeTimestamp), castTo(e.This(), exp.DTypeTimestamp)}, "(", ")", true)
		},
		exp.KindTsOrDsToDate: func(g *Generator, e exp.Expression) string {
			// presto.py:105-111 _ts_or_ds_to_date_sql
			if format, ok := prestoFormatTime(e).(exp.Expression); ok && format != nil && format.Name() != "" && format.Name() != dialects.PrestoTimeFormat && format.Name() != "%Y-%m-%d" {
				return "CAST(" + g.prestoStrToTimeSQL(e) + " AS DATE)"
			}
			return g.gen(castTo(castTo(e.This(), exp.DTypeTimestamp), exp.DTypeDate))
		},
		exp.KindStrPosition: func(g *Generator, e exp.Expression) string {
			return g.strPositionSQLWith(e, "STRPOS", false, true)
		},
		exp.KindStrToUnix:         (*Generator).prestoStrToUnixSQL,
		exp.KindGenerateSeries:    (*Generator).sequenceSQL,
		exp.KindGenerateDateArray: (*Generator).sequenceSQL,
		exp.KindQuantile: func(g *Generator, e exp.Expression) string {
			g.unsupported("Presto does not support exact quantiles")
			return g.funcCall("APPROX_PERCENTILE", []any{e.This(), e.Arg("quantile")}, "(", ")", true)
		},
		exp.KindWithinGroup: func(g *Generator, e exp.Expression) string {
			if out := transforms.RemoveWithinGroupForPercentiles(e); out.Kind() != exp.KindWithinGroup {
				return g.gen(out)
			}
			return g.withinGroupSQL(e)
		},
		exp.KindLateral: (*Generator).prestoExplodeToUnnestSQL,
		exp.KindTable: func(g *Generator, e exp.Expression) string {
			if out := transforms.UnnestGenerateSeries(e); out.Kind() != exp.KindTable {
				return g.gen(out)
			}
			return g.tableSQL(e)
		},
		exp.KindSelect: func(g *Generator, e exp.Expression) string {
			// generators/presto.py:381-392; eliminate_window_clause, explode_projection_to_unnest
			// and amend_exploded_column_table are not ported yet.
			out := transforms.EliminateQualify(e)
			out = transforms.EliminateDistinctOn(out)
			out = transforms.EliminateSemiAndAntiJoins(out)
			if out.Kind() != exp.KindSelect {
				return g.gen(out)
			}
			return g.selectSQL(out)
		},
	}
	registerDialectDispatch("presto", table)
}

// prestoDateDeltaSQL ports _date_delta_sql (presto.py:157-168); the _to_int cast is the
// prestoIntegerTyped heuristic of prestoDateAddSQL.
func (g *Generator) prestoDateDeltaSQL(e exp.Expression, negate bool) string {
	var interval any = e.Expr()
	if iv := asExpression(interval); iv != nil && !prestoIntegerTyped(iv) {
		interval = exp.New(exp.KindCast, exp.Args{"this": iv, "to": exp.DTypeBigInt.IntoExpr(nil)})
	}
	if negate {
		interval = exp.Mul(exp.Args{"this": interval, "expression": exp.LiteralNumber(-1)})
	}
	return g.funcCall("DATE_ADD", []any{prestoUnit(e), interval, e.This()}, "(", ")", true)
}

// prestoStrToUnixSQL ports strtounix_sql (presto.py:565-584).
func (g *Generator) prestoStrToUnixSQL(e exp.Expression) string {
	this := e.This()
	valueAsText := exp.New(exp.KindCast, exp.Args{"this": this.Copy(), "to": exp.DTypeText.IntoExpr(nil)})
	var valueAsTimestamp exp.Expression = this
	if this.IsString() {
		valueAsTimestamp = exp.New(exp.KindCast, exp.Args{"this": this.Copy(), "to": exp.DTypeTimestamp.IntoExpr(nil)})
	}
	format := prestoFormatTime(e)
	parseWithoutTz := g.funcCall("DATE_PARSE", []any{valueAsText, format}, "(", ")", true)
	formatted := g.funcCall("DATE_FORMAT", []any{valueAsTimestamp, format}, "(", ")", true)
	var hiveFormat any
	if f := asExpression(e.Arg("format")); f != nil && f.IsString() {
		hiveFormat = exp.LiteralString(dialects.HiveFormatTime(f.Name()))
	}
	parseWithTz := g.funcCall("PARSE_DATETIME", []any{formatted, hiveFormat}, "(", ")", true)
	coalesced := g.funcCall("COALESCE", []any{g.funcCall("TRY", []any{parseWithoutTz}, "(", ")", true), parseWithTz}, "(", ")", true)
	return g.funcCall("TO_UNIXTIME", []any{coalesced}, "(", ")", true)
}

// prestoExplodeToUnnestSQL ports _explode_to_unnest_sql (presto.py:171-188) over
// explode_to_unnest_sql (dialect.py:2481-2512); the typed-struct alias branch needs
// annotate_types and is skipped.
func (g *Generator) prestoExplodeToUnnestSQL(e exp.Expression) string {
	explode := e.This()
	if explode != nil && explode.Kind() == exp.KindInline {
		explode = explode.Replace(exp.New(exp.KindExplode, exp.Args{"this": explode.This().Copy()}))
	}
	alias := asExpression(e.Arg("alias"))
	var crossJoin exp.Expression
	if explode != nil && explode.Kind() == exp.KindPosexplode && alias != nil {
		columns := listFromValue(alias.Arg("columns"))
		if len(columns) > 0 {
			pos := asExpression(columns[0])
			var cols []exp.Expression
			for _, c := range columns[1:] {
				cols = append(cols, asExpression(c).Copy())
			}
			selects := []exp.Expression{exp.AliasExpr(exp.Sub(exp.Args{"this": pos.Copy(), "expression": exp.LiteralNumber(1)}), pos.Copy(), false)}
			selects = append(selects, cols...)
			unnestCols := append([]exp.Expression{}, cols...)
			for i := range unnestCols {
				unnestCols[i] = unnestCols[i].Copy()
			}
			unnestCols = append(unnestCols, pos.Copy())
			unnest := exp.New(exp.KindUnnest, exp.Args{
				"expressions": []exp.Expression{explode.This()},
				"offset":      true,
				"alias":       exp.New(exp.KindTableAlias, exp.Args{"this": alias.This(), "columns": unnestCols}),
			})
			sub := exp.Select(exp.Args{"expressions": selects, "from_": exp.New(exp.KindFrom, exp.Args{"this": unnest})})
			crossJoin = exp.New(exp.KindLateral, exp.Args{"this": exp.New(exp.KindSubquery, exp.Args{"this": sub})})
		}
	} else if explode != nil && explode.Kind() == exp.KindExplode {
		crossJoin = exp.New(exp.KindUnnest, exp.Args{"expressions": []exp.Expression{explode.This()}, "alias": alias})
	}
	if crossJoin != nil {
		return g.gen(exp.New(exp.KindJoin, exp.Args{"this": crossJoin, "kind": "CROSS"}))
	}
	return g.lateralSQL(e)
}

// strPositionSQLWith ports strposition_sql (dialect.py:1321-1363) for a named function.
func (g *Generator) strPositionSQLWith(e exp.Expression, funcName string, supportsPosition, supportsOccurrence bool) string {
	return g.strPositionSQLFull(e, funcName, supportsPosition, supportsOccurrence, true)
}

// strPositionSQLFull is strposition_sql with its use_ansi_position flag.
func (g *Generator) strPositionSQLFull(e exp.Expression, funcName string, supportsPosition, supportsOccurrence, useANSIPosition bool) string {
	str, substr := e.This(), asExpression(e.Arg("substr"))
	position, occurrence := asExpression(e.Arg("position")), asExpression(e.Arg("occurrence"))
	if supportsOccurrence && occurrence != nil && supportsPosition && position == nil {
		position = exp.LiteralNumber(1)
	}
	transpilePosition := position != nil && !supportsPosition
	if transpilePosition {
		str = exp.New(exp.KindSubstring, exp.Args{"this": str, "start": position})
	}
	var fn exp.Expression
	if funcName == "POSITION" && useANSIPosition {
		fn = exp.Anonymous(exp.Args{"this": funcName, "expressions": []exp.Expression{exp.New(exp.KindIn, exp.Args{"this": substr, "field": str})}})
	} else {
		args := []exp.Expression{str, substr}
		if funcName == "LOCATE" || funcName == "CHARINDEX" {
			args = []exp.Expression{substr, str}
		}
		if supportsPosition && position != nil {
			args = append(args, position)
		}
		if occurrence != nil {
			if supportsOccurrence {
				args = append(args, occurrence)
			} else {
				g.unsupported(funcName + " does not support the occurrence parameter.")
			}
		}
		fn = exp.Anonymous(exp.Args{"this": funcName, "expressions": args})
	}
	if transpilePosition {
		withOffset := exp.Sub(exp.Args{"this": exp.Add(exp.Args{"this": fn.Copy(), "expression": position.Copy()}), "expression": exp.LiteralNumber(1)})
		wrapped := exp.If(exp.Args{"this": exp.EQ(exp.Args{"this": fn, "expression": exp.LiteralNumber(0)}), "true": exp.LiteralNumber(0), "false": withOffset})
		return g.gen(wrapped)
	}
	return g.gen(fn)
}

// noILikeSQL ports no_ilike_sql (dialect.py:1269-1276).
func (g *Generator) noILikeSQL(e exp.Expression) string {
	return g.likeSQL(exp.Like(exp.Args{
		"this":       exp.New(exp.KindLower, exp.Args{"this": e.This()}),
		"expression": exp.New(exp.KindLower, exp.Args{"this": e.Expr()}),
		"negate":     e.Arg("negate"),
	}))
}

// leftToSubstringSQL / rightToSubstringSQL port dialect.py:1752-1767.
func (g *Generator) leftToSubstringSQL(e exp.Expression) string {
	return g.gen(exp.New(exp.KindSubstring, exp.Args{"this": e.This(), "start": exp.LiteralNumber(1), "length": e.Expr()}))
}

func (g *Generator) rightToSubstringSQL(e exp.Expression) string {
	start := exp.Sub(exp.Args{
		"this":       exp.New(exp.KindLength, exp.Args{"this": e.This().Copy()}),
		"expression": exp.Paren(exp.Args{"this": exp.Sub(exp.Args{"this": e.Expr(), "expression": exp.LiteralNumber(1)})}),
	})
	return g.gen(exp.New(exp.KindSubstring, exp.Args{"this": e.This(), "start": start}))
}

var _ = strings.ToUpper
