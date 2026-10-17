package parser

import (
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// Upstream keeps parser callbacks in per-dialect class tables: the base statement,
// no-parentheses-function, and function tables are in .reference/sqlglot-v30.12.0/sqlglot/parser.py:
// 1081-1111, 1470-1477, and 1488-1521; Presto removes TRIM at .reference/sqlglot-v30.12.0/sqlglot/
// parsers/presto.py:137; and Athena adds a statement parser at .reference/sqlglot-v30.12.0/sqlglot/
// parsers/athena.py:17-21. This registry applies the same dependency-inversion principle as
// expressions.ParseIntoFunc (expressions/builders.go:14-15, wired at sqlglot.go:108-114), but
// cannot copy that mechanism literally. ParseIntoFunc uses leaf-package types, while these
// callbacks must resume an in-flight *Parser. Their typed callback table therefore remains in the
// parser package and is selected through a plain-string parser override name (normally the dialect
// Name), avoiding a dialects -> parser import cycle.
type parserOverrideFunc = func(*Parser) exp.Expression
type typeParserOverrideFunc = func(*Parser, bool, bool, bool, bool) exp.Expression

type dialectParserOverrideSet struct {
	FunctionParsers         map[string]parserOverrideFunc
	DisabledFunctionParsers map[string]bool
	StatementParsers        map[tokens.TokenType]parserOverrideFunc
	NoParenFunctionParsers  map[string]parserOverrideFunc
	NoParenFunctions        map[tokens.TokenType]func(exp.Args) exp.Expression
	// DisabledNoParenFunctions removes base NO_PAREN_FUNCTIONS entries (parsers/hive.py:126-128).
	DisabledNoParenFunctions map[tokens.TokenType]bool
	// AliasTokens adds to TABLE_ALIAS_TOKENS (parsers/presto.py:69-72).
	AliasTokens     map[tokens.TokenType]bool
	PropertyParsers map[string]propertyParserFunc
	TypeParser      typeParserOverrideFunc
	// ColumnOperators/CastColumnOperators add to COLUMN_OPERATORS/CAST_COLUMN_OPERATORS and
	// FactorTokens to FACTOR (parsers/databricks.py:40-58).
	ColumnOperators     map[tokens.TokenType]columnOpFunc
	CastColumnOperators map[tokens.TokenType]bool
	FactorTokens        map[tokens.TokenType]func(exp.Args) exp.Expression
	// Removals from the shared tables (parsers/clickhouse.py:327-400).
	DisabledNoParenFunctionParsers map[string]bool
	DisabledColumnOperators        map[tokens.TokenType]bool
	DisabledPropertyParsers        map[string]bool
	// FuncTokensAdded extends FUNC_TOKENS (parsers/clickhouse.py:260-266).
	FuncTokensAdded map[tokens.TokenType]bool
	// Full replacements of the shared token sets, precomputed from the base sets.
	ReservedTokensSet   map[tokens.TokenType]bool
	IDVarTokensSet      map[tokens.TokenType]bool
	JoinKindsSet        map[tokens.TokenType]bool
	TableAliasTokensSet map[tokens.TokenType]bool
	AliasTokensSet      map[tokens.TokenType]bool
	// QueryModifierParsers adds QUERY_MODIFIER_PARSERS entries (parsers/clickhouse.py:351-358).
	QueryModifierParsers map[tokens.TokenType]func(*Parser) (string, any)
	// CreatablesSet replaces CREATABLES, TypeTokensAdded extends TYPE_TOKENS,
	// STRUCT_TYPE_TOKENS and NESTED_TYPE_TOKENS, and ColonPlaceholderTokensSet replaces
	// COLON_PLACEHOLDER_TOKENS (parsers/snowflake.py:334-385, 899-908).
	CreatablesSet             map[tokens.TokenType]bool
	TypeTokensAdded           map[tokens.TokenType]bool
	ColonPlaceholderTokensSet map[tokens.TokenType]bool
}

var dialectParserOverrides = map[string]dialectParserOverrideSet{}

// registerDialectParserOverrides configures an override set during package initialization. A nil
// callback is not a removal marker: function removals must use DisabledFunctionParsers explicitly.
func registerDialectParserOverrides(name string, overrides dialectParserOverrideSet) {
	name = strings.ToLower(name)
	if _, exists := dialectParserOverrides[name]; exists {
		panic("parser: duplicate parser overrides for dialect " + name)
	}

	for _, callback := range overrides.FunctionParsers {
		if callback == nil {
			panic("parser: nil function parser override for dialect " + name)
		}
	}
	for _, callback := range overrides.StatementParsers {
		if callback == nil {
			panic("parser: nil statement parser override for dialect " + name)
		}
	}
	for _, callback := range overrides.NoParenFunctionParsers {
		if callback == nil {
			panic("parser: nil no-parentheses function parser override for dialect " + name)
		}
	}
	for _, callback := range overrides.NoParenFunctions {
		if callback == nil {
			panic("parser: nil no-parentheses function override for dialect " + name)
		}
	}
	for _, callback := range overrides.PropertyParsers {
		if callback == nil {
			panic("parser: nil property parser override for dialect " + name)
		}
	}

	dialectParserOverrides[name] = overrides
}

// parserOverrideKey resolves the parser-class overlay independently from the concrete dialect.
// Athena uses a Trino dialect instance for flags and tokenizer behavior while selecting the
// AthenaTrinoParser callback tables.
func (p *Parser) parserOverrideKey() string {
	if p.parserOverrideName != "" {
		return strings.ToLower(p.parserOverrideName)
	}
	return strings.ToLower(p.dialect.Name)
}

func (p *Parser) functionParser(name string) parserOverrideFunc {
	overrides := dialectParserOverrides[p.parserOverrideKey()]
	if parser := overrides.FunctionParsers[name]; parser != nil {
		return parser
	}
	if overrides.DisabledFunctionParsers[name] {
		return nil
	}

	parser := functionParsers[name]
	if parser == nil {
		return nil
	}

	// Keep the current base-singleton compatibility gates on the fallback only. An overlay above
	// may intentionally add or override any of these names for another dialect. These gates use
	// the concrete dialect rather than the parser overlay key: Athena's query parser is real Trino.
	dialectName := strings.ToLower(p.dialect.Name)
	switch name {
	case "VALUES":
		if !p.dialect.ValuesIsFunction {
			return nil
		}
	case "SUBSTR", "JSON_VALUE":
		if dialectName != "mysql" && !(name == "SUBSTR" && p.dialect.Is("spark")) {
			return nil
		}
	case "DATE_PART":
		if dialectName != "postgres" {
			return nil
		}
	}
	return parser
}

func (p *Parser) statementParser(tokenType tokens.TokenType) parserOverrideFunc {
	if parser := dialectParserOverrides[p.parserOverrideKey()].StatementParsers[tokenType]; parser != nil {
		return parser
	}
	return statementParsers[tokenType]
}

// propertyParser mirrors functionParser and statementParser: the selected dialect overlay wins,
// while the shared PROPERTY_PARSERS singleton remains the fallback for every dialect.
func (p *Parser) propertyParser(name string) propertyParserFunc {
	name = strings.ToUpper(name)
	overrides := dialectParserOverrides[p.parserOverrideKey()]
	if parser := overrides.PropertyParsers[name]; parser != nil {
		return parser
	}
	if overrides.DisabledPropertyParsers[name] {
		return nil
	}
	return propertyParsers[name]
}

// propertyParserKeys returns a fresh union because matchTexts needs every overlay key even when the
// same dialect continues to inherit the rest of the shared registry.
func (p *Parser) propertyParserKeys() map[string]bool {
	overrides := dialectParserOverrides[p.parserOverrideKey()]
	overlay := overrides.PropertyParsers
	keys := make(map[string]bool, len(propertyParsers)+len(overlay))
	for name := range propertyParsers {
		if !overrides.DisabledPropertyParsers[name] {
			keys[name] = true
		}
	}
	for name := range overlay {
		keys[name] = true
	}
	return keys
}

func (p *Parser) typeParserOverride() typeParserOverrideFunc {
	return dialectParserOverrides[p.parserOverrideKey()].TypeParser
}

func (p *Parser) noParenFunctionParserFor(name string) parserOverrideFunc {
	overrides := dialectParserOverrides[p.parserOverrideKey()]
	if parser := overrides.NoParenFunctionParsers[name]; parser != nil {
		return parser
	}
	if overrides.DisabledNoParenFunctionParsers[name] {
		return nil
	}

	parser := noParenFunctionParsers[name]
	if parser == nil || (name == "VARIADIC" && strings.ToLower(p.dialect.Name) != "postgres") ||
		(name == "PRIOR" && p.connectPriorDepth == 0) {
		return nil
	}
	return parser
}

func (p *Parser) noParenFunctionFor(tokenType tokens.TokenType) func(exp.Args) exp.Expression {
	overrides := dialectParserOverrides[p.parserOverrideKey()]
	if build := overrides.NoParenFunctions[tokenType]; build != nil {
		return build
	}
	if overrides.DisabledNoParenFunctions[tokenType] {
		return nil
	}
	return noParenFunctions[tokenType]
}

// tableAliasTokensFor is TABLE_ALIAS_TOKENS plus the dialect overlay's additions.
func (p *Parser) tableAliasTokensFor() map[tokens.TokenType]bool {
	if set := dialectParserOverrides[p.parserOverrideKey()].TableAliasTokensSet; set != nil {
		return set
	}
	extra := dialectParserOverrides[p.parserOverrideKey()].AliasTokens
	if len(extra) == 0 {
		return tableAliasTokens
	}
	merged := make(map[tokens.TokenType]bool, len(tableAliasTokens)+len(extra))
	for tt := range tableAliasTokens {
		merged[tt] = true
	}
	for tt := range extra {
		merged[tt] = true
	}
	return merged
}

func (p *Parser) columnOperator(tokenType tokens.TokenType) (columnOpFunc, bool) {
	overrides := dialectParserOverrides[p.parserOverrideKey()]
	if op, ok := overrides.ColumnOperators[tokenType]; ok {
		return op, true
	}
	if overrides.DisabledColumnOperators[tokenType] {
		return nil, false
	}
	op, ok := columnOperators[tokenType]
	return op, ok
}

// aliasTokensFor is ALIAS_TOKENS, or the dialect's replacement set.
func (p *Parser) aliasTokensFor() map[tokens.TokenType]bool {
	if set := dialectParserOverrides[p.parserOverrideKey()].AliasTokensSet; set != nil {
		return set
	}
	return aliasTokens
}

// idVarTokensFor is ID_VAR_TOKENS, or the dialect's replacement set.
func (p *Parser) idVarTokensFor() map[tokens.TokenType]bool {
	if set := dialectParserOverrides[p.parserOverrideKey()].IDVarTokensSet; set != nil {
		return set
	}
	return idVarTokens
}

// reservedTokensFor is RESERVED_TOKENS, or the dialect's replacement set.
func (p *Parser) reservedTokensFor() map[tokens.TokenType]bool {
	if set := dialectParserOverrides[p.parserOverrideKey()].ReservedTokensSet; set != nil {
		return set
	}
	return reservedTokens
}

// joinKindsFor is JOIN_KINDS, or the dialect's replacement set.
func (p *Parser) joinKindsFor() map[tokens.TokenType]bool {
	if set := dialectParserOverrides[p.parserOverrideKey()].JoinKindsSet; set != nil {
		return set
	}
	return joinKinds
}

// queryModifierParser is QUERY_MODIFIER_PARSERS plus the dialect overlay's additions.
func (p *Parser) queryModifierParser(tokenType tokens.TokenType) func(*Parser) (string, any) {
	if parse := dialectParserOverrides[p.parserOverrideKey()].QueryModifierParsers[tokenType]; parse != nil {
		return parse
	}
	if queryModifierTokens[tokenType] {
		return queryModifierParsers[tokenType]
	}
	return nil
}

func (p *Parser) isFuncToken(tokenType tokens.TokenType) bool {
	return funcTokens[tokenType] || p.dialect.FuncTokens[tokenType] || dialectParserOverrides[p.parserOverrideKey()].FuncTokensAdded[tokenType]
}

func (p *Parser) isCastColumnOperator(tokenType tokens.TokenType) bool {
	return castColumnOperators[tokenType] || dialectParserOverrides[p.parserOverrideKey()].CastColumnOperators[tokenType]
}

// isColumnFastBail is columnFastBailTokens plus the overlay's COLUMN_OPERATORS.
func (p *Parser) isColumnFastBail(tokenType tokens.TokenType) bool {
	if columnFastBailTokens[tokenType] {
		return true
	}
	_, ok := dialectParserOverrides[p.parserOverrideKey()].ColumnOperators[tokenType]
	return ok
}

func (p *Parser) factorToken(tokenType tokens.TokenType) (func(exp.Args) exp.Expression, bool) {
	if build, ok := dialectParserOverrides[p.parserOverrideKey()].FactorTokens[tokenType]; ok {
		return build, true
	}
	build, ok := factorTokens[tokenType]
	return build, ok
}

// creatablesFor is CREATABLES, or the dialect's replacement set.
func (p *Parser) creatablesFor() map[tokens.TokenType]bool {
	if set := dialectParserOverrides[p.parserOverrideKey()].CreatablesSet; set != nil {
		return set
	}
	return creatables
}

// isTypeToken is TYPE_TOKENS plus the dialect overlay's additions.
func (p *Parser) isTypeToken(tokenType tokens.TokenType) bool {
	return typeTokens[tokenType] || dialectParserOverrides[p.parserOverrideKey()].TypeTokensAdded[tokenType]
}

// typeTokensFor is TYPE_TOKENS plus the dialect overlay's additions.
func (p *Parser) typeTokensFor() map[tokens.TokenType]bool {
	extra := dialectParserOverrides[p.parserOverrideKey()].TypeTokensAdded
	if len(extra) == 0 {
		return typeTokens
	}
	return withTokens(typeTokens, keysOf(extra)...)
}

func keysOf(set map[tokens.TokenType]bool) []tokens.TokenType {
	out := make([]tokens.TokenType, 0, len(set))
	for tt := range set {
		out = append(out, tt)
	}
	return out
}

// colonPlaceholderTokensFor is COLON_PLACEHOLDER_TOKENS (parser.py: ID_VAR_TOKENS by default).
func (p *Parser) colonPlaceholderTokensFor() map[tokens.TokenType]bool {
	if set := dialectParserOverrides[p.parserOverrideKey()].ColonPlaceholderTokensSet; set != nil {
		return set
	}
	return idVarTokens
}
