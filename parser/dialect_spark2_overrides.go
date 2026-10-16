package parser

import (
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// parsers/spark2.py:86-98 FUNCTION_PARSERS on top of the Hive overlay (no parent chain here).
func init() {
	registerDialectParserOverrides("spark2", spark2ParserOverrideSet())
}

func spark2ParserOverrideSet() dialectParserOverrideSet {
	overrides := hiveParserOverrideSet()
	overrides.FunctionParsers["AND"] = func(p *Parser) exp.Expression { return p.parseConnectorFunction(exp.And_) }
	overrides.FunctionParsers["OR"] = func(p *Parser) exp.Expression { return p.parseConnectorFunction(exp.Or_) }
	overrides.FunctionParsers["APPROX_PERCENTILE"] = func(p *Parser) exp.Expression {
		return p.parseDistinctArgFunction(exp.KindApproxQuantile)
	}
	for _, hint := range []string{"BROADCAST", "BROADCASTJOIN", "MAPJOIN", "MERGE", "SHUFFLEMERGE", "MERGEJOIN", "SHUFFLE_HASH", "SHUFFLE_REPLICATE_NL"} {
		name := hint
		overrides.FunctionParsers[name] = func(p *Parser) exp.Expression { return p.parseJoinHint(name) }
	}
	return overrides
}

// parseConnectorFunction ports _parse_connector_function (parser.py:7180-7186).
func (p *Parser) parseConnectorFunction(connector func(...exp.Expression) exp.Expression) exp.Expression {
	args := p.parseFunctionArgs(false)
	if len(args) == 0 {
		p.raiseError("Expected at least one argument")
	}
	return exp.Paren(exp.Args{"this": connector(args...)})
}

// parseDistinctArgFunction ports _parse_distinct_arg_function (parser.py:10109-10121) with
// distinct_index=0.
func (p *Parser) parseDistinctArgFunction(kind exp.Kind) exp.Expression {
	isDistinct := p.match(tokens.DISTINCT)
	if !isDistinct {
		p.match(tokens.ALL)
	}
	args := []exp.Expression{p.parseLambda(false)}
	if p.match(tokens.COMMA) {
		args = append(args, p.parseFunctionArgs(false)...)
	}
	if isDistinct && args[0] != nil {
		args[0] = p.expression(exp.Distinct(exp.Args{"expressions": []exp.Expression{args[0]}}), nil, nil)
	}
	return exp.FromArgList(kind, args)
}

// parseJoinHint ports _parse_join_hint (parser.py:8525-8527).
func (p *Parser) parseJoinHint(name string) exp.Expression {
	args := p.parseCsv(func() exp.Expression { return p.parseTable(false, false, nil, false, false, false, false) })
	return exp.JoinHint(exp.Args{"this": strings.ToUpper(name), "expressions": args})
}
