package parser

import (
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// ClickHouse DDL overrides (parsers/clickhouse.py:714-1008) plus the base PROPERTY_PARSERS
// entries they reach (parser.py PROPERTY_PARSERS, _parse_ttl, _parse_dict_property, ...).

// addDictAndMergeTreePropertyParsers is called from the propertyParsers init (parser_ddl.go)
// once the shared table exists.
func addDictAndMergeTreePropertyParsers() {
	for name, parse := range map[string]propertyParserFunc{
		"EMPTY": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.New(exp.KindEmptyProperty, nil), nil, nil)
		},
		"LAYOUT":   func(p *Parser, _ bool) exp.Expression { return p.parseDictProperty("LAYOUT") },
		"LIFETIME": func(p *Parser, _ bool) exp.Expression { return p.parseDictRange("LIFETIME") },
		"ORDER BY": func(p *Parser, _ bool) exp.Expression { return p.parseOrder(nil, true) },
		"PRIMARY KEY": func(p *Parser, _ bool) exp.Expression {
			return p.parsePrimaryKey(false, true)
		},
		"RANGE": func(p *Parser, _ bool) exp.Expression { return p.parseDictRange("RANGE") },
		"SAMPLE": func(p *Parser, _ bool) exp.Expression {
			var this exp.Expression
			if p.matchTextSeq("BY") {
				this = p.parseBitwise()
			}
			return p.expression(exp.New(exp.KindSampleProperty, exp.Args{"this": this}), nil, nil)
		},
		"SECURE": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.New(exp.KindSecureProperty, nil), nil, nil)
		},
		"SETTINGS": func(p *Parser, _ bool) exp.Expression { return p.parseSettingsProperty() },
		"SOURCE":   func(p *Parser, _ bool) exp.Expression { return p.parseDictProperty("SOURCE") },
		"TO": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.New(exp.KindToTableProperty, exp.Args{"this": p.parseTableParts(true, false, false, false)}), nil, nil)
		},
		"TTL": func(p *Parser, _ bool) exp.Expression { return p.parseTTL() },
	} {
		propertyParsers[name] = parse
	}
}

// parseSettingsProperty ports _parse_settings_property (parser.py:3006-3009).
func (p *Parser) parseSettingsProperty() exp.Expression {
	return p.expression(exp.New(exp.KindSettingsProperty, exp.Args{"expressions": p.parseCsv(p.parseAssignment)}), nil, nil)
}

// parseTTL ports _parse_ttl (parser.py:2317-2351).
func (p *Parser) parseTTL() exp.Expression {
	parseTTLAction := func() exp.Expression {
		this := p.parseBitwise()
		switch {
		case p.matchTextSeq("DELETE"):
			return p.expression(exp.New(exp.KindMergeTreeTTLAction, exp.Args{"this": this, "delete": true}), nil, nil)
		case p.matchTextSeq("RECOMPRESS"):
			return p.expression(exp.New(exp.KindMergeTreeTTLAction, exp.Args{"this": this, "recompress": p.parseBitwise()}), nil, nil)
		case p.matchTextSeq("TO", "DISK"):
			return p.expression(exp.New(exp.KindMergeTreeTTLAction, exp.Args{"this": this, "to_disk": p.parseString()}), nil, nil)
		case p.matchTextSeq("TO", "VOLUME"):
			return p.expression(exp.New(exp.KindMergeTreeTTLAction, exp.Args{"this": this, "to_volume": p.parseString()}), nil, nil)
		}
		return this
	}
	expressions := p.parseCsv(parseTTLAction)
	where := p.parseWhere(false)
	group := p.parseGroup(false)
	var aggregates []exp.Expression
	if group != nil && p.match(tokens.SET) {
		aggregates = p.parseCsv(p.parseSetItem)
	}
	return p.expression(exp.New(exp.KindMergeTreeTTL, exp.Args{
		"expressions": expressions, "where": where, "group": group, "aggregates": nonEmpty(aggregates),
	}), nil, nil)
}

// parseDictProperty ports _parse_dict_property (parser.py:9607-9626).
func (p *Parser) parseDictProperty(this string) exp.Expression {
	var settings []exp.Expression
	p.matchLParen(nil)
	kind := p.parseIdVar(true, nil)
	if p.match(tokens.L_PAREN) {
		for {
			key := p.parseIdVar(true, nil)
			value := p.parseFunction(nil, false, true, false)
			if value == nil {
				value = p.parsePrimaryOrVar()
			}
			if key == nil && value == nil {
				break
			}
			settings = append(settings, p.expression(exp.New(exp.KindDictSubProperty, exp.Args{"this": key, "value": value}), nil, nil))
		}
		p.match(tokens.R_PAREN)
	}
	p.matchRParen(nil)
	var kindName any
	if kind != nil {
		kindName = kind.Arg("this")
	}
	return p.expression(exp.New(exp.KindDictProperty, exp.Args{"this": this, "kind": kindName, "settings": nonEmpty(settings)}), nil, nil)
}

