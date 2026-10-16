package dialects_test

import (
	"testing"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

func TestSparkConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("spark")
	if err != nil {
		t.Fatalf("GetOrRaise(spark): %v", err)
	}
	if d.Name != "spark" || !d.Is("spark2") || !d.Is("hive") {
		t.Fatalf("Name=%q Is(spark2)=%v Is(hive)=%v", d.Name, d.Is("spark2"), d.Is("hive"))
	}
	if !d.SupportsOrderByAll || !d.SupportsLimitAll || !d.ArrayFuncsPropagatesNulls || d.LenientTimeMapping || d.PadFillPatternIsRequired {
		t.Fatalf("Spark dialect flags not applied: %+v", d)
	}
	if d.TokenizerConfig.FormatStrings["r'"].TokenType != tokens.RAW_STRING || d.TokenizerConfig.StringEscapesAllowedInRawStrings {
		t.Fatal("raw strings not configured")
	}
	if d.TokenizerConfig.Keywords["DECLARE"] != tokens.DECLARE {
		t.Fatal("DECLARE keyword missing")
	}
	spark2 := dialects.Spark2()
	if spark2.SupportsLimitAll || !spark2.LenientTimeMapping {
		t.Fatal("Spark construction leaked into Spark2")
	}
}
