package parser

import (
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// parsers/snowflake.py: SnowflakeParser, a direct Parser subclass. Its flags and FUNCTIONS
// overlay live on dialects.Snowflake; the method overrides that hook into shared parser code
// are gated there with p.dialect.Is("snowflake").
func init() {
	registerDialectParserOverrides("snowflake", dialectParserOverrideSet{
		FunctionParsers: map[string]parserOverrideFunc{
			"DATE_PART":                  (*Parser).parseSnowflakeDatePart,
			"DIRECTORY":                  (*Parser).parseSnowflakeDirectory,
			"OBJECT_CONSTRUCT_KEEP_NULL": func(p *Parser) exp.Expression { return p.parseJSONObject(false) },
			"LISTAGG":                    (*Parser).parseStringAgg,
			"SEMANTIC_VIEW":              (*Parser).parseSnowflakeSemanticView,
			"SUBSTR":                     (*Parser).parseSubstring,
		},
		DisabledFunctionParsers: map[string]bool{"TRIM": true},
		StatementParsers: map[tokens.TokenType]parserOverrideFunc{
			tokens.GET:    (*Parser).parseSnowflakeGet,
			tokens.PUT:    (*Parser).parseSnowflakePut,
			tokens.SHOW:   (*Parser).parseSnowflakeShowStatement,
			tokens.UNDROP: (*Parser).parseSnowflakeUndrop,
		},
		NoParenFunctions: map[tokens.TokenType]func(exp.Args) exp.Expression{
			tokens.LOCALTIME:      exp.Localtime,
			tokens.LOCALTIMESTAMP: exp.Localtimestamp,
			tokens.CURRENT_TIME:   exp.Localtime,
		},
		PropertyParsers: map[string]propertyParserFunc{
			// Base PROPERTY_PARSERS entries (parser.py:1310-1413) the shared registry does not
			// carry yet.
			"COPY": func(p *Parser, _ bool) exp.Expression { return p.parseCopyProperty() },
			"DYNAMIC": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindDynamicProperty, nil), nil, nil)
			},
			"EXTERNAL": func(p *Parser, _ bool) exp.Expression { return p.expression(exp.ExternalProperty(nil), nil, nil) },
			"ICEBERG": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindIcebergProperty, nil), nil, nil)
			},
			"MULTISET": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindSetProperty, exp.Args{"multi": true}), nil, nil)
			},
			"SECURE": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindSecureProperty, nil), nil, nil)
			},
			"SET": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindSetProperty, exp.Args{"multi": false}), nil, nil)
			},
			"TRANSIENT": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindTransientProperty, nil), nil, nil)
			},
			"VOLATILE": func(p *Parser, _ bool) exp.Expression { return p.parseVolatileProperty() },
			"CREDENTIALS": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindCredentialsProperty, exp.Args{"expressions": p.parseWrappedOptions()}), nil, nil)
			},
			"FILE_FORMAT": func(p *Parser, _ bool) exp.Expression { return p.parseSnowflakeFileFormatProperty() },
			"LOCATION": func(p *Parser, _ bool) exp.Expression {
				p.match(tokens.EQ)
				return p.expression(exp.LocationProperty(exp.Args{"this": p.parseSnowflakeLocationPath()}), nil, nil)
			},
			"ROW": func(p *Parser, _ bool) exp.Expression {
				if p.matchTextSeq("ACCESS", "POLICY") {
					return p.parseSnowflakeRowAccessPolicy()
				}
				return p.parseRow()
			},
			"TAG": func(p *Parser, _ bool) exp.Expression { return p.parseSnowflakeTag() },
			"USING": func(p *Parser, _ bool) exp.Expression {
				if !p.matchTextSeq("TEMPLATE") {
					return nil
				}
				return p.expression(exp.New(exp.KindUsingTemplateProperty, exp.Args{"this": p.parseStatement()}), nil, nil)
			},
		},
		ColumnOperators: map[tokens.TokenType]columnOpFunc{
			tokens.EXCLAMATION: func(p *Parser, this, attr exp.Expression) exp.Expression {
				return p.expression(exp.New(exp.KindModelAttribute, exp.Args{"this": this, "expression": attr}), nil, nil)
			},
		},
		IDVarTokensSet:            snowflakeIDVarTokens,
		AliasTokensSet:            withTokens(aliasTokens, tokens.INTEGRATION, tokens.PACKAGE, tokens.POLICY, tokens.POOL, tokens.ROLE, tokens.RULE, tokens.VOLUME),
		TableAliasTokensSet:       withoutTokens(withTokens(tableAliasTokens, tokens.ANTI, tokens.INTEGRATION, tokens.PACKAGE, tokens.POLICY, tokens.POOL, tokens.ROLE, tokens.RULE, tokens.SEMI, tokens.VOLUME, tokens.WINDOW), tokens.MATCH_CONDITION),
		CreatablesSet:             withTokens(creatables, tokens.INTEGRATION, tokens.PACKAGE, tokens.POLICY, tokens.POOL, tokens.ROLE, tokens.RULE, tokens.VOLUME),
		TypeTokensAdded:           map[tokens.TokenType]bool{tokens.FILE: true},
		ColonPlaceholderTokensSet: withTokens(snowflakeIDVarTokens, tokens.NUMBER),
	})
}

