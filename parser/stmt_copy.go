package parser

import (
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.COPY] = (*Parser).parseCopy
}

// parseCopy ports _parse_copy (parser.py:9909-9941): `COPY [INTO] <table-or-select>
// FROM|TO <files> [<credentials>] [WITH] [(<params>)]`; trailing tokens degrade to Command.
func (p *Parser) parseCopy() exp.Expression {
	start := p.prev

	p.match(tokens.INTO)

	var this exp.Expression
	if p.match(tokens.L_PAREN, false) {
		this = p.parseSelect(true, false, false)
	} else {
		this = p.parseTable(true, false, nil, false, false, false, false)
	}

	// kind = self._match(TokenType.FROM) or not self._match_text_seq("TO")
	kind := p.match(tokens.FROM)
	if !kind {
		kind = !p.matchTextSeq("TO")
	}

	files := p.parseCsv(p.parseFileLocation)
	if p.match(tokens.EQ, false) {
		// Backtrack one token since we've consumed the lhs of a parameter assignment here.
		// This can happen for Snowflake dialect. Instead, we'd like to parse the parameter
		// list via `_parse_wrapped(..)` below.
		p.advance(-1)
		files = nil
	}

	credentials := p.parseCredentials()

	p.matchTextSeq("WITH")

	// _parse_wrapped(self._parse_copy_parameters, optional=True): Go has no list-returning
	// parseWrapped, so the wrapper is inlined here.
	wrapped := p.match(tokens.L_PAREN)
	params := p.parseCopyParameters()
	if wrapped {
		p.matchRParen(nil)
	}

	// Fallback case
	if p.curr.IsValid() {
		return p.parseAsCommand(start)
	}

	return p.expression(exp.Copy(exp.Args{
		"this":        this,
		"kind":        kind,
		"credentials": credentials,
		"files":       files,
		"params":      params,
	}), nil, nil)
}

// parseFileLocation ports _parse_file_location (parser.py:9906-9907) and its Snowflake
// override (parsers/snowflake.py:1297-1303).
func (p *Parser) parseFileLocation() exp.Expression {
	if p.dialect.Is("snowflake") {
		return p.parseSnowflakeFileLocation()
	}
	return p.parseField(false, nil, false)
}

// copyIntoVarlenOptions ports COPY_INTO_VARLEN_OPTIONS (parser.py:1781-1786).
var copyIntoVarlenOptions = map[string]bool{"FILE_FORMAT": true, "COPY_OPTIONS": true, "FORMAT_OPTIONS": true, "CREDENTIAL": true}

// parseWrappedOptions ports _parse_wrapped_options (parser.py:9823-9842).
func (p *Parser) parseWrappedOptions() []exp.Expression {
	p.match(tokens.EQ)
	p.match(tokens.L_PAREN)

	var opts []exp.Expression
	for p.curr.IsValid() && !p.match(tokens.R_PAREN) {
		var option exp.Expression
		if p.matchTextSeq("FORMAT_NAME", "=") {
			// The FORMAT_NAME can be set to an identifier for Snowflake and T-SQL
			option = p.parseFormatName()
		} else {
			option = p.parseProperty()
		}
		if option == nil {
			p.raiseError("Unable to parse option")
			break
		}
		opts = append(opts, option)
	}
	return opts
}

// parseFormatName ports _parse_format_name (parser.py:10100-10107).
func (p *Parser) parseFormatName() exp.Expression {
	value := p.parseString()
	if value == nil {
		value = p.parseTableParts(false, false, false, false)
	}
	return p.expression(exp.Property(exp.Args{"this": exp.Var(exp.Args{"this": "FORMAT_NAME"}), "value": value}), nil, nil)
}

// parseCopyParameters ports _parse_copy_parameters (parser.py:9844-9881).
func (p *Parser) parseCopyParameters() []exp.Expression {
	sep := p.dialect.CopyParamsAreCsv

	var options []exp.Expression
	for p.curr.IsValid() && !p.match(tokens.R_PAREN, false) {
		startIndex := p.index
		option := p.parseVar(true, nil, false)
		prev := stringsUpper(p.prev.Text)

		// Different dialects might separate options and values by white space, "=" and "AS".
		p.match(tokens.EQ)
		matchedAlias := p.match(tokens.ALIAS)

		param := p.expression(exp.CopyParameter(exp.Args{"this": option}), nil, nil)

		if copyIntoVarlenOptions[prev] && p.match(tokens.L_PAREN, false) {
			// Snowflake FILE_FORMAT case, Databricks COPY & FORMAT options
			param.Set("expressions", p.parseWrappedOptions())
		} else if prev == "FILE_FORMAT" {
			// T-SQL's external file format case
			param.Set("expression", p.parseField(false, nil, false))
		} else if prev == "FORMAT" && matchedAlias && p.matchTexts(map[string]bool{"AVRO": true, "JSON": true}) {
			// parser.py:9573-9579: `FORMAT AS AVRO|JSON`. Fold the keyword into `this` as
			// the var "FORMAT AS <fmt>" and take the following field (if any) as the value,
			// so it round-trips as `FORMAT AS JSON` / `FORMAT AS JSON 'x'` rather than the
			// lossy `FORMAT JSON` / `FORMAT JSON, x` the generic branch would produce.
			param.Set("this", exp.Var(exp.Args{"this": "FORMAT AS " + stringsUpper(p.prev.Text)}))
			param.Set("expression", p.parseField(false, nil, false))
		} else {
			expression := p.parseUnquotedField()
			if expression == nil {
				expression = p.parseBracket(nil)
			}
			param.Set("expression", expression)
		}

		// No-progress guard: upstream's _parse_var(any_token=True) consumes any token, but
		// this port's parseVar returns nil for reserved tokens (operators, stray separators).
		// If an iteration consumed nothing, stop instead of spinning — the leftover tokens
		// reach parseCopy's trailing-token Command fallback (degrade-to-Command). This now
		// also runs on unwrapped parameter lists, so guarding termination here matters.
		if p.index == startIndex {
			break
		}

		options = append(options, param)

		if sep {
			p.match(tokens.COMMA)
		}
	}

	return options
}

// parseCredentials ports _parse_credentials (parser.py:9883-9904).
func (p *Parser) parseCredentials() exp.Expression {
	expr := p.expression(exp.Credentials(exp.Args{}), nil, nil)

	if p.matchTextSeq("STORAGE_INTEGRATION", "=") {
		expr.Set("storage", p.parseField(false, nil, false))
	}
	if p.matchTextSeq("CREDENTIALS") {
		// Snowflake case: CREDENTIALS = (...), Redshift case: CREDENTIALS <string>
		if p.match(tokens.EQ) {
			expr.Set("credentials", p.parseWrappedOptions())
		} else {
			expr.Set("credentials", p.parseField(false, nil, false))
		}
	}
	if p.matchTextSeq("ENCRYPTION") {
		expr.Set("encryption", p.parseWrappedOptions())
	}
	if p.matchTextSeq("IAM_ROLE") {
		if p.match(tokens.DEFAULT) {
			expr.Set("iam_role", exp.Var(exp.Args{"this": p.prev.Text}))
		} else {
			expr.Set("iam_role", p.parseField(false, nil, false))
		}
	}
	if p.matchTextSeq("REGION") {
		expr.Set("region", p.parseField(false, nil, false))
	}

	return expr
}
