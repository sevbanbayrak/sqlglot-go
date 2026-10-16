package sqlglot_test

import (
	"testing"

	sqlglot "github.com/sevbanbayrak/sqlglot-go"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/generator"
	"github.com/sevbanbayrak/sqlglot-go/optimizer"
)

// Ports the AST assertions of tests/dialects/test_clickhouse.py; its validate_identity and
// validate_all calls are covered by the dialect and transpile corpora.

func chParse(t *testing.T, sql string) exp.Expression {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, "clickhouse")
	if err != nil {
		t.Fatalf("ParseOne(%q): %v", sql, err)
	}
	return e
}

func chSQL(t *testing.T, e exp.Expression, dialect string) string {
	t.Helper()
	out, err := sqlglot.Generate(e, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return out
}

func chKind(t *testing.T, got exp.Expression, want exp.Kind) {
	t.Helper()
	if got == nil || got.Kind() != want {
		t.Fatalf("got %v, want %s", got, exp.ClassName(want))
	}
}

func TestClickHouseAST(t *testing.T) {
	// test_clickhouse:14-16
	chKind(t, chParse(t, "SELECT col.^nested, t.col2.^nested, t.col3.^nested.twice FROM t").Expressions()[0], exp.KindNestedJSONSelect)

	// test_clickhouse:35-45: arrays, maps and tuples can't be Nullable in ClickHouse.
	for _, typ := range []string{"ARRAY<INT>", "MAP<INT, INT>", "STRUCT(a: INT)"} {
		tryCast, err := sqlglot.ParseOne("TRY_CAST(x AS "+typ+")", "")
		if err != nil {
			t.Fatal(err)
		}
		target := chSQL(t, asExpr(tryCast.Arg("to")), "clickhouse")
		if got := chSQL(t, tryCast, "clickhouse"); got != "CAST(x AS "+target+")" {
			t.Errorf("TRY_CAST(x AS %s) = %s", typ, got)
		}
	}
	for _, typ := range []string{"INT", "UINT", "BIGINT", "FLOAT", "DOUBLE", "TEXT", "DATE", "UUID"} {
		tryCast, err := sqlglot.ParseOne("TRY_CAST(x AS "+typ+")", "")
		if err != nil {
			t.Fatal(err)
		}
		dtype, err := exp.DataTypeBuild(typ, "clickhouse", false, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		dtype.Set("nullable", false)
		target := chSQL(t, dtype, "clickhouse")
		if got := chSQL(t, tryCast, "clickhouse"); got != "CAST(x AS Nullable("+target+"))" {
			t.Errorf("TRY_CAST(x AS %s) = %s", typ, got)
		}
	}
	// test_clickhouse:47-48
	count, _ := sqlglot.ParseOne("count(x)", "")
	if got := chSQL(t, count, "clickhouse"); got != "COUNT(x)" {
		t.Errorf("count(x) = %s", got)
	}

	// test_clickhouse:664-689
	modify := chParse(t, "ALTER TABLE t MODIFY COLUMN c Int64")
	chKind(t, modify, exp.KindAlter)
	chKind(t, listOf(modify.Arg("actions"))[0], exp.KindAlterColumn)
	modifyOrder := chParse(t, "ALTER TABLE t MODIFY ORDER BY (a, b)")
	chKind(t, listOf(modifyOrder.Arg("actions"))[0], exp.KindAlterModifySqlSecurity)
	tuple, err := sqlglot.ParseInto("Tuple(select Int64)", "clickhouse", exp.KindDataType)
	if err != nil {
		t.Fatal(err)
	}
	chKind(t, tuple, exp.KindDataType)

	// test_clickhouse_values:791-794
	if n := len(chParse(t, "SELECT * FROM VALUES (1, 2, 3)").FindAll(exp.KindTuple)); n != 4 {
		t.Errorf("VALUES tuples = %d, want 4", n)
	}

	// test_cte:838-840
	with := asExpr(chParse(t, "WITH (SELECT 1) AS y SELECT * FROM y").Arg("with_"))
	cte := with.Expressions()[0]
	chKind(t, cte.This(), exp.KindSubquery)
	if cte.Alias() != "y" {
		t.Errorf("CTE alias = %q", cte.Alias())
	}

	// test_ternary:874-887
	ternary := chParse(t, "x ? (y ? 1 : 2) : 3")
	chKind(t, ternary, exp.KindIf)
	chKind(t, ternary.This(), exp.KindColumn)
	chKind(t, asExpr(ternary.Arg("true")), exp.KindParen)
	chKind(t, asExpr(ternary.Arg("false")), exp.KindLiteral)
	nested := asExpr(ternary.Arg("true")).This()
	chKind(t, nested.This(), exp.KindColumn)
	chKind(t, asExpr(nested.Arg("true")), exp.KindLiteral)
	chKind(t, asExpr(nested.Arg("false")), exp.KindLiteral)
	andTernary := chParse(t, "a and b ? 1 : 2")
	chKind(t, andTernary, exp.KindIf)
	chKind(t, andTernary.This(), exp.KindAnd)

	// test_ddl:1013-1044
	schemaTable := exp.Table(exp.Args{"schema": exp.ToIdentifier("foo")})
	for _, kind := range []string{"DATABASE", "SCHEMA"} {
		create := exp.Create(exp.Args{
			"this":       schemaTable.Copy(),
			"kind":       kind,
			"properties": exp.Properties(exp.Args{"expressions": []exp.Expression{exp.New(exp.KindOnCluster, exp.Args{"this": exp.ToIdentifier("c")})}}),
		})
		if got := chSQL(t, create, "clickhouse"); got != "CREATE DATABASE foo ON CLUSTER c" {
			t.Errorf("CREATE %s ON CLUSTER = %s", kind, got)
		}
	}
	ctas := exp.Create(exp.Args{
		"this":       exp.Table_("foo", nil, nil, nil, nil),
		"kind":       "TABLE",
		"expression": chParse(t, "SELECT * FROM db.other_table"),
		"properties": exp.Properties(exp.Args{"expressions": []exp.Expression{
			exp.EngineProperty(exp.Args{"this": exp.Var(exp.Args{"this": "Memory"})}),
			exp.SchemaCommentProperty(exp.Args{"this": exp.LiteralString("foo")}),
		}}),
	})
	if got := chSQL(t, ctas, "clickhouse"); got != "CREATE TABLE foo ENGINE=Memory AS (SELECT * FROM db.other_table) COMMENT 'foo'" {
		t.Errorf("CTAS with comment = %s", got)
	}

	// test_agg_functions:1491-1527 and test_agg_functions_multiple_suffixes:1529-1593
	for sql, want := range map[string]exp.Kind{
		"SELECT quantileExactInclusive(0.25)(number) AS x FROM numbers(5)":                exp.KindParameterizedAgg,
		"SELECT quantileGK(100, 0.95) OVER (PARTITION BY id) FROM table":                  exp.KindAnonymousAggFunc,
		"SELECT quantileGK(100, 0.95)(reading) OVER (PARTITION BY id) FROM table":         exp.KindParameterizedAgg,
		"SELECT quantileGKIf(100, 0.95) OVER (PARTITION BY id) FROM table":                exp.KindCombinedAggFunc,
		"SELECT quantileGKIf(100, 0.95)(reading) OVER (PARTITION BY id) FROM table":       exp.KindCombinedParameterizedAgg,
		"SELECT uniqExactIf(x, y) FROM t":                                                 exp.KindCombinedAggFunc,
		"SELECT countIfMerge(state) FROM t":                                               exp.KindCombinedAggFunc,
		"SELECT avgArgMinIfState(x, y) FROM t":                                            exp.KindCombinedAggFunc,
		"SELECT quantileIfState(0.5)(col, cond) FROM t":                                   exp.KindCombinedParameterizedAgg,
		"SELECT sumMap(k, v) FROM t":                                                      exp.KindAnonymousAggFunc,
		"SELECT maxMapIf(k, v, cond) FROM t":                                              exp.KindCombinedAggFunc,
		"SELECT sumMapIfState(k, v, cond) FROM t":                                         exp.KindCombinedAggFunc,
		"SELECT sumMergeIfMerge(s) FROM (SELECT sumMergeIfState(agg, 1 = 1) AS s FROM t)": exp.KindCombinedAggFunc,
	} {
		sel := chParse(t, sql).Expressions()[0]
		for sel.Kind() == exp.KindAlias || sel.Kind() == exp.KindWindow {
			sel = sel.This()
		}
		chKind(t, sel, want)
	}
	foobar, _ := sqlglot.ParseOne("foobar(x)", "")
	chKind(t, foobar, exp.KindAnonymous)
	chKind(t, chParse(t, "SELECT approx_top_sum(column, weight) FROM t").Expressions()[0], exp.KindAnonymousAggFunc)
	chKind(t, chParse(t, "SELECT approx_top_sum(N, reserved)(column, weight) FROM t").Expressions()[0], exp.KindParameterizedAgg)

	// test_array_join:1745-1761
	arrayJoin := chParse(t, "SELECT * FROM arrays_test ARRAY JOIN arr1, arrays_test.arr2 AS foo, ['a', 'b', 'c'] AS elem")
	joins := listOf(arrayJoin.Arg("joins"))
	if len(joins) != 1 || joins[0].Text("kind") != "ARRAY" {
		t.Fatalf("joins = %v", joins)
	}
	chKind(t, joins[0].This(), exp.KindColumn)
	joinExprs := joins[0].Expressions()
	if len(joinExprs) != 2 {
		t.Fatalf("ARRAY JOIN expressions = %d", len(joinExprs))
	}
	chKind(t, joinExprs[0], exp.KindAlias)
	chKind(t, joinExprs[0].This(), exp.KindColumn)
	chKind(t, joinExprs[1], exp.KindAlias)
	chKind(t, joinExprs[1].This(), exp.KindArray)

	// test_traverse_scope:1773-1776
	scopes := optimizer.TraverseScope(chParse(t, "SELECT * FROM t FINAL"))
	if len(scopes) != 1 || len(scopes[0].Sources) != 1 || scopes[0].Sources["t"] == nil {
		t.Fatalf("FINAL scope sources = %v", scopes)
	}

	// test_groupconcat:1786-1799
	gc := chParse(t, "SELECT groupConcat(', ')(part_name) FROM system.parts").Expressions()[0]
	chKind(t, gc, exp.KindGroupConcat)
	if gc.This().Name() != "part_name" || asExpr(gc.Arg("separator")).Name() != ", " {
		t.Errorf("groupConcat parts: %s", gc.ToS())
	}
	gc2 := chParse(t, "SELECT groupConcat(part_name) FROM t").Expressions()[0]
	chKind(t, gc2, exp.KindGroupConcat)
	if gc2.This().Name() != "part_name" || gc2.Arg("separator") != nil {
		t.Errorf("groupConcat without separator: %s", gc2.ToS())
	}
}

func asExpr(v any) exp.Expression {
	e, _ := v.(exp.Expression)
	return e
}

func listOf(v any) []exp.Expression {
	switch l := v.(type) {
	case []exp.Expression:
		return l
	case []any:
		out := make([]exp.Expression, 0, len(l))
		for _, item := range l {
			if e, ok := item.(exp.Expression); ok {
				out = append(out, e)
			}
		}
		return out
	}
	return nil
}