var snowflakeIDVarTokens = withTokens(idVarTokens, tokens.EXCEPT, tokens.INTEGRATION, tokens.MATCH_CONDITION, tokens.PACKAGE, tokens.POLICY, tokens.POOL, tokens.ROLE, tokens.RULE, tokens.VOLUME)

// snowflakeFlattenColumns ports FLATTEN_COLUMNS (parsers/snowflake.py:845).
var snowflakeFlattenColumns = []string{"SEQ", "KEY", "PATH", "INDEX", "VALUE", "THIS"}

// snowflakeSchemaKinds ports SCHEMA_KINDS (parsers/snowflake.py:847-854).
var snowflakeSchemaKinds = map[string]bool{"OBJECTS": true, "TABLES": true, "VIEWS": true, "SEQUENCES": true, "UNIQUE KEYS": true, "IMPORTED KEYS": true}

// snowflakeNonTableCreatables ports NON_TABLE_CREATABLES (parsers/snowflake.py:856).
var snowflakeNonTableCreatables = map[string]bool{"STORAGE INTEGRATION": true, "TAG": true, "WAREHOUSE": true, "STREAMLIT": true}

// snowflakeUndropObjects ports UNDROP_OBJECTS (parsers/snowflake.py:858-878).
var snowflakeUndropObjects = optionsType{
	"ACCOUNT": nil, "DATABASE": nil, "NOTEBOOK": nil, "SCHEMA": nil, "SNAPSHOT": nil,
	"STREAMLIT": nil, "TABLE": nil, "TAG": nil, "TYPE": nil,
	"DYNAMIC":  {{"TABLE"}},
	"EXTERNAL": {{"VOLUME"}},
	"ICEBERG":  {{"TABLE"}},
}

// snowflakeShowParsers ports SHOW_PARSERS (parsers/snowflake.py:794-827).
var snowflakeShowParsers = func() map[string]func(*Parser) exp.Expression {
	show := func(this string, terse, iceberg bool) func(*Parser) exp.Expression {
		return func(p *Parser) exp.Expression { return p.parseShowSnowflake(this, terse, iceberg) }
	}
	return map[string]func(*Parser) exp.Expression{
		"DATABASES":            show("DATABASES", false, false),
		"SCHEMAS":              show("SCHEMAS", false, false),
		"OBJECTS":              show("OBJECTS", false, false),
		"TABLES":               show("TABLES", false, false),
		"VIEWS":                show("VIEWS", false, false),
		"PRIMARY KEYS":         show("PRIMARY KEYS", false, false),
		"IMPORTED KEYS":        show("IMPORTED KEYS", false, false),
		"UNIQUE KEYS":          show("UNIQUE KEYS", false, false),
		"SEQUENCES":            show("SEQUENCES", false, false),
		"STAGES":               show("STAGES", false, false),
		"COLUMNS":              show("COLUMNS", false, false),
		"USERS":                show("USERS", false, false),
		"FILE FORMATS":         show("FILE FORMATS", false, false),
		"FUNCTIONS":            show("FUNCTIONS", false, false),
		"PROCEDURES":           show("PROCEDURES", false, false),
		"WAREHOUSES":           show("WAREHOUSES", false, false),
		"ICEBERG TABLES":       show("TABLES", false, true),
		"TERSE ICEBERG TABLES": show("TABLES", true, true),
		"TERSE DATABASES":      show("DATABASES", true, false),
		"TERSE SCHEMAS":        show("SCHEMAS", true, false),
		"TERSE OBJECTS":        show("OBJECTS", true, false),
		"TERSE TABLES":         show("TABLES", true, false),
		"TERSE VIEWS":          show("VIEWS", true, false),
		"TERSE SEQUENCES":      show("SEQUENCES", true, false),
		"TERSE USERS":          show("USERS", true, false),
		// TERSE has no semantic effect for KEYS, so we do not set the terse AST arg
		"TERSE PRIMARY KEYS":  show("PRIMARY KEYS", false, false),
		"TERSE IMPORTED KEYS": show("IMPORTED KEYS", false, false),
		"TERSE UNIQUE KEYS":   show("UNIQUE KEYS", false, false),
	}
}()

