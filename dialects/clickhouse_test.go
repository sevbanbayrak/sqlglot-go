package dialects_test

import (
	"testing"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

func TestClickHouseConfig(t *testing.T) {
	d, err := dialects.GetOrRaise("clickhouse")
	if err != nil {
		t.Fatalf("GetOrRaise(clickhouse): %v", err)
	}
	if d.Name != "clickhouse" || d.Is("hive") {
		t.Fatalf("Name=%q Is(hive)=%v", d.Name, d.Is("hive"))
	}
	if d.IndexOffset != 1 || d.NormalizeFunctions != "" || d.NullOrdering != "nulls_are_last" || d.SupportsUserDefinedTypes ||
		d.LogBaseFirst != nil || !d.ForceEarlyAliasRefExpansion || !d.PreserveOriginalNames || !d.HexStringIsIntegerType ||
		d.NormalizationStrategy != dialects.CaseSensitive || d.CreatableKindMapping["DATABASE"] != "SCHEMA" {
		t.Fatalf("ClickHouse dialect flags not applied: %+v", d)
	}
	if d.ModifiersAttachedToSetOp || d.IntervalSpans || d.OptionalAliasTokenCTE || !d.JoinsHaveEqualPrecedence || !d.LogDefaultsToLn {
		t.Fatalf("ClickHouse parser flags not applied: %+v", d)
	}
	if d.QueryHints || d.StructDelimiter != [2]string{"(", ")"} || d.Nvl2Supported || d.TablesampleKeywords != "SAMPLE" ||
		d.GroupingsSep != "" || d.SetOpModifiers || d.ArraySizeName != "LENGTH" || !d.AutoRefreshBareIntervals || len(d.SelectKinds) != 0 {
		t.Fatalf("ClickHouse generator flags not applied: %+v", d)
	}
	// SET_OP_DISTINCT_BY_DEFAULT: EXCEPT/INTERSECT default to ALL, UNION must be explicit.
	if v := d.SetOpDistinctDefault(exp.KindExcept); v == nil || *v {
		t.Fatal("EXCEPT should default to ALL")
	}
	if d.SetOpDistinctDefault(exp.KindUnion) != nil {
		t.Fatal("UNION should have no default")
	}
	if v := dialects.Base().SetOpDistinctDefault(exp.KindUnion); v == nil || !*v {
		t.Fatal("base UNION should default to DISTINCT")
	}
	versioned, err := dialects.GetOrRaise("clickhouse, version=23.8")
	if err != nil || !versioned.VersionLess(23, 12) || versioned.VersionLess(23, 8) || d.VersionLess(23, 12) {
		t.Fatalf("version setting: err=%v", err)
	}
	if got := d.FormatTime("%Y-%mstrict-%dstrict"); got != "%Y-%m-%d" {
		t.Fatalf("strict time formats should degrade, got %q", got)
	}
}

func TestClickHouseTokenizer(t *testing.T) {
	d := dialects.ClickHouse()
	keywords := map[string]tokens.TokenType{
		".:": tokens.DOTCOLON, ".^": tokens.DOTCARET, "ATTACH": tokens.COMMAND, "FINAL": tokens.FINAL,
		"FLOAT32": tokens.FLOAT, "FLOAT64": tokens.DOUBLE, "GLOBAL": tokens.GLOBAL, "SAMPLE": tokens.TABLE_SAMPLE,
		"TUPLE": tokens.STRUCT, "UINT8": tokens.UTINYINT, "PROJECTION": tokens.PROJECTION, "PREWHERE": tokens.PREWHERE,
		"SYSTEM": tokens.COMMAND,
	}
	for keyword, want := range keywords {
		if got := d.TokenizerConfig.Keywords[keyword]; got != want {
			t.Errorf("keyword %s = %v, want %v", keyword, got, want)
		}
	}
	if _, ok := d.TokenizerConfig.Keywords["/*+"]; ok {
		t.Error("the /*+ hint keyword should be removed")
	}

	toks, err := d.NewTokenizer().Tokenize("SELECT `a`, 'x\\0y', 0x1F, 1_000 # c\n FROM t #! more")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	var types []tokens.TokenType
	for _, tok := range toks {
		types = append(types, tok.TokenType)
	}
	want := []tokens.TokenType{tokens.SELECT, tokens.IDENTIFIER, tokens.COMMA, tokens.STRING, tokens.COMMA, tokens.HEX_STRING, tokens.COMMA, tokens.NUMBER, tokens.FROM, tokens.VAR}
	if len(types) != len(want) {
		t.Fatalf("token types = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("token %d = %v, want %v (all: %v)", i, types[i], want[i], types)
		}
	}
	if toks[3].Text != "x\x00y" {
		t.Errorf(`\0 should unescape to NUL, got %q`, toks[3].Text)
	}
	if toks[7].Text != "1000" {
		t.Errorf("underscore-separated number = %q", toks[7].Text)
	}
	if len(toks[7].Comments) != 1 || toks[7].Comments[0] != " c" {
		t.Errorf("# comment not attached: %#v", toks[7].Comments)
	}
}
