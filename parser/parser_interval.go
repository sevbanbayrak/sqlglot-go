package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

var intervalStringRE = regexp.MustCompile(`\s*(-?[0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]+)\s*`)

// pyNumberString renders a Go number the way Python's str() would, so INTERVAL
// literal canonicalization matches upstream (e.g. 1.0 -> "1.0", not "1").
func pyNumberString(v any) string {
	switch n := v.(type) {
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		s := strconv.FormatFloat(n, 'g', -1, 64)
		// Python renders integer-valued floats with a trailing ".0".
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	default:
		return fmt.Sprint(v)
	}
}

func (p *Parser) parseIntervalSpan(this exp.Expression) exp.Expression {
	return p.parseIntervalSpanWith(this, true)
}

func (p *Parser) parseIntervalSpanWith(this exp.Expression, parseFunctionUnit bool) exp.Expression {
	// is_unit gate (parser.py:4054-4060, v30.17.0): only attempt a unit when the current
	// token can actually be one (VAR or a dialect-valid unit word), so keyword-token
	// pseudo-functions no longer swallow the position — `INTERVAL '-1' CURRENT_DATE`
	// parses as Alias(Interval, CURRENT_DATE) and `INTERVAL '1' CURRENT_TIMESTAMP()`
	// errors, both matching pinned upstream.
	isUnit := p.curr.IsValid() && (p.curr.TokenType == tokens.VAR || p.dialect.ValidIntervalUnits[stringsUpper(p.curr.Text)])
	var unit exp.Expression
	if isUnit {
		if parseFunctionUnit {
			unit = p.parseFunction(nil, false, true, false)
		}
		if unit == nil {
			unit = p.parseVar(true, nil, true)
		}
	}

	// Most dialects support, e.g., the form INTERVAL '5' day, thus we try to parse
	// each INTERVAL expression into this canonical form so it's easy to transpile.
	if this != nil && this.IsNumber() {
		// Mirror upstream `exp.Literal.string(this.to_py())`: Python's str() of the
		// number, which keeps decimals (1.0 -> "1.0") and the sign for Neg literals.
		// fmt.Sprint on a float64 would drop the trailing ".0", so format explicitly.
		this = exp.LiteralString(pyNumberString(this.ToPy()))
	} else if this != nil && this.IsString() {
		parts := intervalStringRE.FindAllStringSubmatch(this.Name(), -1)
		if len(parts) > 0 && unit != nil {
			unit = nil
			p.retreat(p.index - 1)
		}
		if len(parts) == 1 {
			this = exp.LiteralString(parts[0][1])
			unit = p.expression(exp.Var(exp.Args{"this": stringsUpper(parts[0][2])}), nil, nil)
		}
	}

	if p.dialect.IntervalSpans && p.matchTextSeq("TO") {
		expression := p.parseFunction(nil, false, true, false)
		if expression == nil {
			expression = p.parseVar(true, nil, true)
		}
		unit = p.expression(exp.IntervalSpan(exp.Args{"this": unit, "expression": expression}), nil, nil)
	}

	// Interval is a TimeUnit, whose constructor normalizes a Var/Column/Literal unit
	// (expressions/core.py:2055-2068).
	return p.expression(exp.Interval(exp.Args{"this": this, "unit": exp.TimeUnitVar(unit)}), nil, nil)
}

func (p *Parser) parseInterval(requireInterval bool) exp.Expression {
	return p.parseIntervalWith(requireInterval, true)
}

// parseIntervalWith is _parse_interval with its parse_function_unit flag (parser.py:6244-6283).
func (p *Parser) parseIntervalWith(requireInterval, parseFunctionUnit bool) exp.Expression {
	index := p.index
	if !p.match(tokens.INTERVAL) && requireInterval {
		return nil
	}

	var this exp.Expression
	if p.match(tokens.STRING, false) {
		this = p.parsePrimary()
	} else {
		this = p.parseTerm()
	}

	if this == nil || isBareIntervalColumnWithInvalidUnit(p, this) {
		p.retreat(index)
		return nil
	}

	interval := p.parseIntervalSpanWith(this, parseFunctionUnit)

	index = p.index
	p.match(tokens.PLUS)
	if p.matchSet(map[tokens.TokenType]bool{tokens.STRING: true, tokens.NUMBER: true}, false) {
		return p.expression(exp.Add(exp.Args{"this": interval, "expression": p.parseIntervalWith(false, parseFunctionUnit)}), nil, nil)
	}

	p.retreat(index)
	return interval
}

func isBareIntervalColumnWithInvalidUnit(p *Parser, this exp.Expression) bool {
	if this.Kind() != exp.KindColumn || this.Arg("table") != nil || !p.curr.IsValid() {
		return false
	}
	identifier := this.This()
	quoted := false
	if identifier != nil {
		quoted, _ = identifier.Arg("quoted").(bool)
	}
	return !quoted && !p.dialect.ValidIntervalUnits[stringsUpper(p.curr.Text)]
}