var snowflakeShowTrie = func() wordTrie {
	keys := make([]string, 0, len(snowflakeShowParsers))
	for key := range snowflakeShowParsers {
		keys = append(keys, key)
	}
	return newTrie(keys)
}()

// snowflakeDescribeQualifierParsers ports DESCRIBE_QUALIFIER_PARSERS (parsers/snowflake.py:
// 761-788).
var snowflakeDescribeQualifierParsers = map[string]func(*Parser) exp.Expression{
	"API": func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindApiProperty, nil), nil, nil) },
	"APPLICATION": func(p *Parser) exp.Expression {
		return p.expression(exp.New(exp.KindApplicationProperty, nil), nil, nil)
	},
	"CATALOG": func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindCatalogProperty, nil), nil, nil) },
	"COMPUTE": func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindComputeProperty, nil), nil, nil) },
	"DATABASE": func(p *Parser) exp.Expression {
		if p.curr.IsValid() && stringsUpper(p.curr.Text) == "ROLE" {
			return p.expression(exp.New(exp.KindDatabaseProperty, nil), nil, nil)
		}
		return nil
	},
	"DYNAMIC":  func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindDynamicProperty, nil), nil, nil) },
	"EXTERNAL": func(p *Parser) exp.Expression { return p.expression(exp.ExternalProperty(nil), nil, nil) },
	"HYBRID":   func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindHybridProperty, nil), nil, nil) },
	"ICEBERG":  func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindIcebergProperty, nil), nil, nil) },
	"MASKING":  func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindMaskingProperty, nil), nil, nil) },
	"MATERIALIZED": func(p *Parser) exp.Expression {
		return p.expression(exp.MaterializedProperty(nil), nil, nil)
	},
	"NETWORK": func(p *Parser) exp.Expression { return p.expression(exp.New(exp.KindNetworkProperty, nil), nil, nil) },
	"ROW": func(p *Parser) exp.Expression {
		if p.matchTextSeq("ACCESS") {
			return p.expression(exp.New(exp.KindRowAccessProperty, nil), nil, nil)
		}
		return nil
	},
	"SECURITY": func(p *Parser) exp.Expression {
		if p.curr.IsValid() && stringsUpper(p.curr.Text) == "INTEGRATION" {
			return p.expression(exp.New(exp.KindSecurityIntegrationProperty, nil), nil, nil)
		}
		return nil
	},
}

var snowflakeDescribeQualifierKeys = func() map[string]bool {
	keys := map[string]bool{}
	for key := range snowflakeDescribeQualifierParsers {
		keys[key] = true
	}
	return keys
}()

// parseSnowflakeDirectory ports _parse_directory (parsers/snowflake.py:930-933).
func (p *Parser) parseSnowflakeDirectory() exp.Expression {
	table := p.parseTableParts(false, false, false, false)
	this := table
	if table != nil && table.Kind() == exp.KindTable {
		this = table.This()
	}
	return p.expression(exp.New(exp.KindDirectoryStage, exp.Args{"this": this}), nil, nil)
}

// parseSnowflakeDescribe ports _parse_describe (parsers/snowflake.py:935-958); nil means the
// base _parse_describe runs.
func (p *Parser) parseSnowflakeDescribe() exp.Expression {
	index := p.index
	if p.matchTexts(snowflakeDescribeQualifierKeys) {
		qualifier := snowflakeDescribeQualifierParsers[stringsUpper(p.prev.Text)](p)
		if qualifier != nil && p.matchSet(p.creatablesFor()) {
			kind := stringsUpper(p.prev.Text)
			this := p.parseTable(true, false, nil, false, false, false, false)
			properties := p.expression(exp.Properties(exp.Args{"expressions": []exp.Expression{qualifier}}), nil, nil)
			var expressions any
			if postProps := p.parseProperties(); postProps != nil {
				expressions = postProps.Expressions()
			}
			return p.expression(exp.Describe(exp.Args{"this": this, "kind": kind, "properties": properties, "expressions": expressions}), nil, nil)
		}
	}
	p.retreat(index)
	return nil
}

