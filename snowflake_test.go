package sqlglot_test

import (
	"testing"

	sqlglot "github.com/sevbanbayrak/sqlglot-go"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/optimizer"
)

// Ports the AST assertions of tests/dialects/test_snowflake.py; its validate_identity and
// validate_all calls are covered by the dialect and transpile corpora.

func sfParse(t *testing.T, sql string) exp.Expression {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, "snowflake")
	if err != nil {
		t.Fatalf("ParseOne(%q): %v", sql, err)
	}
	return e
}

func sfIdentity(t *testing.T, sql string, want ...string) exp.Expression {
	t.Helper()
	e := sfParse(t, sql)
	expected := sql
	if len(want) > 0 {
		expected = want[0]
	}
	if got := chSQL(t, e, "snowflake"); got != expected {
		t.Errorf("%q: got %q, want %q", sql, got, expected)
	}
	return e
}

func sfName(e any) string {
	if n := asExpr(e); n != nil {
		return n.Name()
	}
	return ""
}

func TestSnowflakeAST(t *testing.T) {
	// test_snowflake:34-36
	dateadd := sfParse(t, "DATEADD(DAY, n, d)")
	dateadd.Set("unit", exp.LiteralString("MONTH"))
	if got := chSQL(t, dateadd, "snowflake"); got != "DATEADD(MONTH, n, d)" {
		t.Errorf("DATEADD unit = %s", got)
	}

	// test_snowflake:79, 464, 498
	if !sfIdentity(t, "APPROX_TOP_K(C4, 3, 5)").Is(exp.TraitAggFunc) {
		t.Error("APPROX_TOP_K is not an AggFunc")
	}
	chKind(t, sfIdentity(t, "SELECT number").Selects()[0], exp.KindColumn)
	chKind(t, sfIdentity(t, "SELECT TO_TIMESTAMP(123.4)").Selects()[0], exp.KindAnonymous)

	// test_snowflake:510-617
	for _, tc := range []struct {
		sql, format, precision, scale string
		safe                          bool
	}{
		{"TO_NUMBER('12.3456')", "", "38", "0", false},
		{"TO_NUMBER('12.3456', 10, 1)", "", "10", "1", false},
		{"TO_NUMBER('12.3456', '99.99')", "99.99", "38", "0", false},
		{"TO_NUMBER('12.3456', '99.99', 10, 1)", "99.99", "10", "1", false},
		{"TO_NUMBER('12.3456', 3)", "", "3", "0", false},
		{"TRY_TO_NUMBER('12.3456')", "", "38", "0", true},
		{"TRY_TO_NUMBER('12.3456', 10, 1)", "", "10", "1", true},
		{"TRY_TO_NUMBER('12.3456', '99.99')", "99.99", "38", "0", true},
		{"TRY_TO_NUMBER('12.3456', '99.99', 10, 1)", "99.99", "10", "1", true},
		{"TRY_TO_NUMBER('12.3456', 3)", "", "3", "0", true},
	} {
		ast := sfIdentity(t, tc.sql)
		chKind(t, ast, exp.KindToNumber)
		if sfName(ast.Arg("format")) != tc.format || sfName(ast.Arg("precision")) != tc.precision ||
			sfName(ast.Arg("scale")) != tc.scale || (ast.Arg("safe") == true) != tc.safe {
			t.Errorf("%s: %s", tc.sql, ast.ToS())
		}
		if tc.format == "" && ast.Arg("format") != nil {
			t.Errorf("%s: unexpected format", tc.sql)
		}
	}

	// test_snowflake:842-844
	chKind(t, asExpr(sfIdentity(t, "SELECT DATEADD(DAY, -7, DATEADD(t.m, 1, CAST('2023-01-03' AS DATE))) FROM (SELECT 'month' AS m) AS t").Selects()[0].This().Arg("unit")), exp.KindColumn)

	// test_snowflake:862-864
	stage := asExpr(sfIdentity(t, "SELECT FILE_URL FROM DIRECTORY(@mystage) WHERE SIZE > 100000").Arg("from_")).This().This()
	chKind(t, stage, exp.KindDirectoryStage)
	chKind(t, stage.This(), exp.KindVar)

	// test_snowflake:2298-2299, 3989-4014
	chKind(t, sfIdentity(t, "TRY_TO_TIMESTAMP(foo)"), exp.KindAnonymous)
	chKind(t, sfIdentity(t, "TRY_TO_TIMESTAMP('12345')"), exp.KindAnonymous)
	chKind(t, sfIdentity(t, "TO_DATE('12345')"), exp.KindAnonymous)
	chKind(t, sfIdentity(t, "TO_DATE(x)"), exp.KindTsOrDsToDate)
	chKind(t, sfIdentity(t, "TRY_TO_DATE(x)"), exp.KindTsOrDsToDate)

	// test_snowflake:2482-2486
	if _, err := sqlglot.ParseOne("SELECT id, PRIOR name AS parent_name, name FROM tree CONNECT BY NOCYCLE PRIOR id = parent_id", "snowflake"); err == nil {
		t.Error("CONNECT BY NOCYCLE PRIOR: expected a ParseError")
	}

	// test_snowflake:3503-3508
	staged := sfParse(t, "SELECT * FROM @foo")
	before := chSQL(t, staged, "snowflake")
	if got := chSQL(t, optimizer.NormalizeIdentifiers(staged, "snowflake"), "snowflake"); got != before {
		t.Errorf("normalized staged file = %s, want %s", got, before)
	}

	// test_snowflake:3652-3662
	for sql, want := range map[string]string{
		"SELECT DATE_PART(year FROM CAST('2024-04-08' AS DATE))":    "SELECT DATE_PART(year, CAST('2024-04-08' AS DATE))",
		"SELECT DATE_PART('month' FROM CAST('2024-04-08' AS DATE))": "SELECT DATE_PART('month', CAST('2024-04-08' AS DATE))",
		"SELECT DATE_PART(day FROM a)":                              "SELECT DATE_PART(day, a)",
	} {
		chKind(t, sfIdentity(t, sql, want).Expressions()[0], exp.KindExtract)
	}

	// test_snowflake:4082-4096
	for _, sql := range []string{"TRUNC(3.14159, 2)", "TRUNC(price, 0)", "TRUNC(3.14159)", "TRUNC(col)"} {
		chKind(t, sfIdentity(t, sql), exp.KindTrunc)
	}
	for _, typ := range []string{"DATE", "TIMESTAMP", "DATETIME"} {
		chKind(t, sfParse(t, "TRUNC(CAST(x AS "+typ+"), 'MONTH')"), exp.KindDateTrunc)
	}
	chKind(t, sfIdentity(t, "TRUNC(foo, bar)"), exp.KindAnonymous)

	// test_snowflake:4367-4392
	chKind(t, sfIdentity(t, "CREATE WAREHOUSE x").This(), exp.KindIdentifier)
	chKind(t, sfIdentity(t, "CREATE STREAMLIT x").This(), exp.KindIdentifier)
	chKind(t, sfIdentity(t, "CREATE TEMPORARY STAGE stage1 FILE_FORMAT=(TYPE=PARQUET)").This(), exp.KindTable)
	// Upstream raises a ParseError; the port's CREATE degrade path yields a Command (DEVIATIONS §5).
	chKind(t, sfParse(t, "CREATE STAGE stage1 FILE_FORMAT=123"), exp.KindCommand)
	chKind(t, sfIdentity(t, "CREATE OR REPLACE TAG IF NOT EXISTS cost_center COMMENT='cost_center tag'").This(), exp.KindIdentifier)
	chKind(t, sfIdentity(t, "CREATE TEMPORARY FILE FORMAT fileformat1 TYPE=PARQUET COMPRESSION=auto").This(), exp.KindTable)

	// test_snowflake:4845-4867
	for _, tc := range []struct {
		kind, object, last string
		prop               exp.Kind
	}{
		{"DYNAMIC TABLE", "db.schema.t1", "TABLE", exp.KindDynamicProperty},
		{"MATERIALIZED VIEW", "my_view", "VIEW", exp.KindMaterializedProperty},
		{"EXTERNAL VOLUME", "vol1", "VOLUME", exp.KindExternalProperty},
		{"COMPUTE POOL", "pool1", "POOL", exp.KindComputeProperty},
		{"MASKING POLICY", "db.schema.pol1", "POLICY", exp.KindMaskingProperty},
		{"ROW ACCESS POLICY", "pol1", "POLICY", exp.KindRowAccessProperty},
		{"API INTEGRATION", "int1", "INTEGRATION", exp.KindApiProperty},
		{"APPLICATION PACKAGE", "pkg1", "PACKAGE", exp.KindApplicationProperty},
		{"SECURITY INTEGRATION", "int1", "INTEGRATION", exp.KindSecurityIntegrationProperty},
		{"NETWORK RULE", "rule1", "RULE", exp.KindNetworkProperty},
		{"ICEBERG TABLE", "db.schema.t1", "TABLE", exp.KindIcebergProperty},
		{"HYBRID TABLE", "t1", "TABLE", exp.KindHybridProperty},
		{"CATALOG INTEGRATION", "int1", "INTEGRATION", exp.KindCatalogProperty},
		{"DATABASE ROLE", "role1", "ROLE", exp.KindDatabaseProperty},
	} {
		sql := "DESCRIBE " + tc.kind + " " + tc.object
		ast := sfIdentity(t, sql)
		if ast.Text("kind") != tc.last || ast.Find(tc.prop) == nil {
			t.Errorf("%s: %s", sql, ast.ToS())
		}
		sfIdentity(t, "DESC "+tc.kind+" "+tc.object, sql)
	}

	// test_snowflake:5043-5070
	for _, tc := range []struct {
		sql, want          string
		analyzer, searchMd bool
	}{
		{"SELECT SEARCH(line, 'king')", "", false, false},
		{"SELECT SEARCH(line, 'king', ANALYZER => 'UNICODE_ANALYZER')", "", true, false},
		{"SELECT SEARCH(character, 'king queen', SEARCH_MODE => 'AND')", "", false, true},
		{"SELECT SEARCH(line, 'king', SEARCH_MODE => 'AND', ANALYZER => 'PATTERN_ANALYZER')", "SELECT SEARCH(line, 'king', ANALYZER => 'PATTERN_ANALYZER', SEARCH_MODE => 'AND')", true, true},
	} {
		want := []string{}
		if tc.want != "" {
			want = append(want, tc.want)
		}
		search := sfIdentity(t, tc.sql, want...).Find(exp.KindSearch)
		if search == nil || search.This() == nil || search.Arg("expression") == nil ||
			(search.Arg("analyzer") != nil) != tc.analyzer || (search.Arg("search_mode") != nil) != tc.searchMd {
			t.Errorf("%s: %v", tc.sql, search)
		}
	}

	// test_snowflake:5321-5552
	for sql, want := range map[string][2]string{
		"SHOW DATABASES IN ACCOUNT":    {"DATABASES", "ACCOUNT"},
		"SHOW FILE FORMATS IN ACCOUNT": {"FILE FORMATS", "ACCOUNT"},
		"SHOW FUNCTIONS IN ACCOUNT":    {"FUNCTIONS", "ACCOUNT"},
		"SHOW PROCEDURES IN ACCOUNT":   {"PROCEDURES", "ACCOUNT"},
		"SHOW STAGES IN ACCOUNT":       {"STAGES", "ACCOUNT"},
		"SHOW WAREHOUSES":              {"WAREHOUSES", ""},
	} {
		ast := sfParse(t, sql)
		if ast.Name() != want[0] || ast.Text("scope_kind") != want[1] {
			t.Errorf("%s: %s", sql, ast.ToS())
		}
	}
	for sql, want := range map[string][2]string{
		"SHOW SCHEMAS IN DATABASE db1": {"DATABASE", "db1"},
		"SHOW OBJECTS IN db1.schema1":  {"SCHEMA", "db1.schema1"},
		"SHOW SEQUENCES IN dt_test":    {"SCHEMA", ""},
	} {
		ast := sfParse(t, sql)
		if ast.Text("scope_kind") != want[0] {
			t.Errorf("%s: scope_kind %q", sql, ast.Text("scope_kind"))
		}
		if want[1] != "" && chSQL(t, ast.Find(exp.KindTable), "snowflake") != want[1] {
			t.Errorf("%s: %s", sql, ast.ToS())
		}
	}
	columns := sfParse(t, "SHOW COLUMNS LIKE '_testing%' IN dt_test")
	if chSQL(t, columns.Find(exp.KindTable), "snowflake") != "dt_test" || chSQL(t, columns.Find(exp.KindLiteral), "snowflake") != "'_testing%'" {
		t.Errorf("SHOW COLUMNS: %s", columns.ToS())
	}
	for _, tc := range []struct {
		sql            string
		terse, iceberg bool
	}{
		{"SHOW TABLES IN db1.schema1", false, false},
		{"SHOW ICEBERG TABLES IN db1.schema1", false, true},
		{"SHOW TERSE ICEBERG TABLES IN db1.schema1", true, true},
	} {
		ast := sfParse(t, tc.sql)
		if chSQL(t, ast.Find(exp.KindTable), "snowflake") != "db1.schema1" ||
			(ast.Arg("iceberg") == true) != tc.iceberg || (tc.terse && ast.Arg("terse") != true) {
			t.Errorf("%s: %s", tc.sql, ast.ToS())
		}
	}

	// test_snowflake:5441-5462
	if sfIdentity(t, "ALTER ICEBERG TABLE t RENAME TO x").Arg("iceberg") != true {
		t.Error("ALTER ICEBERG TABLE: iceberg not set")
	}
	chKind(t, sfParse(t, "ALTER ICEBERG VIEW v RENAME TO x"), exp.KindCommand)
	if sfIdentity(t, "DROP ICEBERG TABLE t").Arg("iceberg") != true {
		t.Error("DROP ICEBERG TABLE: iceberg not set")
	}

	// test_snowflake:5555-5568
	chKind(t, sfParse(t, "CREATE STORAGE INTEGRATION s3_int TYPE=EXTERNAL_STAGE STORAGE_PROVIDER='S3' ENABLED=TRUE").This(), exp.KindIdentifier)
	swap := sfParse(t, "ALTER TABLE a SWAP WITH b")
	chKind(t, swap, exp.KindAlter)
	chKind(t, listOf(swap.Arg("actions"))[0], exp.KindSwapTable)

	// test_snowflake:6176
	chKind(t, sfIdentity(t, "GET(foo, bar)"), exp.KindGetExtract)
}