// parseDictRange ports _parse_dict_range (parser.py:9628-9639).
func (p *Parser) parseDictRange(this string) exp.Expression {
	p.matchLParen(nil)
	var minValue, maxValue exp.Expression
	if p.matchTextSeq("MIN") {
		minValue = p.parseVar(false, nil, false)
		if minValue == nil {
			minValue = p.parsePrimary()
		}
		p.matchTextSeq("MAX")
		maxValue = p.parseVar(false, nil, false)
		if maxValue == nil {
			maxValue = p.parsePrimary()
		}
	} else {
		maxValue = p.parseVar(false, nil, false)
		if maxValue == nil {
			maxValue = p.parsePrimary()
		}
		minValue = exp.LiteralNumber(0)
	}
	p.matchRParen(nil)
	return p.expression(exp.New(exp.KindDictRange, exp.Args{"this": this, "min": minValue, "max": maxValue}), nil, nil)
}

// parseClickHouseOnProperty ports _parse_on_property (parsers/clickhouse.py:735-743).
func (p *Parser) parseClickHouseOnProperty() exp.Expression {
	index := p.index
	if p.matchTextSeq("CLUSTER") {
		this := p.parseString()
		if this == nil {
			this = p.parseIdVar(true, nil)
		}
		if this != nil {
			return p.expression(exp.New(exp.KindOnCluster, exp.Args{"this": this}), nil, nil)
		}
		p.retreat(index)
	}
	return nil
}

// parseClickHouseAutoRefreshProperty ports _parse_auto_refresh_property
// (parsers/clickhouse.py:745-796).
func (p *Parser) parseClickHouseAutoRefreshProperty() exp.Expression {
	index := p.index - 1
	var cadence any
	var interval exp.Expression
	if p.matchTexts(map[string]bool{"EVERY": true, "AFTER": true}) {
		cadence = stringsUpper(p.prev.Text)
		interval = p.parseIntervalWith(false, false)
	}
	if cadence != nil && interval == nil {
		p.retreat(index)
		return nil
	}
	var offset exp.Expression
	if p.matchTextSeq("OFFSET") {
		if offset = p.parseIntervalWith(false, false); offset == nil {
			p.retreat(index)
			return nil
		}
	}
	var randomize exp.Expression
	if p.matchTextSeq("RANDOMIZE", "FOR") {
		if randomize = p.parseIntervalWith(false, false); randomize == nil {
			p.retreat(index)
			return nil
		}
	}
	var dependencies []exp.Expression
	if p.matchTextSeq("DEPENDS", "ON") {
		dependencies = p.parseCsv(func() exp.Expression { return p.parseTableParts(true, false, false, false) })
		if len(dependencies) == 0 {
			p.retreat(index)
			return nil
		}
	}
	if cadence == nil && len(dependencies) == 0 {
		p.retreat(index)
		return nil
	}
	var settings exp.Expression
	if p.matchTextSeq("SETTINGS") {
		settings = p.parseSettingsProperty()
	}
	return p.expression(exp.New(exp.KindAutoRefreshProperty, exp.Args{
		"this":        interval,
		"cadence":     cadence,
		"offset":      offset,
		"randomize":   randomize,
		"expressions": nonEmpty(dependencies),
		"settings":    settings,
		"append":      p.matchTextSeq("APPEND"),
	}), nil, nil)
}

// parseClickHouseIndexConstraint ports _parse_index_constraint (parsers/clickhouse.py:798-812):
// INDEX name1 expr TYPE type1(args) GRANULARITY value.
func (p *Parser) parseClickHouseIndexConstraint() exp.Expression {
	this := p.parseIdVar(true, nil)
	expression := p.parseAssignment()
	var indexType exp.Expression
	if p.matchTextSeq("TYPE") {
		indexType = p.parseFunction(nil, false, true, false)
		if indexType == nil {
			indexType = p.parseVar(false, nil, false)
		}
	}
	var granularity exp.Expression
	if p.matchTextSeq("GRANULARITY") {
		granularity = p.parseTerm()
	}
	// Args are set in upstream's keyword order so ToS() matches the Python repr.
	node := exp.IndexColumnConstraint(nil)
	node.Set("this", this)
	node.Set("expression", expression)
	node.Set("index_type", indexType)
	node.Set("granularity", granularity)
	return p.expression(node, nil, nil)
}