// parseSnowflakeUse ports _parse_use (parsers/snowflake.py:960-966); nil means the base
// _parse_use runs.
func (p *Parser) parseSnowflakeUse() exp.Expression {
	if !p.matchTextSeq("SECONDARY", "ROLES") {
		return nil
	}
	var this any = false
	if p.matchTexts(map[string]bool{"ALL": true, "NONE": true}) {
		this = exp.Var(exp.Args{"this": stringsUpper(p.prev.Text)})
	}
	var roles any
	if this == false {
		roles = p.parseCsv(func() exp.Expression { return p.parseTable(false, false, nil, false, false, false, false) })
	}
	return p.expression(exp.Use(exp.Args{"kind": "SECONDARY ROLES", "this": this, "expressions": roles}), nil, nil)
}

// snowflakeNegateRange ports _negate_range (parsers/snowflake.py:968-982).
func (p *Parser) snowflakeNegateRange(this exp.Expression) exp.Expression {
	if this == nil {
		return this
	}
	if query := asExpr(this.Arg("query")); this.Kind() == exp.KindIn && query != nil && query.Is(exp.TraitQuery) {
		// Snowflake treats `value NOT IN (subquery)` as `VALUE <> ALL (subquery)`, so
		// we do this conversion here to avoid parsing it into `NOT value IN (subquery)`
		// which can produce different results (most likely a SnowFlake bug).
		return p.expression(exp.NEQ(exp.Args{"this": this.This(), "expression": exp.New(exp.KindAll, exp.Args{"this": query.Unnest()})}), nil, nil)
	}
	return p.expression(exp.Not(exp.Args{"this": this}), nil, nil)
}

// parseSnowflakeTag ports _parse_tag (parsers/snowflake.py:984-985).
func (p *Parser) parseSnowflakeTag() exp.Expression {
	return p.expression(exp.New(exp.KindTags, exp.Args{"expressions": p.parseWrappedCsv(p.parseProperty)}), nil, nil)
}

// parseSnowflakePropertyBefore ports _parse_property_before's Snowflake tail (parsers/
// snowflake.py:987-996): `KEY = value` options of DDL that has no PROPERTY_PARSERS entry.
func (p *Parser) parseSnowflakePropertyBefore() exp.Expression {
	if !p.next.IsValid() || p.next.TokenType != tokens.EQ {
		return nil
	}
	if seq := p.parseSequenceProperties(); seq != nil {
		return seq
	}
	return p.parseKeyValueProperty(p.parsePrimaryOrVar)
}

// parseSnowflakeWithConstraint ports _parse_with_constraint (parsers/snowflake.py:998-1022).
func (p *Parser) parseSnowflakeWithConstraint() exp.Expression {
	if p.prev.TokenType != tokens.WITH {
		p.retreat(p.index - 1)
	}
	if p.matchTextSeq("MASKING", "POLICY") {
		policy := p.parseColumn()
		if policy != nil && policy.Kind() == exp.KindColumn {
			policy = exp.ColumnsToDot(policy)
		}
		var expressions any = false
		if p.match(tokens.USING) {
			expressions = p.parseWrappedCsv(func() exp.Expression { return p.parseIdVar(true, nil) })
		}
		return p.expression(exp.New(exp.KindMaskingPolicyColumnConstraint, exp.Args{"this": policy, "expressions": expressions}), nil, nil)
	}
	if p.matchTextSeq("PROJECTION", "POLICY") {
		policy := p.parseColumn()
		if policy != nil && policy.Kind() == exp.KindColumn {
			policy = exp.ColumnsToDot(policy)
		}
		return p.expression(exp.New(exp.KindProjectionPolicyColumnConstraint, exp.Args{"this": policy}), nil, nil)
	}
	if p.match(tokens.TAG) {
		return p.parseSnowflakeTag()
	}
	return nil
}

// parseSnowflakeWithProperty ports _parse_with_property (parsers/snowflake.py:1024-1031); nil
// means the base _parse_with_property runs.
func (p *Parser) parseSnowflakeWithProperty() exp.Expression {
	if p.match(tokens.TAG) {
		return p.parseSnowflakeTag()
	}
	if p.matchTextSeq("ROW", "ACCESS", "POLICY") {
		return p.parseSnowflakeRowAccessPolicy()
	}
	return nil
}

