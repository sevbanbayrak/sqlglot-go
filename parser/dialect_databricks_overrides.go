package parser

import (
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// parsers/databricks.py on top of the Spark overlay (no parent chain here). The flags and the
// FUNCTIONS overlay live on dialects.Databricks.
func init() {
	overrides := sparkParserOverrideSet()
	overrides.NoParenFunctionParsers = map[string]parserOverrideFunc{"CURDATE": (*Parser).parseCurdate}
	for name, kind := range map[string]exp.Kind{"REGR_AVGX": exp.KindRegrAvgx, "REGR_SXX": exp.KindRegrSxx, "REGR_SYY": exp.KindRegrSyy} {
		kind := kind
		overrides.FunctionParsers[name] = func(p *Parser) exp.Expression { return p.parseDistinctArgFunction(kind, 1) }
	}
	for name, kind := range map[string]exp.Kind{"REGR_AVGY": exp.KindRegrAvgy, "REGR_SXY": exp.KindRegrSxy} {
		kind := kind
		overrides.FunctionParsers[name] = func(p *Parser) exp.Expression { return p.parseDistinctArgFunction(kind, 0) }
	}
	overrides.FactorTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{tokens.COLON: exp.JSONExtract}
	overrides.ColumnOperators = map[tokens.TokenType]columnOpFunc{
		tokens.QDCOLON: func(p *Parser, this, to exp.Expression) exp.Expression {
			return p.buildCast(false, exp.Args{"this": this, "to": to})
		},
	}
	overrides.CastColumnOperators = map[tokens.TokenType]bool{tokens.QDCOLON: true}
	registerDialectParserOverrides("databricks", overrides)
}

// parseCurdate ports _parse_curdate (parsers/databricks.py:60-64): CURDATE, an alias for
// CURRENT_DATE, has optional parentheses.
func (p *Parser) parseCurdate() exp.Expression {
	if p.match(tokens.L_PAREN) {
		p.matchRParen(nil)
	}
	return p.expression(exp.New(exp.KindCurrentDate, nil), nil, nil)
}

// buildJSONExtract ports _build_json_extract (parser.py:6918-6934). requires_json is
// JSON_EXTRACT_REQUIRES_JSON_EXPRESSION, False for every ported dialect, so it is not set.
func (p *Parser) buildJSONExtract(this exp.Expression, pathParts []exp.Expression) (exp.Expression, []exp.Expression) {
	if len(pathParts) > 1 {
		this = p.expression(exp.JSONExtract(exp.Args{
			"this":            this,
			"expression":      exp.New(exp.KindJSONPath, exp.Args{"expressions": pathParts}),
			"variant_extract": true,
		}), nil, nil)
		pathParts = []exp.Expression{exp.New(exp.KindJSONPathRoot, nil)}
	}
	return this, pathParts
}

// parseColonAsVariantExtract ports _parse_colon_as_variant_extract (parser.py:6936-6990).
func (p *Parser) parseColonAsVariantExtract(this exp.Expression) exp.Expression {
	pathParts := []exp.Expression{exp.New(exp.KindJSONPathRoot, nil)}
	keyTokens := map[tokens.TokenType]bool{tokens.SELECT: true}
	pathKey := func(key exp.Expression) exp.Expression {
		quoted, _ := key.Arg("quoted").(bool)
		quoted = quoted && key.Kind() == exp.KindIdentifier
		return exp.New(exp.KindJSONPathKey, exp.Args{"this": key.Name(), "quoted": quoted})
	}

	for p.match(tokens.COLON) {
		if !p.dialect.ColonChainIsSingleExtract {
			this, pathParts = p.buildJSONExtract(this, pathParts)
		}

		if key := p.parseIdVar(true, keyTokens); key != nil {
			pathParts = append(pathParts, pathKey(key))
		}

		for {
			if p.match(tokens.DOT) {
				if nextKey := p.parseIdVar(true, keyTokens); nextKey != nil {
					pathParts = append(pathParts, pathKey(nextKey))
				}
			} else if p.match(tokens.L_BRACKET) {
				bracketExpr := p.parseBracketKeyValue()
				if !p.match(tokens.R_BRACKET) {
					p.raiseError("Expected ]")
				}
				if bracketExpr == nil {
					continue
				}
				switch {
				case bracketExpr.IsString():
					pathParts = append(pathParts, exp.New(exp.KindJSONPathKey, exp.Args{"this": bracketExpr.Name(), "quoted": true}))
				case bracketExpr.IsStar():
					pathParts = append(pathParts, exp.New(exp.KindJSONPathSubscript, exp.Args{"this": exp.New(exp.KindJSONPathWildcard, nil)}))
				case bracketExpr.IsNumber():
					index := bracketExpr.ToPy()
					if i, ok := index.(int64); ok {
						index = int(i)
					}
					pathParts = append(pathParts, exp.New(exp.KindJSONPathSubscript, exp.Args{"this": index}))
				default:
					this, pathParts = p.buildJSONExtract(this, pathParts)
					this = p.expression(exp.Bracket(exp.Args{"this": this, "expressions": []exp.Expression{bracketExpr}, "json_access": true}), nil, nil)
				}
			} else if p.match(tokens.DCOLON) {
				this, pathParts = p.buildJSONExtract(this, pathParts)
				castType := p.parseTypes(false, false, true, false)
				if castType != nil {
					this = p.expression(exp.Cast(exp.Args{"this": this, "to": castType}), nil, nil)
				} else {
					p.raiseError("Expected type after '::'")
				}
			} else {
				break
			}
		}
	}

	this, _ = p.buildJSONExtract(this, pathParts)
	return this
}
