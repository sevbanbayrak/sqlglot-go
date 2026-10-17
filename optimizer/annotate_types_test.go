package optimizer_test

import (
	"fmt"
	"strings"
	"testing"

	sqlglot "github.com/sevbanbayrak/sqlglot-go"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/generator"
	"github.com/sevbanbayrak/sqlglot-go/optimizer"
	"github.com/sevbanbayrak/sqlglot-go/schema"
)

// unportedTypingDialects are fixture dialects with no port; their cases are skipped
// (ROADMAP "annotate_types").
var unportedTypingDialects = map[string]bool{"bigquery": true, "duckdb": true, "tsql": true, "redshift": true}

func typeSQL(t *testing.T, e exp.Expression, dialect string) string {
	t.Helper()
	if e == nil {
		return "<nil>"
	}
	sql, err := sqlglot.Generate(e, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate type: %v", err)
	}
	return sql
}

func expectedTypeSQL(t *testing.T, expected, dialect string) string {
	t.Helper()
	built, err := exp.DataTypeBuild(expected, dialect, false, true, nil)
	if err != nil {
		t.Fatalf("DataType.build(%q, %q): %v", expected, dialect, err)
	}
	return typeSQL(t, built, dialect)
}

// TestAnnotateTypesFixtures ports test_optimizer.py test_annotate_types.
func TestAnnotateTypesFixtures(t *testing.T) {
	ran := 0
	for i, pair := range loadSQLFixturePairs(t, "annotate_types.sql") {
		title := pair.Meta["title"]
		if title == "" {
			title = fmt.Sprintf("%d, %s", i+1, pair.SQL)
		}
		dialect := pair.Meta["dialect"]
		t.Run(title, func(t *testing.T) {
			if unportedTypingDialects[dialect] {
				t.Skipf("dialect %s is not ported", dialect)
			}
			ran++
			expression, err := sqlglot.ParseOne(pair.SQL, dialect)
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			result := optimizer.AnnotateTypes(expression, optimizer.AnnotateTypesOpts{Dialect: dialect})
			if got, want := typeSQL(t, result.Type(), dialect), expectedTypeSQL(t, pair.Expected, dialect); got != want {
				t.Errorf("type = %s, want %s", got, want)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no annotate_types.sql case ran")
	}

	// Programmatic set operations with VALUES operands must not crash the Query-typed
	// left/right accessors in compiled builds
	values, err := sqlglot.ParseOne("SELECT * FROM (SELECT 1 UNION VALUES (2)) AS s", "")
	if err != nil {
		t.Fatal(err)
	}
	optimizer.AnnotateTypes(values, optimizer.AnnotateTypesOpts{})
}

func annotateFunctionsSchema() *schema.Mapping {
	return schema.M(
		"tbl", schema.M(
			"bin_col", "BINARY",
			"str_col", "STRING",
			"bignum_col", "BIGNUMERIC",
			"date_col", "DATE",
			"decfloat_col", "DECFLOAT",
			"float_col", "FLOAT",
			"timestamp_col", "TIMESTAMP",
			"double_col", "DOUBLE",
			"bigint_col", "BIGINT",
			"obj_col", "OBJECT",
			"int_col", "INT",
			"bool_col", "BOOLEAN",
			"bytes_col", "BYTES",
			"interval_col", "INTERVAL",
			"array_col", "ARRAY<STRING>",
		),
	)
}

// TestAnnotateFunctionsFixtures ports test_optimizer.py test_annotate_funcs.
func TestAnnotateFunctionsFixtures(t *testing.T) {
	passed, skipped := 0, 0
	for i, pair := range loadSQLFixturePairs(t, "annotate_functions.sql") {
		title := pair.Meta["title"]
		if title == "" {
			title = fmt.Sprintf("%d, %s", i+1, pair.SQL)
		}
		sql := "SELECT " + pair.SQL + " FROM tbl"
		for _, dialect := range strings.Split(pair.Meta["dialect"], ", ") {
			dialect := strings.TrimSpace(dialect)
			t.Run(title+" ["+dialect+"]", func(t *testing.T) {
				if unportedTypingDialects[dialect] {
					skipped++
					t.Skipf("dialect %s is not ported", dialect)
				}
				expression, err := sqlglot.ParseOne(sql, dialect)
				if err != nil {
					t.Fatalf("ParseOne: %v", err)
				}
				annotated := optimizer.AnnotateTypes(expression, optimizer.AnnotateTypesOpts{Schema: annotateFunctionsSchema(), Dialect: dialect})
				result := annotated.Expressions()[0]
				if got, want := typeSQL(t, result.Type(), dialect), expectedTypeSQL(t, pair.Expected, dialect); got != want {
					t.Errorf("type = %s, want %s", got, want)
					return
				}
				passed++
			})
		}
	}
	t.Logf("annotate_functions.sql: %d passed, %d skipped (unported dialects)", passed, skipped)
}

func mustParse(t *testing.T, sql, dialect string) exp.Expression {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q): %v", sql, err)
	}
	return e
}

func annotate(e exp.Expression, s schema.SchemaType, dialect string) exp.Expression {
	return optimizer.AnnotateTypes(e, optimizer.AnnotateTypesOpts{Schema: s, Dialect: dialect})
}

func qualifyFor(e exp.Expression, s schema.SchemaType, dialect string) exp.Expression {
	opts := optimizer.DefaultQualifyOpts()
	opts.Schema = s
	opts.Dialect = dialect
	return optimizer.Qualify(e, opts)
}

func mustSQL(t *testing.T, e exp.Expression, dialect string) string {
	t.Helper()
	sql, err := sqlglot.Generate(e, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return sql
}

func typeThis(e exp.Expression) any {
	if e == nil || e.Type() == nil {
		return nil
	}
	return e.Type().Arg("this")
}

func buildType(t *testing.T, dtype, dialect string) exp.Expression {
	t.Helper()
	built, err := exp.DataTypeBuild(dtype, dialect, false, true, nil)
	if err != nil {
		t.Fatalf("DataType.build(%q): %v", dtype, err)
	}
	return built
}

func assertThis(t *testing.T, e exp.Expression, want exp.DType) {
	t.Helper()
	if got := typeThis(e); got != want {
		t.Errorf("type = %v, want %v", got, want)
	}
}

func assertTypeEqual(t *testing.T, got exp.Expression, want exp.Expression) {
	t.Helper()
	if got == nil || !got.Equal(want) {
		t.Errorf("type = %v, want %v", got, want.ToS())
	}
}

// TestAnnotateUntypedBinaryOperands ports test_annotate_untyped_binary_operands.
func TestAnnotateUntypedBinaryOperands(t *testing.T) {
	// Operands that escaped annotation have type None; treat them as UNKNOWN instead
	// of crashing on `.type.this`
	s, err := schema.NewMappingSchema(nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	annotator := optimizer.NewTypeAnnotator(s)

	if !annotator.AnnotateBinary(mustParse(t, "a + b", "")).IsType(exp.DTypeUnknown) {
		t.Error("a + b is not UNKNOWN")
	}
	if !annotator.AnnotateDiv(mustParse(t, "a / b", "")).IsType(exp.DTypeUnknown) {
		t.Error("a / b is not UNKNOWN")
	}
}

// TestAnnotateJSONDotPartsPreserveMeta ports test_annotate_json_dot_parts_preserve_meta. The
// statement is parsed with the base dialect (duckdb is not ported; both parse it to the same
// tree), and since the port's parser records no token positions on identifiers, the preserved
// meta is a marker set before annotation.
func TestAnnotateJSONDotPartsPreserveMeta(t *testing.T) {
	// The case restoration of JSON dot parts must rename identifiers in place, so
	// that their meta (e.g. token positions) is preserved
	s := schema.M("cat", schema.M("sch", schema.M("t", schema.M("id", "varchar", "payload", "json"))))
	sql := "SELECT payload.CaseSensitive FROM cat.sch.t"

	opts := optimizer.DefaultQualifyOpts()
	opts.Schema = s
	opts.Dialect = "snowflake"
	opts.QuoteIdentifiers = false
	qualified := optimizer.Qualify(mustParse(t, sql, ""), opts)
	asExpr(qualified.Selects()[0].This().Arg("expression")).Meta()["marker"] = true
	annotated := annotate(qualified, s, "snowflake")

	identifier := asExpr(annotated.Selects()[0].This().Arg("expression"))
	if identifier == nil || identifier.Kind() != exp.KindIdentifier {
		t.Fatalf("not an identifier: %v", identifier)
	}
	if identifier.Name() != "CaseSensitive" || identifier.Arg("quoted") != true {
		t.Errorf("identifier = %s", identifier.ToS())
	}
	if identifier.MetaGet("marker") != true {
		t.Error("identifier meta was not preserved")
	}
}

func asExpr(v any) exp.Expression {
	e, _ := v.(exp.Expression)
	return e
}

// TestAnnotateTypesCachesSchemaLookups ports test_annotate_types_caches_schema_lookups.
func TestAnnotateTypesCachesSchemaLookups(t *testing.T) {
	s, err := schema.NewMappingSchema(schema.M("t", schema.M("a", "INT")), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	qualified := qualifyFor(mustParse(t, "SELECT a, a FROM t", ""), s, "")
	pre := s.FindCacheLen()
	annotate(qualified, s, "")
	if got := s.FindCacheLen() - pre; got != 1 {
		t.Errorf("find cache grew by %d, want 1", got)
	}
}

// TestCastTypeAnnotation ports test_cast_type_annotation.
func TestCastTypeAnnotation(t *testing.T) {
	expression := annotate(mustParse(t, "CAST('2020-01-01' AS TIMESTAMPTZ(9))", ""), nil, "")
	assertThis(t, expression, exp.DTypeTimestampTz)
	assertThis(t, expression.This(), exp.DTypeVarchar)
	to := asExpr(expression.Arg("to"))
	assertThis(t, to, exp.DTypeTimestampTz)
	assertThis(t, to.Expressions()[0].This(), exp.DTypeInt)

	expression = annotate(mustParse(t, "ARRAY(1)::ARRAY<INT>", ""), nil, "")
	want, err := sqlglot.ParseInto("ARRAY<INT>", "", exp.KindDataType)
	if err != nil {
		t.Fatal(err)
	}
	assertTypeEqual(t, expression.Type(), want)

	expression = annotate(mustParse(t, "CAST(x AS INTERVAL)", ""), nil, "")
	assertThis(t, expression, exp.DTypeInterval)
	assertThis(t, expression.This(), exp.DTypeUnknown)
	assertThis(t, asExpr(expression.Arg("to")), exp.DTypeInterval)
}

// TestCacheAnnotation ports test_cache_annotation.
func TestCacheAnnotation(t *testing.T) {
	expression := annotate(mustParse(t, "CACHE LAZY TABLE x OPTIONS('storageLevel' = 'value') AS SELECT 1", ""), nil, "")
	assertThis(t, expression.Expr().Expressions()[0], exp.DTypeInt)
}

// TestBinaryAnnotation ports test_binary_annotation.
func TestBinaryAnnotation(t *testing.T) {
	expression := annotate(mustParse(t, "SELECT 0.0 + (2 + 3)", ""), nil, "").Expressions()[0]

	assertThis(t, expression, exp.DTypeDouble)
	assertThis(t, expression.Left(), exp.DTypeDouble)
	assertThis(t, expression.Right(), exp.DTypeInt)
	assertThis(t, expression.Right().This(), exp.DTypeInt)
	assertThis(t, expression.Right().This().Left(), exp.DTypeInt)
	assertThis(t, expression.Right().This().Right(), exp.DTypeInt)

	for _, numericType := range []string{"BIGINT", "DOUBLE", "INT"} {
		expression := annotate(mustParse(t, "SELECT '1' + CAST(x AS "+numericType+")", ""), nil, "").Expressions()[0]
		assertTypeEqual(t, expression.Type(), buildType(t, numericType, ""))
	}
}

// TestTypedDivAnnotation ports test_typeddiv_annotation.
func TestTypedDivAnnotation(t *testing.T) {
	expressions := annotate(mustParse(t, "SELECT 2 / 3, 2 / 3.0", "presto"), nil, "").Expressions()
	assertThis(t, expressions[0], exp.DTypeBigInt)
	assertThis(t, expressions[1], exp.DTypeDouble)

	expressions = annotate(mustParse(t, "SELECT SUM(2 / 3), CAST(2 AS DECIMAL) / 3", "mysql"), nil, "").Expressions()
	assertThis(t, expressions[0], exp.DTypeDouble)
	assertThis(t, expressions[0].This(), exp.DTypeDouble)
	assertThis(t, expressions[1], exp.DTypeDecimal)
}

// TestBracketAnnotation ports test_bracket_annotation.
func TestBracketAnnotation(t *testing.T) {
	expression := annotate(mustParse(t, "SELECT A[:]", ""), nil, "").Expressions()[0]
	assertThis(t, expression, exp.DTypeUnknown)
	assertThis(t, expression.Expressions()[0], exp.DTypeUnknown)

	expression = annotate(mustParse(t, "SELECT ARRAY[1, 2, 3][1]", ""), nil, "").Expressions()[0]
	if got := mustSQL(t, expression.This().Type(), ""); got != "ARRAY<INT>" {
		t.Errorf("this type = %s", got)
	}
	assertThis(t, expression, exp.DTypeInt)

	expression = annotate(mustParse(t, "SELECT ARRAY[1, 2, 3][1 : 2]", ""), nil, "").Expressions()[0]
	if got := mustSQL(t, expression.This().Type(), ""); got != "ARRAY<INT>" {
		t.Errorf("this type = %s", got)
	}
	if got := mustSQL(t, expression.Type(), ""); got != "ARRAY<INT>" {
		t.Errorf("type = %s", got)
	}

	expression = annotate(mustParse(t, "SELECT ARRAY[ARRAY[1], ARRAY[2], ARRAY[3]][1][2]", ""), nil, "").Expressions()[0]
	if got := mustSQL(t, expression.This().This().Type(), ""); got != "ARRAY<ARRAY<INT>>" {
		t.Errorf("this.this type = %s", got)
	}
	if got := mustSQL(t, expression.This().Type(), ""); got != "ARRAY<INT>" {
		t.Errorf("this type = %s", got)
	}
	assertThis(t, expression, exp.DTypeInt)

	expression = annotate(mustParse(t, "SELECT ARRAY[ARRAY[1], ARRAY[2], ARRAY[3]][1:2]", ""), nil, "").Expressions()[0]
	if got := mustSQL(t, expression.Type(), ""); got != "ARRAY<ARRAY<INT>>" {
		t.Errorf("type = %s", got)
	}

	assertThis(t, annotate(mustParse(t, "MAP(1.0, 2, '2', 3.0)['2']", "spark"), nil, ""), exp.DTypeDouble)
	assertThis(t, annotate(mustParse(t, "MAP(1.0, 2, x, 3.0)[2]", "spark"), nil, ""), exp.DTypeUnknown)
	assertThis(t, annotate(mustParse(t, "MAP(ARRAY(1.0, x), ARRAY(2, 3.0))[x]", ""), nil, ""), exp.DTypeDouble)

	expression = annotate(mustParse(t, "SELECT MAP(1.0, 2, 2, t.y)[2] FROM t", "spark"), schema.M("t", schema.M("y", "int")), "").Expressions()[0]
	assertThis(t, expression, exp.DTypeInt)
}

// TestIntervalMathAnnotation ports test_interval_math_annotation.
func TestIntervalMathAnnotation(t *testing.T) {
	s := schema.M("x", schema.M("a", "DATE", "b", "DATETIME"))
	for _, tc := range []struct {
		sql  string
		want exp.DType
	}{
		{"SELECT '2023-01-01' + INTERVAL '1' DAY", exp.DTypeDate},
		{"SELECT '2023-01-01' + INTERVAL '1' HOUR", exp.DTypeDatetime},
		{"SELECT '2023-01-01 00:00:01' + INTERVAL '1' HOUR", exp.DTypeDatetime},
		{"SELECT 'nonsense' + INTERVAL '1' DAY", exp.DTypeUnknown},
		{"SELECT x.a + INTERVAL '1' DAY FROM x AS x", exp.DTypeDate},
		{"SELECT x.a + INTERVAL '1' HOUR FROM x AS x", exp.DTypeDatetime},
		{"SELECT x.b + INTERVAL '1' DAY FROM x AS x", exp.DTypeDatetime},
		{"SELECT x.b + INTERVAL '1' HOUR FROM x AS x", exp.DTypeDatetime},
		{"SELECT DATE_ADD('2023-01-01', 1, 'DAY')", exp.DTypeDate},
		{"SELECT DATE_ADD('2023-01-01 00:00:00', 1, 'DAY')", exp.DTypeDatetime},
		{"SELECT DATE_ADD(x.a, 1, 'DAY') FROM x AS x", exp.DTypeDate},
		{"SELECT DATE_ADD(x.a, 1, 'HOUR') FROM x AS x", exp.DTypeDatetime},
		{"SELECT DATE_ADD(x.b, 1, 'DAY') FROM x AS x", exp.DTypeDatetime},
		{"SELECT DATE_TRUNC('DAY', x.a) FROM x AS x", exp.DTypeDate},
		{"SELECT DATE_TRUNC('DAY', x.b) FROM x AS x", exp.DTypeDatetime},
		{"SELECT DATE_TRUNC('SECOND', x.a) FROM x AS x", exp.DTypeDatetime},
		{"SELECT DATE_TRUNC('DAY', '2023-01-01') FROM x AS x", exp.DTypeDate},
		{"SELECT DATEDIFF('2023-01-01', '2023-01-02', DAY) FROM x AS x", exp.DTypeInt},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			expression := annotate(mustParse(t, tc.sql, ""), s, "")
			assertThis(t, expression.Expressions()[0], tc.want)
			if got := mustSQL(t, expression, ""); got != tc.sql {
				t.Errorf("sql = %s", got)
			}
		})
	}
}

// TestLateralAnnotation ports test_lateral_annotation. optimize() is not ported; qualify +
// annotate_types are the passes the type assertion rests on.
func TestLateralAnnotation(t *testing.T) {
	e := qualifyFor(mustParse(t, "SELECT c FROM (select 1 a) as x LATERAL VIEW EXPLODE (a) AS c", ""), nil, "")
	assertThis(t, annotate(e, nil, "").Expressions()[0], exp.DTypeInt)
}

// TestDerivedTablesColumnAnnotation ports test_derived_tables_column_annotation.
func TestDerivedTablesColumnAnnotation(t *testing.T) {
	s := schema.M("x", schema.M("cola", "INT"), "y", schema.M("cola", "FLOAT"))
	sql := `
            SELECT a.cola AS cola
            FROM (
                SELECT x.cola + y.cola AS cola
                FROM (
                    SELECT x.cola AS cola
                    FROM x AS x
                ) AS x
                JOIN (
                    SELECT y.cola AS cola
                    FROM y AS y
                ) AS y
            ) AS a
        `
	expression := annotate(mustParse(t, sql, ""), s, "")
	assertThis(t, expression.Expressions()[0], exp.DTypeFloat) // a.cola AS cola

	additionAlias := asExpr(expression.Arg("from_")).This().This().Expressions()[0]
	assertThis(t, additionAlias, exp.DTypeFloat) // x.cola + y.cola AS cola

	addition := additionAlias.This()
	assertThis(t, addition, exp.DTypeFloat)
	assertThis(t, addition.This(), exp.DTypeInt)
	assertThis(t, addition.Expr(), exp.DTypeFloat)
}

// TestCTEColumnAnnotation ports test_cte_column_annotation.
func TestCTEColumnAnnotation(t *testing.T) {
	s := schema.M("x", schema.M("cola", "CHAR"), "y", schema.M("colb", "TEXT", "colc", "BOOLEAN"))
	sql := `
            WITH tbl AS (
                SELECT x.cola + 'bla' AS cola, y.colb AS colb, y.colc AS colc
                FROM (
                    SELECT x.cola AS cola
                    FROM x AS x
                ) AS x
                JOIN (
                    SELECT y.colb AS colb, y.colc AS colc
                    FROM y AS y
                ) AS y
            )
            SELECT tbl.cola + tbl.colb + 'foo' AS col
            FROM tbl AS tbl
            WHERE tbl.colc = True
        `
	expression := annotate(mustParse(t, sql, ""), s, "")
	assertThis(t, expression.Expressions()[0], exp.DTypeText) // tbl.cola + tbl.colb + 'foo' AS col

	outerAddition := expression.Expressions()[0].This() // (tbl.cola + tbl.colb) + 'foo'
	assertThis(t, outerAddition, exp.DTypeText)
	assertThis(t, outerAddition.Left(), exp.DTypeText)
	assertThis(t, outerAddition.Right(), exp.DTypeVarchar)

	innerAddition := expression.Expressions()[0].This().Left() // tbl.cola + tbl.colb
	assertThis(t, innerAddition.Left(), exp.DTypeVarchar)
	assertThis(t, innerAddition.Right(), exp.DTypeText)

	// WHERE tbl.colc = True
	assertThis(t, asExpr(expression.Arg("where")).This(), exp.DTypeBoolean)

	cteSelect := asExpr(expression.Arg("with_")).Expressions()[0].This()
	assertThis(t, cteSelect.Expressions()[0], exp.DTypeVarchar) // x.cola + 'bla' AS cola
	assertThis(t, cteSelect.Expressions()[1], exp.DTypeText)    // y.colb AS colb
	assertThis(t, cteSelect.Expressions()[2], exp.DTypeBoolean) // y.colc AS colc

	cteSelectAddition := cteSelect.Expressions()[0].This() // x.cola + 'bla'
	assertThis(t, cteSelectAddition, exp.DTypeVarchar)
	assertThis(t, cteSelectAddition.Left(), exp.DTypeChar)
	assertThis(t, cteSelectAddition.Right(), exp.DTypeVarchar)

	// Check that x.cola AS cola and y.colb AS colb have types CHAR and TEXT, respectively
	subqueries := cteSelect.FindAll(exp.KindSubquery)
	for i, want := range []exp.DType{exp.DTypeChar, exp.DTypeText} {
		assertThis(t, subqueries[i].This().Expressions()[0].This(), want)
	}
}

// TestFunctionAnnotation ports test_function_annotation.
func TestFunctionAnnotation(t *testing.T) {
	s := schema.M("x", schema.M("cola", "VARCHAR", "colb", "CHAR"))
	sql := "SELECT x.cola || TRIM(x.colb) AS col, DATE(x.colb), DATEFROMPARTS(y, m, d) FROM x AS x"

	expression := annotate(mustParse(t, sql, ""), s, "")
	concatExprAlias := expression.Expressions()[0]
	assertThis(t, concatExprAlias, exp.DTypeVarchar)

	concatExpr := concatExprAlias.This()
	assertThis(t, concatExpr, exp.DTypeVarchar)
	assertThis(t, concatExpr.Left(), exp.DTypeVarchar)        // x.cola
	assertThis(t, concatExpr.Right(), exp.DTypeVarchar)       // TRIM(x.colb)
	assertThis(t, concatExpr.Right().This(), exp.DTypeChar)   // x.colb
	assertThis(t, expression.Expressions()[1], exp.DTypeDate) // DATE(x.colb)
	assertThis(t, expression.Expressions()[2], exp.DTypeDate) // DATEFROMPARTS(y, m, d)

	sql = "SELECT CASE WHEN 1=1 THEN x.cola ELSE x.colb END AS col FROM x AS x"

	caseExprAlias := annotate(mustParse(t, sql, ""), s, "").Expressions()[0]
	assertThis(t, caseExprAlias, exp.DTypeVarchar)

	caseExpr := caseExprAlias.This()
	assertThis(t, caseExpr, exp.DTypeVarchar)
	assertThis(t, asExpr(caseExpr.Arg("default")), exp.DTypeChar)

	caseIfsExpr := exp.ExpressionsOf(caseExpr, "ifs")[0]
	assertThis(t, caseIfsExpr, exp.DTypeVarchar)
	assertThis(t, asExpr(caseIfsExpr.Arg("true")), exp.DTypeVarchar)

	assertThis(t, annotate(mustParse(t, "TIMESTAMP(x)", ""), nil, ""), exp.DTypeTimestamp)
	// The TIMESTAMP(x) read="bigquery" -> TIMESTAMPTZ case is skipped: bigquery is not ported.
}

// TestUnknownAnnotation ports test_unknown_annotation.
func TestUnknownAnnotation(t *testing.T) {
	s := schema.M("x", schema.M("cola", "VARCHAR"))
	sql := "SELECT x.cola + SOME_ANONYMOUS_FUNC(x.cola) AS col FROM x AS x"

	concatExprAlias := annotate(mustParse(t, sql, ""), s, "").Expressions()[0]
	assertThis(t, concatExprAlias, exp.DTypeUnknown)

	concatExpr := concatExprAlias.This()
	assertThis(t, concatExpr, exp.DTypeUnknown)
	assertThis(t, concatExpr.Left(), exp.DTypeVarchar)                   // x.cola
	assertThis(t, concatExpr.Right(), exp.DTypeUnknown)                  // SOME_ANONYMOUS_FUNC(x.cola)
	assertThis(t, concatExpr.Right().Expressions()[0], exp.DTypeVarchar) // x.cola (arg)

	// Ensures we don't raise if there are unqualified columns
	annotate(mustParse(t, "select x from y lateral view explode(y) as x", ""), nil, "")

	// NULL <op> UNKNOWN should yield UNKNOWN
	assertThis(t, annotate(mustParse(t, "SELECT NULL + ANONYMOUS_FUNC()", ""), nil, "").Expressions()[0], exp.DTypeUnknown)
}

// TestUDFAnnotation ports the part of test_udf_annotation that needs no udf_mapping: the port's
// MappingSchema has no udf_mapping yet (schema.go GetUDFType), so every UDF types as UNKNOWN.
func TestUDFAnnotation(t *testing.T) {
	s := schema.M("t", schema.M("col", "INT"))
	expr := annotate(mustParse(t, "SELECT unknown_func(col) FROM t", ""), s, "")
	assertThis(t, expr.Selects()[0], exp.DTypeUnknown)
}

// TestPredicateAnnotation ports test_predicate_annotation.
func TestPredicateAnnotation(t *testing.T) {
	assertThis(t, annotate(mustParse(t, "x BETWEEN a AND b", ""), nil, ""), exp.DTypeBoolean)
	assertThis(t, annotate(mustParse(t, "x IN (a, b, c, d)", ""), nil, ""), exp.DTypeBoolean)
}

// TestAggFuncAnnotation ports test_aggfunc_annotation.
func TestAggFuncAnnotation(t *testing.T) {
	s := schema.M("x", schema.M("cola", "SMALLINT", "colb", "FLOAT", "colc", "TEXT", "cold", "DATE"))
	for _, tc := range []struct {
		fn, col string
		want    exp.DType
	}{
		{"AVG", "cola", exp.DTypeDouble},
		{"SUM", "cola", exp.DTypeBigInt},
		{"SUM", "colb", exp.DTypeDouble},
		{"MIN", "cola", exp.DTypeSmallInt},
		{"MIN", "colb", exp.DTypeFloat},
		{"MAX", "colc", exp.DTypeText},
		{"MAX", "cold", exp.DTypeDate},
		{"COUNT", "colb", exp.DTypeBigInt},
		{"STDDEV", "cola", exp.DTypeDouble},
		{"ABS", "cola", exp.DTypeSmallInt},
		{"ABS", "colb", exp.DTypeFloat},
	} {
		expression := annotate(mustParse(t, "SELECT "+tc.fn+"(x."+tc.col+") AS _col_0 FROM x AS x", ""), s, "")
		if got := typeThis(expression.Expressions()[0]); got != tc.want {
			t.Errorf("%s(%s) = %v, want %v", tc.fn, tc.col, got, tc.want)
		}
	}
}

// TestConcatAnnotation ports test_concat_annotation.
func TestConcatAnnotation(t *testing.T) {
	assertThis(t, annotate(mustParse(t, "CONCAT('A', 'B')", ""), nil, ""), exp.DTypeVarchar)
}

// TestRootSubqueryAnnotation ports test_root_subquery_annotation.
func TestRootSubqueryAnnotation(t *testing.T) {
	expression := annotate(mustParse(t, "(SELECT 1, 2 FROM x) LIMIT 0", ""), nil, "")
	if expression.Kind() != exp.KindSubquery {
		t.Fatalf("root = %s", exp.ClassName(expression.Kind()))
	}
	assertThis(t, expression.Selects()[0], exp.DTypeInt)
	assertThis(t, expression.Selects()[1], exp.DTypeInt)
}

// TestNestedTypeAnnotation ports test_nested_type_annotation, minus its bigquery-schema case.
func TestNestedTypeAnnotation(t *testing.T) {
	s := schema.M("order", schema.M("customer_id", "bigint", "item_id", "bigint", "item_price", "numeric"))
	sql := `
            SELECT ARRAY_AGG(DISTINCT order.item_id) FILTER (WHERE order.item_price > 10) AS items,
            FROM order AS order
            GROUP BY order.customer_id
        `
	expression := annotate(mustParse(t, sql, ""), s, "")
	assertThis(t, expression.Selects()[0], exp.DTypeArray)
	if got := mustSQL(t, expression.Selects()[0].Type(), ""); got != "ARRAY<BIGINT>" {
		t.Errorf("type = %s", got)
	}

	expression = annotate(mustParse(t, "SELECT ARRAY_CAT(ARRAY[1,2,3], ARRAY[4,5])", "postgres"), nil, "")
	assertThis(t, expression.Selects()[0], exp.DTypeArray)
	if got := mustSQL(t, expression.Selects()[0].Type(), ""); got != "ARRAY<INT>" {
		t.Errorf("type = %s", got)
	}

	expression = annotate(mustParse(t, "SELECT unnest(t.x) FROM t AS t", "postgres"), schema.M("t", schema.M("x", "array<int>")), "")
	if !expression.Selects()[0].IsType("int") {
		t.Errorf("unnest type = %v", expression.Selects()[0].Type())
	}
}

// TestTypeAnnotationCache ports test_type_annotation_cache.
func TestTypeAnnotationCache(t *testing.T) {
	expression := annotate(mustParse(t, "SELECT 1 + 1", ""), nil, "")
	assertThis(t, expression.Selects()[0], exp.DTypeInt)

	expression.Selects()[0].This().Replace(mustParse(t, "1.2", ""))
	expression = annotate(expression, nil, "")
	assertThis(t, expression.Selects()[0], exp.DTypeDouble)
}

// TestUserDefinedTypeAnnotation ports test_user_defined_type_annotation.
func TestUserDefinedTypeAnnotation(t *testing.T) {
	s, err := schema.NewMappingSchema(schema.M("t", schema.M("x", "int")), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	expression := annotate(mustParse(t, "SELECT CAST(x AS IPADDRESS) FROM t", ""), s, "")
	assertThis(t, expression.Selects()[0], exp.DTypeUserDefined)
	if got := mustSQL(t, expression.Selects()[0].Type(), "postgres"); got != "IPADDRESS" {
		t.Errorf("type = %s", got)
	}
}

// TestUnnestAnnotation ports the base-dialect cases of test_unnest_annotation.
func TestUnnestAnnotation(t *testing.T) {
	expression := annotate(qualifyFor(mustParse(t, "SELECT x FROM UNNEST(GENERATE_DATE_ARRAY('2021-01-01', current_date(), interval 1 day)) AS x", ""), nil, ""), nil, "")
	assertTypeEqual(t, expression.Selects()[0].Type(), buildType(t, "date", ""))

	expression = annotate(qualifyFor(mustParse(t, "SELECT x FROM UNNEST(GENERATE_TIMESTAMP_ARRAY('2016-10-05 00:00:00', '2016-10-06 02:00:00', interval 1 day)) AS x", ""), nil, ""), nil, "")
	assertTypeEqual(t, expression.Selects()[0].Type(), buildType(t, "timestamp", ""))
}

// TestMapAnnotation ports the spark case of test_map_annotation (the others read duckdb).
func TestMapAnnotation(t *testing.T) {
	// VarMap annotation
	expression := annotate(mustParse(t, "SELECT MAP('a', 'b')", "spark"), nil, "")
	assertTypeEqual(t, expression.Selects()[0].Type(), buildType(t, "MAP(VARCHAR, VARCHAR)", ""))
}

// TestUnionAnnotation ports test_union_annotation, minus its duckdb and bigquery cases.
func TestUnionAnnotation(t *testing.T) {
	for _, tc := range []struct{ left, right, want string }{
		{"SELECT 1::INT AS c", "SELECT 2::BIGINT AS c", "BIGINT"},
		{"SELECT 1::INT AS c", "SELECT 2::BIGDECIMAL AS c", "BIGDECIMAL"},
		{"SELECT 1 AS c", "SELECT NULL AS c", "INT"},
		{"SELECT FOO() AS c", "SELECT 1 AS c", "UNKNOWN"},
		{"SELECT FOO() AS c", "SELECT BAR() AS c", "UNKNOWN"},
	} {
		lr := annotate(mustParse(t, "SELECT t.c FROM ("+tc.left+" UNION ALL "+tc.right+") t(c)", ""), nil, "")
		rl := annotate(mustParse(t, "SELECT t.c FROM ("+tc.right+" UNION ALL "+tc.left+") t(c)", ""), nil, "")
		want := buildType(t, tc.want, "")
		if !lr.Selects()[0].Type().Equal(want) || !rl.Selects()[0].Type().Equal(want) {
			t.Errorf("%s / %s: %v %v, want %s", tc.left, tc.right, lr.Selects()[0].Type(), rl.Selects()[0].Type(), tc.want)
		}
	}

	unionByName := annotate(mustParse(t, "SELECT t.a, t.d FROM (SELECT 1 a, 3 d, UNION ALL BY NAME SELECT 7.0 d, 8::BIGINT a) AS t(a, d)", ""), nil, "")
	assertThis(t, unionByName.Selects()[0], exp.DTypeBigInt)
	assertThis(t, unionByName.Selects()[1], exp.DTypeDouble)

	// Test chained UNIONs. optimize() is not ported; qualify + annotate_types are the passes the
	// type assertions rest on.
	for _, sql := range []string{
		`WITH t AS
            (
                SELECT NULL AS col
                UNION
                SELECT NULL AS col
                UNION
                SELECT 'a' AS col
                UNION
                SELECT NULL AS col
                UNION
                SELECT NULL AS col
            )
            SELECT col FROM t;`,
		// Test UNIONs with nested subqueries
		`WITH t AS
            (
                SELECT NULL AS col
                UNION
                (SELECT NULL AS col UNION ALL SELECT 'a' AS col)
            )
            SELECT col FROM t;`,
		`WITH t AS
            (
                (SELECT NULL AS col UNION ALL SELECT 'a' AS col)
                UNION
                SELECT NULL AS col
            )
            SELECT col FROM t;`,
	} {
		assertThis(t, annotate(qualifyFor(mustParse(t, sql, ""), nil, ""), nil, "").Selects()[0], exp.DTypeVarchar)
	}
}

// TestUDTFAnnotation ports test_udtf_annotation.
func TestUDTFAnnotation(t *testing.T) {
	tableUDTF := mustParse(t, "SELECT * FROM TABLE(GENERATOR(ROWCOUNT => 100000))", "snowflake")
	if got := mustSQL(t, annotate(tableUDTF, nil, "snowflake"), "snowflake"); got != "SELECT * FROM TABLE(GENERATOR(ROWCOUNT => 100000))" {
		t.Errorf("sql = %s", got)
	}
}

// TestManuallyAnnotateSnowflake ports test_manually_annotate_snowflake.
func TestManuallyAnnotateSnowflake(t *testing.T) {
	s := schema.M("SCHEMA", schema.M("TBL", schema.M("COL", "INT", "col2", "VARCHAR")))
	qual := qualifyFor(mustParse(t, `SELECT * FROM "SCHEMA"."TBL"`, "snowflake"), s, "snowflake")
	annotated := annotate(qual, s, "snowflake")
	if !annotated.Selects()[0].IsType("INT") || !annotated.Selects()[1].IsType("VARCHAR") {
		t.Errorf("types = %v, %v", annotated.Selects()[0].Type(), annotated.Selects()[1].Type())
	}
}

// TestStructFieldCaseSensitivityAnnotation ports the clickhouse cases of
// test_struct_field_case_sensitivity_annotation.
func TestStructFieldCaseSensitivityAnnotation(t *testing.T) {
	s := schema.M("t", schema.M("struct_col", "STRUCT<fooBar STRING>"))
	for _, tc := range []struct {
		sql  string
		want exp.DType
	}{
		// ClickHouse is case-sensitive: exact field name match
		{"SELECT struct_col.fooBar FROM t", exp.DTypeText},
		// ClickHouse: lower case
		{"SELECT struct_col.foobar FROM t", exp.DTypeUnknown},
	} {
		qualified := qualifyFor(mustParse(t, tc.sql, "clickhouse"), s, "clickhouse")
		assertThis(t, annotate(qualified, s, "clickhouse").Selects()[0], tc.want)
	}
}

// TestAnnotateObjectConstruct ports test_annotate_object_construct.
func TestAnnotateObjectConstruct(t *testing.T) {
	annotated := annotate(mustParse(t, "SELECT OBJECT_CONSTRUCT('foo', 'bar', 'a b', 'c d') AS c", "snowflake"), nil, "snowflake")
	if got := mustSQL(t, annotated.Selects()[0].Type(), "snowflake"); got != `OBJECT("foo" VARCHAR, "a b" VARCHAR)` {
		t.Errorf("type = %s", got)
	}
}

// TestNonnullAnnotation ports test_nonnull_annotation.
func TestNonnullAnnotation(t *testing.T) {
	nonnull := func(e exp.Expression) any { return e.MetaGet("nonnull") }

	for _, literal := range []string{"1", "'foo'", "2.5"} {
		if got := nonnull(annotate(mustParse(t, "SELECT "+literal, ""), nil, "").Selects()[0]); got != true {
			t.Errorf("literal %s nonnull = %v", literal, got)
		}
	}

	s := schema.M("foo", schema.M("id", "INT"))
	operandPairs := []struct {
		a, b string
		want any
	}{
		{"1", "1", true},
		{"foo.id", "foo.id", nil},
		{"1", "foo.id", nil},
		{"foo.id", "1", nil},
	}
	predicates := []string{">", "<", ">=", "<=", "=", "!=", "<>", "LIKE", "NOT LIKE"}

	for _, predicate := range predicates {
		for _, op := range operandPairs {
			sql := "SELECT " + op.a + " " + predicate + " " + op.b + " FROM foo"
			if got := nonnull(annotate(mustParse(t, sql, ""), s, "").Selects()[0]); got != op.want {
				t.Errorf("%s nonnull = %v, want %v", sql, got, op.want)
			}
		}
	}

	for _, predicate := range []string{"IS NULL", "IS NOT NULL"} {
		sql := "SELECT foo.id " + predicate + " FROM foo"
		if got := nonnull(annotate(mustParse(t, sql, ""), s, "").Selects()[0]); got != true {
			t.Errorf("%s nonnull = %v", sql, got)
		}
	}

	for _, connector := range []string{"AND", "OR"} {
		for _, predicate := range predicates {
			for _, op := range operandPairs {
				p := "(" + op.a + " " + predicate + " " + op.b + ")"
				sql := "SELECT " + p + " " + connector + " " + p + " FROM foo"
				if got := nonnull(annotate(mustParse(t, sql, ""), s, "").Selects()[0]); got != op.want {
					t.Errorf("%s nonnull = %v, want %v", sql, got, op.want)
				}
			}
		}
	}

	for _, unary := range []string{"NOT", "-"} {
		for _, v := range []struct {
			value string
			want  any
		}{{"1", true}, {"foo.id", nil}} {
			sql := "SELECT " + unary + " " + v.value + " FROM foo"
			if got := nonnull(annotate(mustParse(t, sql, ""), s, "").Selects()[0]); got != v.want {
				t.Errorf("%s nonnull = %v, want %v", sql, got, v.want)
			}
		}
	}

	chSchema := schema.M("t", schema.M("c1", "Int32", "c2", "Nullable(Int32)"))
	qualified := optimizer.QualifyColumns(mustParse(t, "select c1, c2 from t", ""), chSchema, true, true, boolPtr(true), false, "clickhouse")
	annotated := annotate(qualified, chSchema, "clickhouse")
	if got := nonnull(annotated.Selects()[0]); got != true {
		t.Errorf("c1 nonnull = %v", got)
	}
	if got := nonnull(annotated.Selects()[1]); got != nil {
		t.Errorf("c2 nonnull = %v", got)
	}
}

// TestDeepASTTypeAnnotation ports test_deep_ast_type_annotation.
func TestDeepASTTypeAnnotation(t *testing.T) {
	unionSQL := strings.Repeat("SELECT 1 UNION ALL ", 2000) + "SELECT 1"
	annotated := annotate(mustParse(t, unionSQL, ""), nil, "")
	if mustSQL(t, annotated, "") != unionSQL {
		t.Error("union sql changed")
	}
	assertThis(t, annotated.Selects()[0], exp.DTypeInt)

	binarySQL := "SELECT " + strings.Repeat("t.a + ", 2000) + "t.a FROM t"
	annotated = annotate(mustParse(t, binarySQL, ""), schema.M("t", schema.M("a", "INT")), "")
	if mustSQL(t, annotated, "") != binarySQL {
		t.Error("binary sql changed")
	}
	assertThis(t, annotated.Selects()[0], exp.DTypeInt)
}

// TestCorrelatedSubqueriesAnnotation ports the base-dialect case of
// test_correlated_subqueries_annotation.
func TestCorrelatedSubqueriesAnnotation(t *testing.T) {
	s := schema.M("t", schema.M("col", "TEXT"))
	qualified := qualifyFor(mustParse(t, "SELECT (SELECT col FROM t) as u FROM (SELECT 1 AS col) AS t", ""), s, "")
	annotated := annotate(qualified, s, "")
	if got := mustSQL(t, annotated, ""); got != `SELECT (SELECT "t"."col" AS "col" FROM "t" AS "t") AS "u" FROM (SELECT 1 AS "col") AS "t"` {
		t.Errorf("sql = %s", got)
	}
	assertTypeEqual(t, annotated.Selects()[0].Type(), buildType(t, "TEXT", ""))
}

// TestOrderByAliasAnnotation ports test_order_by_alias_annotation.
func TestOrderByAliasAnnotation(t *testing.T) {
	s := schema.M(
		"t", schema.M("x", "INT", "z", "TEXT", "category", "TEXT", "col", "INT"),
		"u", schema.M("a", "INT", "x", "INT"),
	)
	orderTypes := func(sql string) []string {
		query := qualifyFor(mustParse(t, sql, ""), s, "")
		annotated := annotate(query, s, "")
		order := annotated.Find(exp.KindOrder)
		if order == nil {
			t.Fatalf("No ORDER BY found in: %s", sql)
		}
		var out []string
		for _, o := range order.Expressions() {
			out = append(out, mustSQL(t, o.This().Type(), ""))
		}
		return out
	}
	for _, tc := range []struct {
		sql  string
		want string
	}{
		// Basic alias resolution
		{"SELECT x + 1 AS y FROM t ORDER BY y", "INT"},
		{"SELECT x, z FROM t ORDER BY x", "INT"},
		{"SELECT category, COUNT(*) AS cnt FROM t GROUP BY category ORDER BY cnt", "BIGINT"},
		{"SELECT CASE WHEN x > 0 THEN 'a' ELSE 'b' END AS label FROM t ORDER BY label", "VARCHAR"},
		{"SELECT CAST(x AS TEXT) AS s FROM t ORDER BY s", "TEXT"},
		// Alias shadows column name
		{"SELECT z AS x FROM t ORDER BY x", "TEXT"},
		// Alias shadows column from joined table (ambiguous column, alias wins)
		{"SELECT t.x + u.a AS a FROM t, u ORDER BY a", "INT"},
		// Alias shadows column across cross-join (u also has x)
		{"SELECT t.z AS x FROM t, u ORDER BY x", "TEXT"},
		// CTE name collides with alias
		{"WITH y AS (SELECT 999 AS v) SELECT x + 1 AS y FROM t ORDER BY y", "INT"},
		// Column name equals alias name (self-referential guard)
		{"SELECT x FROM t ORDER BY x", "INT"},
		// Multiple ORDER BY columns
		{"SELECT x + 1 AS y, z AS w FROM t ORDER BY y, w", "INT,TEXT"},
		// Sort modifiers
		{"SELECT x + 1 AS y FROM t ORDER BY y DESC", "INT"},
		{"SELECT x + 1 AS y FROM t ORDER BY y NULLS FIRST", "INT"},
		{"SELECT x + 1 AS y FROM t ORDER BY y DESC NULLS LAST", "INT"},
		// Three-column sort with mixed ASC/DESC
		{"SELECT x + 1 AS y, z AS w, category AS c FROM t ORDER BY y ASC, w DESC, c ASC", "INT,TEXT,TEXT"},
		// Compound expressions using aliases
		{"SELECT x + 1 AS y FROM t ORDER BY y + 1", "INT"},
		{"SELECT x + 1 AS y FROM t ORDER BY (y + 1) * 2", "INT"},
		{"SELECT x + 1 AS y FROM t ORDER BY ((y + 1) * 2) + 3", "INT"},
		{"SELECT x + 1 AS y FROM t ORDER BY ABS(y + 1)", "INT"},
		{"SELECT x + 1 AS y FROM t ORDER BY COALESCE(y, 0) + 1", "INT"},
		{"SELECT x AS a, z AS b FROM t ORDER BY CONCAT(b, CAST(a AS TEXT))", "VARCHAR"},
		// Non-projected column in ORDER BY
		{"SELECT x FROM t ORDER BY z", "TEXT"},
		// Mixed alias + expression
		{"SELECT x + 1 AS y FROM t ORDER BY y, x + 2", "INT,INT"},
		// GROUP BY + ORDER BY alias
		{"SELECT x + 1 AS y FROM t GROUP BY y ORDER BY y", "INT"},
		// Set operations
		{"SELECT x AS y FROM t UNION ALL SELECT a FROM u ORDER BY y", "INT"},
		{"SELECT x AS y FROM t UNION SELECT a FROM u ORDER BY y", "INT"},
		{"SELECT x AS y FROM t INTERSECT SELECT a FROM u ORDER BY y", "INT"},
		{"SELECT x AS y FROM t EXCEPT SELECT a FROM u ORDER BY y", "INT"},
		{"SELECT x AS y FROM t UNION ALL SELECT a FROM u ORDER BY y + 1", "INT"},
		{"SELECT x AS y FROM t UNION ALL SELECT a FROM u ORDER BY 1", "INT"},
		// Subquery with ORDER BY
		{"SELECT * FROM (SELECT x AS y FROM t ORDER BY y) AS sub", "INT"},
		// Window function alias (SUM is typed, unlike ROW_NUMBER)
		{"SELECT SUM(x) OVER () AS s FROM t ORDER BY s", "BIGINT"},
		// Subquery-as-projection alias
		{"SELECT (SELECT MAX(a) FROM u) AS m FROM t ORDER BY m", "INT"},
		// Alias name collides with table name
		{"SELECT (SELECT MAX(a) FROM u) AS u FROM t ORDER BY u", "INT"},
		// Type coercion through lazy annotation
		{"SELECT CASE WHEN x > 0 THEN x ELSE CAST(x AS BIGINT) END AS y FROM t ORDER BY y", "BIGINT"},
		// Subquery in ORDER BY with alias name clash (inner col is table-qualified)
		{"SELECT x AS a FROM t ORDER BY (SELECT MAX(a) FROM u)", "INT"},
		// CAST in ORDER BY using alias
		{"SELECT x AS y FROM t ORDER BY CAST(y AS TEXT)", "TEXT"},
	} {
		if got := strings.Join(orderTypes(tc.sql), ","); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.sql, got, tc.want)
		}
	}
}

// TestMultiplePivotsAnnotateTypes ports test_multiple_pivots_annotate_types.
func TestMultiplePivotsAnnotateTypes(t *testing.T) {
	// NOTE: the value column takes the type of the first IN-list entry, so the columns
	// folded by a single operator are kept uniformly typed here
	selectTypes := func(e exp.Expression) string {
		var out []string
		for _, s := range e.Selects() {
			out = append(out, s.AliasOrName()+" "+mustSQL(t, s.Type(), ""))
		}
		return strings.Join(out, ", ")
	}

	s := schema.M("t", schema.M("id", "int", "jan", "int", "feb", "int", "north", "double", "south", "double"))
	expression := annotate(qualifyFor(mustParse(t, `
                    SELECT * FROM t
                    UNPIVOT(revenue FOR month IN (jan, feb))
                    UNPIVOT(headcount FOR region IN (north, south))
                    `, "snowflake"), s, "snowflake"), s, "snowflake")
	// Types flow through every operator, not just the last one: the name columns are
	// text and each value column takes the type of the columns it was folded from
	if got := selectTypes(expression); got != "ID INT, MONTH VARCHAR, REVENUE INT, REGION VARCHAR, HEADCOUNT DOUBLE" {
		t.Errorf("unpivot types = %s", got)
	}

	pivotSchema := schema.M("t", schema.M("id", "int", "cat", "text", "val", "int", "kind", "text", "amt", "double"))
	expression = annotate(qualifyFor(mustParse(t, `
                    SELECT * FROM t
                    PIVOT(SUM(val) FOR cat IN ('a' AS a, 'b' AS b))
                    PIVOT(SUM(amt) FOR kind IN ('x' AS x, 'y' AS y))
                    `, "snowflake"), pivotSchema, "snowflake"), pivotSchema, "snowflake")
	// Each PIVOT's outputs take the type of its own aggregate
	if got := selectTypes(expression); got != "ID INT, A BIGINT, B BIGINT, X DOUBLE, Y DOUBLE" {
		t.Errorf("pivot types = %s", got)
	}
}