// parseSnowflakeRowAccessPolicy ports _parse_row_access_policy (parsers/snowflake.py:
// 1033-1049).
func (p *Parser) parseSnowflakeRowAccessPolicy() exp.Expression {
	// GET_DDL outputs #unknown_policy when the user lacks privileges to see the policy name
	var policy exp.Expression
	var expressions any
	if p.match(tokens.HASH) {
		if v := p.parseVar(true, nil, false); v != nil {
			policy = exp.Var(exp.Args{"this": "#" + v.Name()})
		}
	} else {
		policy = p.parseColumn()
		if policy != nil && policy.Kind() == exp.KindColumn {
			policy = exp.ColumnsToDot(policy)
		}
		if !p.match(tokens.ON) {
			p.raiseError("Expected ON after ROW ACCESS POLICY name")
		}
		expressions = p.parseWrappedCsv(func() exp.Expression { return p.parseIdVar(true, nil) })
	}
	return p.expression(exp.New(exp.KindRowAccessProperty, exp.Args{"this": policy, "expressions": expressions}), nil, nil)
}

// snowflakeFixCreate ports _parse_create (parsers/snowflake.py:1051-1057): for a non-table
// creatable the Table node is replaced by its enclosed Identifier.
func snowflakeFixCreate(expression exp.Expression) exp.Expression {
	if expression != nil && expression.Kind() == exp.KindCreate && snowflakeNonTableCreatables[expression.Text("kind")] {
		if this := expression.This(); this != nil && this.This() != nil {
			this.Replace(this.This())
		}
	}
	return expression
}

// parseSnowflakeDatePart ports _parse_date_part (parsers/snowflake.py:1059-1072).
func (p *Parser) parseSnowflakeDatePart() exp.Expression {
	this := p.parseVar(false, nil, false)
	if this == nil {
		this = p.parseType(true, false)
	}
	if this == nil {
		return nil
	}
	// Handle both syntaxes: DATE_PART(part, expr) and DATE_PART(part FROM expr)
	var expression exp.Expression
	if p.matchSet(map[tokens.TokenType]bool{tokens.FROM: true, tokens.COMMA: true}) {
		expression = p.parseBitwise()
	}
	return p.expression(exp.Extract(exp.Args{"this": dialects.MapDatePart(this, p.dialect), "expression": expression}), nil, nil)
}

// parseSnowflakeBracketKeyValue ports _parse_bracket_key_value (parsers/snowflake.py:
// 1074-1081).
func (p *Parser) parseSnowflakeBracketKeyValue(isMap bool) exp.Expression {
	if isMap {
		// Keys are strings in Snowflake's objects
		if s := p.parseSlice(p.parseString()); s != nil {
			return s
		}
		return p.parseAssignment()
	}
	return p.parseSlice(p.parseAlias(p.parseAssignment(), true))
}

// snowflakeFixLateral ports _parse_lateral's Snowflake tail (parsers/snowflake.py:1083-1096):
// a FLATTEN source gets the FLATTEN output columns as its alias columns.
func snowflakeFixLateral(lateral exp.Expression) exp.Expression {
	if lateral == nil || lateral.This() == nil || lateral.This().Kind() != exp.KindExplode {
		return lateral
	}
	tableAlias := asExpr(lateral.Arg("alias"))
	columns := make([]exp.Expression, len(snowflakeFlattenColumns))
	for i, col := range snowflakeFlattenColumns {
		columns[i] = exp.ToIdentifier(col)
	}
	if tableAlias != nil && len(listArg(tableAlias, "columns")) == 0 {
		tableAlias.Set("columns", columns)
	} else if tableAlias == nil {
		lateral.Set("alias", exp.TableAlias(exp.Args{"this": exp.ToIdentifier("_flattened"), "columns": columns}))
	}
	return lateral
}

// parseSnowflakeStagedFile ports the staged-file head of _parse_table_parts (parsers/
// snowflake.py:1098-1133); nil means the base _parse_table_parts runs.
func (p *Parser) parseSnowflakeStagedFile(isDBReference bool) exp.Expression {
	var table exp.Expression
	if p.match(tokens.STRING, false) {
		table = p.parseString()
	} else if p.curr.IsValid() && p.curr.Text == "@" {
		table = p.parseSnowflakeLocationPath()
	} else {
		return nil
	}
	var fileFormat, pattern exp.Expression
	wrapped := p.match(tokens.L_PAREN)
	for p.curr.IsValid() && wrapped && !p.match(tokens.R_PAREN) {
		if p.matchTextSeq("FILE_FORMAT", "=>") {
			fileFormat = p.parseString()
			if fileFormat == nil {
				fileFormat = p.parseTablePartsBase(false, isDBReference, false, false)
			}
		} else if p.matchTextSeq("PATTERN", "=>") {
			pattern = p.parseString()
		} else {
			break
		}
		p.match(tokens.COMMA)
	}
	return p.expression(exp.Table(exp.Args{"this": table, "format": fileFormat, "pattern": pattern}), nil, nil)
}