func TestSnowflakeStageCommands(t *testing.T) {
	stageProps := func(e exp.Expression) map[string]string {
		out := map[string]string{}
		for _, prop := range asExpr(e.Arg("properties")).Expressions() {
			value := asExpr(prop.Arg("value"))
			out[prop.This().Name()] = value.ToS()
		}
		return out
	}

	// test_snowflake:5707-5745
	put := sfIdentity(t, "PUT 'file://my file.txt' '@s1/my folder'")
	chKind(t, put, exp.KindPut)
	if !put.This().IsString() || put.This().Name() != "file://my file.txt" || asExpr(put.Arg("target")).Kind() != exp.KindVar || sfName(put.Arg("target")) != "'@s1/my folder'" {
		t.Errorf("PUT: %s", put.ToS())
	}
	put = sfParse(t, "PUT 'file:///tmp/my.txt' @stage1/folder PARALLEL = 1 AUTO_COMPRESS=false source_compression=gzip OVERWRITE=TRUE")
	chKind(t, put, exp.KindPut)
	if put.This().Name() != "file:///tmp/my.txt" || sfName(put.Arg("target")) != "@stage1/folder" {
		t.Errorf("PUT: %s", put.ToS())
	}
	props := stageProps(put)
	if len(props) != 4 || props["PARALLEL"] != "Literal(this=1, is_string=False)" || props["AUTO_COMPRESS"] != "Boolean(this=False)" ||
		props["source_compression"] != "Var(this=gzip)" || props["OVERWRITE"] != "Boolean(this=True)" {
		t.Errorf("PUT properties: %v", props)
	}
	chKind(t, sfParse(t, "PUT file:///dir/tmp.csv @%table"), exp.KindCommand)

	// test_snowflake:5747-5770
	chKind(t, sfIdentity(t, "GET @s1/test 'file:///dir/tmp.csv'"), exp.KindGet)
	get := sfIdentity(t, "GET '@s1/my folder' 'file://my file.txt'")
	chKind(t, get, exp.KindGet)
	if sfName(get.Arg("target")) != "'@s1/my folder'" || get.This().Name() != "file://my file.txt" {
		t.Errorf("GET: %s", get.ToS())
	}
	get = sfParse(t, "GET @stage1/folder 'file:///tmp/my.txt' PARALLEL = 1")
	if props := stageProps(get); len(props) != 1 || props["PARALLEL"] != "Literal(this=1, is_string=False)" {
		t.Errorf("GET properties: %v", props)
	}
	chKind(t, sfParse(t, "GET @%table file:///dir/tmp.csv"), exp.KindCommand)
}

