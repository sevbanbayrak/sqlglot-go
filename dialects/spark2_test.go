package dialects_test

import (
	"testing"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

func TestSpark2ConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("spark2")
	if err != nil {
		t.Fatalf("GetOrRaise(spark2): %v", err)
	}
	if d.Name != "spark2" || !d.Is("hive") || d.Is("presto") {
		t.Fatalf("Name=%q Is(hive)=%v Is(presto)=%v", d.Name, d.Is("hive"), d.Is("presto"))
	}
	if d.AlterTableSupportsCascade || !d.TrimPatternFirst || !d.ChangeColumnAlterSyntax || !d.LenientTimeMapping {
		t.Fatalf("Spark2 parser/dialect flags not applied: %+v", d)
	}
	if !d.FuncTokens[tokens.AND] || !d.FuncTokens[tokens.OR] {
		t.Fatal("AND/OR must be FUNC_TOKENS in Spark2")
	}
	if d.TokenizerConfig.Keywords["TIMESTAMP"] != tokens.TIMESTAMPTZ {
		t.Fatal("TIMESTAMP keyword must tokenize as TIMESTAMPTZ")
	}
	if d.TokenizerConfig.FormatStrings["X'"].TokenType != tokens.HEX_STRING || d.HexStart != "X'" {
		t.Fatal("X'..' hex strings not configured")
	}
	hive := dialects.Hive()
	if hive.Name != "hive" || hive.TrimPatternFirst || hive.TokenizerConfig.Keywords["TIMESTAMP"] == tokens.TIMESTAMPTZ {
		t.Fatal("Spark2 construction leaked into Hive")
	}
}
