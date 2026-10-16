package parser

import (
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// parsers/spark.py:55-58,121-123 on top of the Spark2 overlay (no parent chain here).
func init() {
	overrides := spark2ParserOverrideSet()
	overrides.NoParenFunctions = map[tokens.TokenType]func(exp.Args) exp.Expression{
		tokens.SESSION_USER: func(args exp.Args) exp.Expression { return exp.New(exp.KindSessionUser, args) },
	}
	overrides.FunctionParsers["SUBSTR"] = (*Parser).parseSubstring
	// DECLARE/REFRESH are base statement parsers reached through the Spark tokenizer keywords.
	overrides.StatementParsers = map[tokens.TokenType]parserOverrideFunc{
		tokens.DECLARE: (*Parser).parseDeclareStatement,
		tokens.REFRESH: (*Parser).parseRefresh,
	}
	registerDialectParserOverrides("spark", overrides)
}