func TestSnowflakeMiscAST(t *testing.T) {
	// test_snowflake:5962-5970
	query := "SELECT * FROM TABLE(db.schema.FUNC(a) OVER ())"
	ast := sfIdentity(t, query)
	if n := len(ast.FindAll(exp.KindColumn)); n != 1 {
		t.Errorf("columns = %d", n)
	}
	if got := chSQL(t, ast.Find(exp.KindWindow).This(), "snowflake"); got != "db.schema.FUNC(a)" {
		t.Errorf("window.this = %s", got)
	}

	// test_snowflake:6006-6008
	placeholder := sfIdentity(t, "SELECT :1").Find(exp.KindPlaceholder)
	if placeholder == nil || placeholder.Name() != "1" {
		t.Errorf("placeholder = %v", placeholder)
	}

	// test_snowflake:6012-6026
	for _, sql := range []string{"MAX_BY(DISTINCT selected_col, filtered_col)", "MIN_BY(DISTINCT selected_col, filtered_col)"} {
		node := sfIdentity(t, sql)
		if len(node.This().Expressions()) != 1 || asExpr(node.Arg("expression")).Kind() != exp.KindColumn {
			t.Errorf("%s: %s", sql, node.ToS())
		}
	}
	for _, sql := range []string{"MAX_BY(selected_col, filtered_col, 5)", "MIN_BY(selected_col, filtered_col, 3)"} {
		if sfIdentity(t, sql).Arg("count") == nil {
			t.Errorf("%s: count missing", sql)
		}
	}

	// test_snowflake:6091-6094
	// Upstream raises a ParseError; the port's CREATE degrade path yields a Command (DEVIATIONS §5).
	chKind(t, sfParse(t, "CREATE VIEW v WITH ROW ACCESS POLICY p AS SELECT 1"), exp.KindCommand)

	// test_snowflake:6251-6260
	alterSession := sfParse(t, "ALTER SESSION SET autocommit = FALSE").Find(exp.KindSetItem)
	if alterSession == nil || alterSession.Arg("kind") != nil {
		t.Errorf("ALTER SESSION SET item = %v", alterSession)
	}
	set := sfParse(t, "SET a = 1").Find(exp.KindSetItem)
	if set == nil || set.Text("kind") != "VARIABLE" {
		t.Errorf("SET item = %v", set)
	}
}