// parseClickHousePartition ports _parse_partition (parsers/clickhouse.py:814-828).
func (p *Parser) parseClickHousePartition() exp.Expression {
	if !p.match(tokens.PARTITION) {
		return nil
	}
	var expressions []exp.Expression
	if p.matchTextSeq("ID") {
		// Corresponds to the PARTITION ID <string_value> syntax
		expressions = []exp.Expression{p.expression(exp.New(exp.KindPartitionId, exp.Args{"this": p.parseString()}), nil, nil)}
	} else {
		expressions = p.parseExpressions()
	}
	return p.expression(exp.Partition(exp.Args{"expressions": expressions}), nil, nil)
}

// parseClickHouseAlterTableReplace ports _parse_alter_table_replace (parsers/clickhouse.py:830-838).
func (p *Parser) parseClickHouseAlterTableReplace() exp.Expression {
	partition := p.parseClickHousePartition()
	if partition == nil || !p.match(tokens.FROM) {
		return nil
	}
	return p.expression(exp.New(exp.KindReplacePartition, exp.Args{"expression": partition, "source": p.parseTableParts(false, false, false, false)}), nil, nil)
}

// parseClickHouseAlterTableAlter ports _parse_alter_table_alter (parsers/clickhouse.py:840-853).
func (p *Parser) parseClickHouseAlterTableAlter() exp.Expression {
	// MODIFY forms other than MODIFY COLUMN (SQL SECURITY, ORDER BY, TTL, COMMENT, ...)
	// are parsed as properties, since ALTER can now reach this path too
	if stringsUpper(p.prev.Text) == "MODIFY" && !p.match(tokens.COLUMN, false) {
		if properties := p.parseProperties(); properties != nil {
			return p.expression(exp.New(exp.KindAlterModifySqlSecurity, exp.Args{"expressions": properties.Expressions()}), nil, nil)
		}
		return nil
	}
	// https://clickhouse.com/docs/sql-reference/statements/alter/column#modify-column
	alter := p.parseAlterTableAlter()
	if p.curr.IsValid() {
		return nil
	}
	return alter
}

// parseClickHouseDefiner ports _parse_definer (parsers/clickhouse.py:855-859).
func (p *Parser) parseClickHouseDefiner() exp.Expression {
	p.match(tokens.EQ)
	if p.match(tokens.CURRENT_USER) {
		return exp.DefinerProperty(exp.Args{"this": exp.Var(exp.Args{"this": stringsUpper(p.prev.Text)})})
	}
	return exp.DefinerProperty(exp.Args{"this": p.parseString()})
}

// parseClickHouseProjectionDef ports _parse_projection_def (parsers/clickhouse.py:861-869).
func (p *Parser) parseClickHouseProjectionDef() exp.Expression {
	if !p.match(tokens.PROJECTION) {
		return nil
	}
	return p.expression(exp.New(exp.KindProjectionDef, exp.Args{
		"this":       p.parseIdVar(true, nil),
		"expression": p.parseWrapped(p.parseStatement, false),
	}), nil, nil)
}

// parseClickHouseCheckOrAssume ports _parse_check_constraint/_parse_assume_constraint
// (parsers/clickhouse.py:396-404): the body is an optionally parenthesized SELECT or condition.
func (p *Parser) parseClickHouseCheckOrAssume(kind exp.Kind) exp.Expression {
	this := p.parseWrapped(func() exp.Expression {
		if sel := p.parseSelect(false, false, true, true); sel != nil {
			return sel
		}
		return p.parseAssignment()
	}, true)
	return p.expression(exp.New(kind, exp.Args{"this": this}), nil, nil)
}

// parseClickHouseDetach ports _parse_detach (parsers/clickhouse.py:990-1008).
func (p *Parser) parseClickHouseDetach() exp.Expression {
	var kind any
	if p.matchSet(dbCreatables) {
		kind = stringsUpper(p.prev.Text)
	}
	exists := p.parseExists(false)
	this := p.parseTableParts(false, false, false, false)
	var cluster exp.Expression
	if p.match(tokens.ON) {
		cluster = p.parseClickHouseOnProperty()
	}
	return p.expression(exp.New(exp.KindDetach, exp.Args{
		"this":      this,
		"kind":      kind,
		"exists":    exists,
		"cluster":   cluster,
		"permanent": p.matchTextSeq("PERMANENTLY"),
		"sync":      p.matchTextSeq("SYNC"),
	}), nil, nil)
}
