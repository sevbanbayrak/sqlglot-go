package dialects

import (
	"regexp"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// Databricks ports dialects/databricks.py and the parsers/databricks.py flags and FUNCTIONS
// overlay on top of Spark. Generator deltas live in generator/dialect_databricks.go, parser
// callbacks in parser/dialect_databricks_overrides.go. EXPRESSION_METADATA and COERCES_TO
// (typing) are out of scope until annotate_types.
func Databricks() *Dialect {
	d := Spark()
	d.Name = "databricks"
	// dialects/databricks.py:16-17.
	d.SafeDivision = false
	d.CopyParamsAreCsv = false
	// dialects/databricks.py:30-31.
	d.JSONPathIdentifiers = map[rune]string{'`': "`", '"': "\""}
	// parsers/databricks.py:11-14.
	d.LogDefaultsToLn = true
	d.StrictCast = true
	d.ColonIsVariantExtract = true
	d.ColonChainIsSingleExtract = false
	// generators/databricks.py:14-20.
	d.TablesampleSeedKeyword = "REPEATABLE"
	d.CopyParamsAreWrapped = false
	d.CopyParamsEqRequired = true
	d.JSONPathSingleQuoteEscape = false
	d.JSONPathKeyQuotedForcesBrackets = true
	// exp.SAFE_IDENTIFIER_RE (expressions/core.py:2810), Unicode-aware like the base default.
	d.SafeJSONPathKeyRE = regexp.MustCompile(`^[_a-zA-Z][\p{L}\p{N}_]*$`)
	d.QuoteJSONPath = false

	// parsers/databricks.py:16-28 FUNCTIONS overlay.
	for name, builder := range map[string]func([]exp.Expression) exp.Expression{
		"IFF":       exp.FromArgListFunc(exp.KindIf),
		"GETDATE":   exp.FromArgListFunc(exp.KindCurrentTimestamp),
		"DATEDIFF":  sparkBuildDateDelta(exp.KindDateDiff),
		"DATE_DIFF": sparkBuildDateDelta(exp.KindDateDiff),
		"NOW":       exp.FromArgListFunc(exp.KindCurrentTimestamp),
		"TO_DATE": func(args []exp.Expression) exp.Expression {
			return hiveBuildFormattedTime(exp.KindTsOrDsToDate, hiveSeqGet(args, 0), hiveSeqGet(args, 1), false)
		},
		"UNIFORM": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindUniform, exp.Args{"this": hiveSeqGet(args, 0), "expression": hiveSeqGet(args, 1), "seed": hiveSeqGet(args, 2)})
		},
	} {
		d.Functions[name] = builder
	}

	cfg := d.TokenizerConfig
	// dialects/databricks.py:33-38.
	cfg.Keywords["STREAM"] = tokens.STREAM
	cfg.Keywords["VOID"] = tokens.VOID
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}