func TestMultitableInserts(t *testing.T) {
	// test_oracle:685-760; multi-table INSERT is base grammar that Snowflake shares.
	for _, sql := range []string{
		"INSERT ALL INTO dest_tab1 (id, description) VALUES (id, description) INTO dest_tab2 (id, description) VALUES (id, description) SELECT id, description FROM source_tab",
		"INSERT ALL WHEN id <= 3 THEN INTO dest_tab1 (id, description) VALUES (id, description) WHEN id BETWEEN 4 AND 7 THEN INTO dest_tab2 (id, description) VALUES (id, description) SELECT id, description FROM source_tab",
		"INSERT FIRST WHEN id <= 3 THEN INTO dest_tab1 (id, description) VALUES (id, description) ELSE INTO dest_tab2 (id, description) VALUES (id, description) INTO dest_tab3 (id, description) VALUES (id, description) SELECT id, description FROM source_tab",
		"/* COMMENT */ INSERT FIRST WHEN salary > 4000 THEN INTO emp2 WHEN salary > 5000 THEN INTO emp3 WHEN salary > 6000 THEN INTO emp4 SELECT salary FROM employees",
	} {
		ast := sfIdentity(t, sql)
		chKind(t, ast, exp.KindMultitableInserts)
		chKind(t, listOf(ast.Arg("expressions"))[0], exp.KindConditionalInsert)
	}
}