// snowflakeHoistTableFromRows ports _parse_table's Snowflake tail (parsers/snowflake.py:
// 1135-1160): a TABLE(...) source becomes the TableFromRows node itself.
func snowflakeHoistTableFromRows(table exp.Expression) exp.Expression {
	if table == nil || table.Kind() != exp.KindTable || table.This() == nil || table.This().Kind() != exp.KindTableFromRows {
		return table
	}
	tableFromRows := table.This()
	for _, arg := range exp.ArgKeys(exp.KindTableFromRows) {
		if arg != "this" {
			tableFromRows.Set(arg, table.Arg(arg))
		}
	}
	return tableFromRows
}

// snowflakeDynamicIdentifierCall ports _parse_function_call's Snowflake tail (parsers/
// snowflake.py:1162-1182): `IDENTIFIER('my_func')(1, 2)`.
func (p *Parser) snowflakeDynamicIdentifierCall(this exp.Expression) exp.Expression {
	if this != nil && this.Kind() == exp.KindDynamicIdentifier && p.match(tokens.L_PAREN, false) {
		this.Set("expressions", p.parseWrappedCsv(func() exp.Expression { return p.parseLambda(false) }))
	}
	return this
}

// parseSnowflakeIDVar ports _parse_id_var (parsers/snowflake.py:1184-1197); nil means the base
// _parse_id_var runs.
func (p *Parser) parseSnowflakeIDVar(anyToken bool, toks map[tokens.TokenType]bool) (exp.Expression, bool) {
	if !p.matchTextSeq("IDENTIFIER", "(") {
		return nil, false
	}
	identifier := p.parseIdVarBase(anyToken, toks)
	if identifier == nil {
		identifier = p.parseString()
	}
	p.matchRParen(nil)
	return p.expression(exp.New(exp.KindDynamicIdentifier, exp.Args{"this": identifier}), nil, nil), true
}

// parseSnowflakeShowStatement ports SHOW through _parse_show (parser.py:9226-9230) with
// Snowflake's SHOW_PARSERS.
func (p *Parser) parseSnowflakeShowStatement() exp.Expression {
	if parse := p.findParser(snowflakeShowParsers, snowflakeShowTrie); parse != nil {
		return parse(p)
	}
	return p.parseAsCommand(p.prev)
}

// parseShowSnowflake ports _parse_show_snowflake (parsers/snowflake.py:1199-1248).
func (p *Parser) parseShowSnowflake(this string, terse, iceberg bool) exp.Expression {
	var scope exp.Expression
	var scopeKind any

	history := p.matchTextSeq("HISTORY")

	var like exp.Expression
	if p.match(tokens.LIKE) {
		like = p.parseString()
	}

	if p.match(tokens.IN) {
		switch {
		case p.matchTextSeq("ACCOUNT"):
			scopeKind = "ACCOUNT"
		case p.matchTextSeq("CLASS"):
			scopeKind = "CLASS"
			scope = p.parseTableParts(false, false, false, false)
		case p.matchTextSeq("APPLICATION"):
			kind := "APPLICATION"
			if p.matchTextSeq("PACKAGE") {
				kind += " PACKAGE"
			}
			scopeKind = kind
			scope = p.parseTableParts(false, false, false, false)
		case p.matchSet(dbCreatables):
			scopeKind = stringsUpper(p.prev.Text)
			if p.curr.IsValid() {
				scope = p.parseTableParts(false, false, false, false)
			}
		case p.curr.IsValid():
			if snowflakeSchemaKinds[this] {
				scopeKind = "SCHEMA"
			} else {
				scopeKind = "TABLE"
			}
			scope = p.parseTableParts(false, false, false, false)
		}
	}

	var startsWith any = false
	if p.matchTextSeq("STARTS", "WITH") {
		startsWith = p.parseString()
	}
	limit := p.parseLimit(nil, false, false)
	var from exp.Expression
	if p.match(tokens.FROM) {
		from = p.parseString()
	}
	var privileges any = false
	if p.matchTextSeq("WITH", "PRIVILEGES") {
		privileges = p.parseCsv(func() exp.Expression { return p.parseVar(true, nil, true) })
	}
	return p.expression(exp.Show(exp.Args{
		"terse":       terse,
		"iceberg":     iceberg,
		"this":        this,
		"history":     history,
		"like":        like,
		"scope":       scope,
		"scope_kind":  scopeKind,
		"starts_with": startsWith,
		"limit":       limit,
		"from_":       from,
		"privileges":  privileges,
	}), nil, nil)
}

