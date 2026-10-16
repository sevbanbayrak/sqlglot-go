package dialects_test

import (
	"testing"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

func TestDatabricksConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("databricks")
	if err != nil {
		t.Fatalf("GetOrRaise(databricks): %v", err)
	}
	if d.Name != "databricks" || !d.Is("spark") || !d.Is("hive") {
		t.Fatalf("Name=%q Is(spark)=%v Is(hive)=%v", d.Name, d.Is("spark"), d.Is("hive"))
	}
	if d.SafeDivision || d.CopyParamsAreCsv || !d.LogDefaultsToLn || !d.StrictCast || !d.ColonIsVariantExtract || d.ColonChainIsSingleExtract {
		t.Fatalf("Databricks dialect/parser flags not applied: %+v", d)
	}
	if d.TablesampleSeedKeyword != "REPEATABLE" || d.CopyParamsAreWrapped || !d.CopyParamsEqRequired || d.QuoteJSONPath || !d.JSONPathKeyQuotedForcesBrackets || d.JSONPathSingleQuoteEscape {
		t.Fatalf("Databricks generator flags not applied: %+v", d)
	}
	if d.TokenizerConfig.Keywords["STREAM"] != tokens.STREAM || d.TokenizerConfig.Keywords["VOID"] != tokens.VOID {
		t.Fatal("STREAM/VOID keywords missing")
	}
	spark := dialects.Spark()
	if _, ok := spark.TokenizerConfig.Keywords["STREAM"]; ok || spark.ColonIsVariantExtract || !spark.SafeDivision {
		t.Fatal("Databricks construction leaked into Spark")
	}

	// dialects/databricks.py:30-31: the JSONPath tokenizer also accepts backtick identifiers.
	path := d.ToJSONPath(exp.LiteralString("$.`a b`.c"))
	if path.Kind() != exp.KindJSONPath || len(path.Expressions()) != 3 || path.Expressions()[1].Name() != "a b" {
		t.Fatalf("backtick JSONPath key not parsed: %s", path.ToS())
	}
}
