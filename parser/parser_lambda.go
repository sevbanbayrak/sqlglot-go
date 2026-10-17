package parser

import (
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// lambdaArgTerminators ports LAMBDA_ARG_TERMINATORS (parser.py:1006): the fast-path guard
// in parseLambda that recognizes a plain (non-lambda) argument terminated by "," or ")".
var lambdaArgTerminators = map[tokens.TokenType]bool{
	tokens.COMMA:   true,
	tokens.R_PAREN: true,
}

// lambdas ports the base LAMBDAS table (parser.py:988-1001): ARROW -> exp.Lambda and FARROW ->
// exp.Kwarg (named arguments like `foo(bar => 1)`).
//
// Declared empty and populated from func init() (rather than a map literal referencing the
// closure directly): the closure calls parseDisjunction, which transitively calls back
// through parseLambda -> lambdas, and a literal initializer for that cycle is rejected by
// Go's package init-order analysis - the same reason dispatch (generator/dispatch.go) and
// statementParsers (parser.go) use this two-step pattern.
var lambdas = map[tokens.TokenType]func(*Parser, []exp.Expression) exp.Expression{}

func init() {
	lambdas[tokens.ARROW] = func(p *Parser, params []exp.Expression) exp.Expression {
		body := replaceLambda(p.parseDisjunction(), params)
		return p.expression(exp.Lambda(exp.Args{"this": body, "expressions": params}), nil, nil)
	}
	// parser.py:1064-1069 FARROW -> exp.Kwarg
	lambdas[tokens.FARROW] = func(p *Parser, params []exp.Expression) exp.Expression {
		name := ""
		if len(params) > 0 && params[0] != nil {
			name = params[0].Name()
		}
		body := p.parseDisjunction()
		if body == nil {
			body = p.parseSelect()
		}
		return p.expression(exp.New(exp.KindKwarg, exp.Args{"this": exp.Var(exp.Args{"this": name}), "expression": body}), nil, nil)
	}
}

// parseLambdaArg ports _parse_lambda_arg (parser.py:7178-7179): a lambda parameter is just
// an id-var; Snowflake also accepts `x INT` (parsers/snowflake.py).
func (p *Parser) parseLambdaArg() exp.Expression {
	if p.dialect.Is("snowflake") {
		return p.parseSnowflakeLambdaArg()
	}
	return p.parseIdVar(true, nil)
}

// lambdaFor is LAMBDAS, with Snowflake's ARROW override (parsers/snowflake.py:910-921): the body
// is an assignment and typed parameters are stored without their Cast.
func (p *Parser) lambdaFor(tokenType tokens.TokenType) (func(*Parser, []exp.Expression) exp.Expression, bool) {
	if tokenType == tokens.ARROW && p.dialect.Is("snowflake") {
		return func(p *Parser, params []exp.Expression) exp.Expression {
			body := replaceLambda(p.parseAssignment(), params)
			stripped := make([]exp.Expression, len(params))
			for i, param := range params {
				if param != nil && param.Kind() == exp.KindCast {
					param = param.This()
				}
				stripped[i] = param
			}
			return p.expression(exp.Lambda(exp.Args{"this": body, "expressions": stripped}), nil, nil)
		}, true
	}
	builder, ok := lambdas[tokenType]
	return builder, ok
}

// replaceLambda ports _replace_lambda (parser.py:9436-9464). Under TYPED_LAMBDA_ARGS
// (Snowflake) a typed parameter casts its uses in the body.
//
// For every exp.Column in the lambda body whose leading part (parts[0], i.e. the outermost
// qualifier - catalog/db/table/this in that priority) names a parameter, the column is
// replaced by its dot form: bare identifier when the column has no table qualifier, or the
// fully flattened dot chain (walking up through any wrapping exp.Dot ancestors) otherwise.
// This mirrors Column.to_dot()/column.this and Column.parts (expressions/builders.go:290,
// 323 columnPartsToDot; core.go:990 Parts) but needs its own walk here because to_dot's
// default include_dots=True also folds in outer exp.Dot ancestors that columnPartsToDot
// (include_dots=False) does not.
func replaceLambda(node exp.Expression, params []exp.Expression) exp.Expression {
	if node == nil {
		return nil
	}

	names := make(map[string]bool, len(params))
	types := make(map[string]exp.Expression, len(params))
	for _, param := range params {
		if param != nil {
			names[param.Name()] = true
			if to, ok := param.Arg("to").(exp.Expression); ok && to != nil {
				types[param.Name()] = to
			}
		}
	}

	for _, column := range node.FindAll(exp.KindColumn) {
		parts := column.Parts()
		if len(parts) == 0 || !names[parts[0].Name()] {
			continue
		}

		var dotOrID exp.Expression
		if column.Arg("table") != nil {
			dotOrID = columnToDot(column, parts)
		} else {
			dotOrID = column.This()
		}
		// parser.py:9448-9449: a typed lambda parameter (TYPED_LAMBDA_ARGS) casts its uses.
		if typ := types[parts[0].Name()]; typ != nil {
			dotOrID = exp.Cast(exp.Args{"this": dotOrID, "to": typ})
		}

		parent := column.Parent()
		if parent != nil && parent.Kind() == exp.KindDot {
			for {
				grandparent := parent.Parent()
				if grandparent == nil || grandparent.Kind() != exp.KindDot {
					parent.Replace(dotOrID)
					break
				}
				parent = grandparent
			}
		} else if column == node {
			node = dotOrID
		} else {
			column.Replace(dotOrID)
		}
	}

	return node
}

// columnToDot ports Column.to_dot(include_dots=True) (expressions/core.py:1724-1734): the
// column's own parts (catalog/db/table/this, whichever are present) followed by every
// expression hanging off an ancestor chain of exp.Dot nodes, flattened into a single dot
// chain via exp.DotBuild - or the lone identifier when there's nothing to flatten.
func columnToDot(column exp.Expression, parts []exp.Expression) exp.Expression {
	for parent := column.Parent(); parent != nil && parent.Kind() == exp.KindDot; parent = parent.Parent() {
		if expr := parent.Expr(); expr != nil {
			parts = append(parts, expr)
		}
	}
	if len(parts) == 1 {
		return parts[0].Copy()
	}
	copies := make([]exp.Expression, len(parts))
	for i, part := range parts {
		copies[i] = part.Copy()
	}
	return exp.DotBuild(copies)
}