// parseSnowflakeUndrop ports _parse_undrop (parsers/snowflake.py:1250-1261).
func (p *Parser) parseSnowflakeUndrop() exp.Expression {
	start := p.prev
	kind := p.parseVarFromOptions(snowflakeUndropObjects, false)
	if kind == nil {
		return p.parseAsCommand(start)
	}
	name := kind.Name()
	this := p.parseTableParts(false, name == "ACCOUNT" || name == "DATABASE" || name == "SCHEMA", false, false)
	var rename exp.Expression
	if p.matchTextSeq("RENAME", "TO") {
		rename = p.parseTableParts(false, false, false, false)
	}
	return p.expression(exp.New(exp.KindUndrop, exp.Args{"this": this, "kind": name, "rename": rename}), nil, nil)
}

// parseSnowflakePut ports _parse_put (parsers/snowflake.py:1263-1273).
func (p *Parser) parseSnowflakePut() exp.Expression {
	if p.curr.TokenType != tokens.STRING {
		return p.parseAsCommand(p.prev)
	}
	this := p.parseString()
	target := p.parseSnowflakeLocationPath()
	return p.expression(exp.New(exp.KindPut, exp.Args{"this": this, "target": target, "properties": p.parseProperties()}), nil, nil)
}

// parseSnowflakeGet ports _parse_get (parsers/snowflake.py:1275-1291).
func (p *Parser) parseSnowflakeGet() exp.Expression {
	start := p.prev
	// If we detect GET( then we need to parse a function, not a statement
	if p.match(tokens.L_PAREN) {
		p.retreat(p.index - 2)
		return p.parseExpression()
	}
	target := p.parseSnowflakeLocationPath()
	// Parse as command if unquoted file path
	if p.curr.TokenType == tokens.URI_START {
		return p.parseAsCommand(start)
	}
	this := p.parseString()
	return p.expression(exp.New(exp.KindGet, exp.Args{"this": this, "target": target, "properties": p.parseProperties()}), nil, nil)
}

// parseSnowflakeFileLocation ports _parse_file_location (parsers/snowflake.py:1297-1303).
func (p *Parser) parseSnowflakeFileLocation() exp.Expression {
	// Parse either a subquery or a staged file
	if p.match(tokens.L_PAREN, false) {
		return p.parseSelect(false, true, false)
	}
	return p.parseTableParts(false, false, false, false)
}

// parseSnowflakeLocationPath ports _parse_location_path (parsers/snowflake.py:1305-1317).
func (p *Parser) parseSnowflakeLocationPath() exp.Expression {
	start := p.curr
	p.advanceAny(true)
	// We avoid consuming a comma token because external tables like @foo and @bar
	// can be joined in a query with a comma separator, as well as closing paren
	// in case of subqueries
	stop := map[tokens.TokenType]bool{tokens.COMMA: true, tokens.L_PAREN: true, tokens.R_PAREN: true}
	for p.isConnected() && !p.matchSet(stop, false) {
		p.advanceAny(true)
	}
	return exp.Var(exp.Args{"this": p.findSQL(start, p.prev)})
}

// parseSnowflakeLambdaArg ports _parse_lambda_arg (parsers/snowflake.py:1319-1330).
func (p *Parser) parseSnowflakeLambdaArg() exp.Expression {
	this := p.parseIdVar(true, nil)
	if this == nil {
		return this
	}
	if typ := p.parseTypes(false, false, false, false); typ != nil {
		return p.expression(exp.Cast(exp.Args{"this": this, "to": typ}), nil, nil)
	}
	return this
}

// parseSnowflakeFileFormatProperty ports _parse_file_format_property (parsers/snowflake.py:
// 1340-1348).
func (p *Parser) parseSnowflakeFileFormatProperty() exp.Expression {
	p.match(tokens.EQ)
	var expressions []exp.Expression
	if p.match(tokens.L_PAREN, false) {
		expressions = p.parseWrappedOptions()
	} else {
		expressions = []exp.Expression{p.parseFormatName()}
	}
	return p.expression(exp.FileFormatProperty(exp.Args{"expressions": expressions}), nil, nil)
}

// parseSnowflakeSemanticView ports _parse_semantic_view (parsers/snowflake.py:1353-1369).
func (p *Parser) parseSnowflakeSemanticView() exp.Expression {
	args := exp.Args{"this": p.parseTableParts(false, false, false, false)}
	for p.curr.IsValid() && !p.match(tokens.R_PAREN, false) {
		if p.matchTexts(map[string]bool{"DIMENSIONS": true, "METRICS": true, "FACTS": true}) {
			keyword := strings.ToLower(p.prev.Text)
			args[keyword] = p.parseCsv(func() exp.Expression { return p.parseAlias(p.parseDisjunction(), true) })
		} else if p.matchTextSeq("WHERE") {
			args["where"] = p.parseExpression()
		} else {
			p.raiseError("Expecting ) or encountered unexpected keyword")
			break
		}
	}
	return p.expression(exp.New(exp.KindSemanticView, args), nil, nil)
}

// snowflakeSetVariableKind ports _parse_set (parsers/snowflake.py:1371-1378).
func snowflakeSetVariableKind(set exp.Expression) exp.Expression {
	if set != nil && set.Kind() == exp.KindSet {
		for _, expr := range set.Expressions() {
			if expr.Kind() == exp.KindSetItem {
				expr.Set("kind", "VARIABLE")
			}
		}
	}
	return set
}

// snowflakeBuildCast ports build_cast (parsers/snowflake.py:1388-1398); nil means the base
// build_cast result is used unchanged.
func (p *Parser) snowflakeBuildCast(strict bool, args exp.Args) exp.Expression {
	to, _ := args["to"].(exp.Expression)
	if !strict && exp.IsType(to, exp.DTypeBoolean) {
		return p.expression(exp.New(exp.KindToBoolean, exp.Args{"this": args["this"], "safe": true}), nil, nil)
	}
	return nil
}

// snowflakeFixTryCast ports the TryCast tail of build_cast (parsers/snowflake.py:1393-1398).
func snowflakeFixTryCast(cast exp.Expression, to exp.Expression) exp.Expression {
	if cast == nil || cast.Kind() != exp.KindTryCast || to == nil || to.Kind() != exp.KindDataType {
		return cast
	}
	dtype, _ := to.Arg("this").(exp.DType)
	if exp.TextTypes[dtype] && len(to.Expressions()) > 0 {
		cast.Set("null_on_text_overflow", true)
	} else if dtype == exp.DTypeDate && cast.This() != nil && cast.This().IsString() {
		cast.Set("probe_date_format", true)
	}
	return cast
}

// snowflakeNthValueFrom ports the NTH_VALUE head of _parse_window (parsers/snowflake.py:
// 1400-1405).
func (p *Parser) snowflakeNthValueFrom(this exp.Expression) {
	if this == nil || this.Kind() != exp.KindNthValue {
		return
	}
	if p.matchTextSeq("FROM", "FIRST") {
		this.Set("from_first", true)
	} else if p.matchTextSeq("FROM", "LAST") {
		this.Set("from_first", false)
	}
}

// snowflakeDefaultRankingFrame ports the tail of _parse_window (parsers/snowflake.py:
// 1407-1421): ranking window functions get the default full frame.
func (p *Parser) snowflakeDefaultRankingFrame(this, result exp.Expression) exp.Expression {
	if result == nil || result.Kind() != exp.KindWindow || this == nil || result.Arg("spec") != nil {
		return result
	}
	switch this.Kind() {
	case exp.KindFirstValue, exp.KindLastValue, exp.KindNthValue:
		result.Set("spec", exp.New(exp.KindWindowSpec, exp.Args{
			"kind":       "ROWS",
			"start":      "UNBOUNDED",
			"start_side": "PRECEDING",
			"end":        "UNBOUNDED",
			"end_side":   "FOLLOWING",
		}))
	}
	return result
}

// parseCopyProperty ports _parse_copy_property (parser.py:2994-2999).
func (p *Parser) parseCopyProperty() exp.Expression {
	if !p.matchTextSeq("GRANTS") {
		p.retreat(p.index - 1)
		return nil
	}
	return p.expression(exp.New(exp.KindCopyGrantsProperty, nil), nil, nil)
}

// preVolatileTokens ports PRE_VOLATILE_TOKENS (parser.py:1641).
var preVolatileTokens = map[tokens.TokenType]bool{tokens.CREATE: true, tokens.REPLACE: true, tokens.UNIQUE: true}

// parseVolatileProperty ports _parse_volatile_property (parser.py:3018-3027).
func (p *Parser) parseVolatileProperty() exp.Expression {
	if p.index >= 2 && preVolatileTokens[p.tokens[p.index-2].TokenType] {
		return exp.New(exp.KindVolatileProperty, nil)
	}
	return p.expression(exp.StabilityProperty(exp.Args{"this": exp.LiteralString("VOLATILE")}), nil, nil)
}
