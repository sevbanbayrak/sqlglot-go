package parser

import (
	"sort"
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// parsers/clickhouse.py: ClickHouseParser, a direct Parser subclass. Its flags and FUNCTIONS
// overlay live on dialects.ClickHouse; the method overrides that hook into shared parser code
// are gated there with p.dialect.Is("clickhouse").
func init() {
	registerDialectParserOverrides("clickhouse", dialectParserOverrideSet{
		FunctionParsers: map[string]parserOverrideFunc{
			"ARRAYJOIN": func(p *Parser) exp.Expression {
				return p.expression(exp.New(exp.KindExplode, exp.Args{"this": p.parseExpression()}), nil, nil)
			},
			"GROUPCONCAT": (*Parser).parseClickHouseGroupConcat,
			"QUANTILE":    (*Parser).parseClickHouseQuantile,
			"MEDIAN":      (*Parser).parseClickHouseQuantile,
			"COLUMNS":     (*Parser).parseClickHouseColumns,
			"TUPLE": func(p *Parser) exp.Expression {
				return exp.FromArgList(exp.KindStruct, p.parseFunctionArgs(true))
			},
			"AND": func(p *Parser) exp.Expression { return p.parseConnectorFunction(exp.And_) },
			"OR":  func(p *Parser) exp.Expression { return p.parseConnectorFunction(exp.Or_) },
			"XOR": func(p *Parser) exp.Expression { return exp.Xor_(p.parseFunctionArgs(false)...) },
		},
		DisabledFunctionParsers:        map[string]bool{"MATCH": true},
		DisabledNoParenFunctionParsers: map[string]bool{"ANY": true},
		DisabledNoParenFunctions:       map[tokens.TokenType]bool{tokens.CURRENT_TIMESTAMP: true},
		DisabledColumnOperators:        map[tokens.TokenType]bool{tokens.PLACEHOLDER: true},
		ColumnOperators: map[tokens.TokenType]columnOpFunc{
			tokens.DOTCARET: func(p *Parser, this, field exp.Expression) exp.Expression {
				return p.expression(exp.New(exp.KindNestedJSONSelect, exp.Args{"this": this, "expression": field}), nil, nil)
			},
		},
		DisabledPropertyParsers: map[string]bool{"DYNAMIC": true},
		PropertyParsers: map[string]propertyParserFunc{
			"ENGINE":  func(p *Parser, _ bool) exp.Expression { return p.parseClickHouseEngineProperty() },
			"REFRESH": func(p *Parser, _ bool) exp.Expression { return p.parseClickHouseAutoRefreshProperty() },
			"UUID": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.New(exp.KindUuidProperty, exp.Args{"this": p.parseString()}), nil, nil)
			},
		},
		StatementParsers: map[tokens.TokenType]parserOverrideFunc{
			tokens.DETACH: (*Parser).parseClickHouseDetach,
		},
		TypeParser:          (*Parser).parseClickHouseTypes,
		FuncTokensAdded:     map[tokens.TokenType]bool{tokens.AND: true, tokens.FILE: true, tokens.OR: true, tokens.SET: true},
		ReservedTokensSet:   withoutTokens(reservedTokens, tokens.SELECT),
		IDVarTokensSet:      withTokens(idVarTokens, tokens.LIKE),
		JoinKindsSet:        withTokens(joinKinds, tokens.ALL, tokens.ANY, tokens.ASOF, tokens.ARRAY),
		TableAliasTokensSet: withoutTokens(tableAliasTokens, tokens.ALL, tokens.ANY, tokens.ARRAY, tokens.ASOF, tokens.FINAL, tokens.FORMAT, tokens.SETTINGS),
		AliasTokensSet:      withoutTokens(aliasTokens, tokens.FORMAT, tokens.SETTINGS),
		QueryModifierParsers: map[tokens.TokenType]func(*Parser) (string, any){
			tokens.SETTINGS: func(p *Parser) (string, any) {
				p.advance()
				return "settings", nonEmpty(p.parseCsv(p.parseAssignment))
			},
			tokens.FORMAT: func(p *Parser) (string, any) {
				p.advance()
				return "format", p.parseIdVar(true, nil)
			},
		},
	})
}

func withTokens(base map[tokens.TokenType]bool, add ...tokens.TokenType) map[tokens.TokenType]bool {
	out := make(map[tokens.TokenType]bool, len(base)+len(add))
	for tt, ok := range base {
		out[tt] = ok
	}
	for _, tt := range add {
		out[tt] = true
	}
	return out
}

func withoutTokens(base map[tokens.TokenType]bool, remove ...tokens.TokenType) map[tokens.TokenType]bool {
	out := make(map[tokens.TokenType]bool, len(base))
	for tt, ok := range base {
		out[tt] = ok
	}
	for _, tt := range remove {
		delete(out, tt)
	}
	return out
}

func nonEmpty(list []exp.Expression) any {
	if len(list) == 0 {
		return nil
	}
	return list
}

// clickHouseAggFunctions ports _AGG_FUNCTIONS (parsers/clickhouse.py:91-209).
var clickHouseAggFunctions = []string{
	"count", "min", "max", "sum", "avg", "any", "stddevPop", "stddevSamp", "varPop", "varSamp",
	"corr", "covarPop", "covarSamp", "entropy", "exponentialMovingAverage", "intervalLengthSum",
	"kolmogorovSmirnovTest", "mannWhitneyUTest", "median", "rankCorr", "sumKahan", "studentTTest",
	"welchTTest", "anyHeavy", "anyLast", "boundingRatio", "first_value", "last_value", "argMin",
	"argMax", "avgWeighted", "topK", "approx_top_sum", "topKWeighted", "deltaSum",
	"deltaSumTimestamp", "groupArray", "groupArrayLast", "groupConcat", "groupUniqArray",
	"groupArrayInsertAt", "groupArrayMovingAvg", "groupArrayMovingSum", "groupArraySample",
	"groupBitAnd", "groupBitOr", "groupBitXor", "groupBitmap", "groupBitmapAnd", "groupBitmapOr",
	"groupBitmapXor", "sumWithOverflow", "sumMap", "minMap", "maxMap", "skewSamp", "skewPop",
	"kurtSamp", "kurtPop", "uniq", "uniqExact", "uniqCombined", "uniqCombined64", "uniqHLL12",
	"uniqTheta", "quantile", "quantiles", "quantileExact", "quantileExactInclusive",
	"quantilesExact", "quantilesExactExclusive", "quantileExactLow", "quantilesExactLow",
	"quantileExactHigh", "quantilesExactHigh", "quantileExactWeighted", "quantilesExactWeighted",
	"quantileTiming", "quantilesTiming", "quantileTimingWeighted", "quantilesTimingWeighted",
	"quantileDeterministic", "quantilesDeterministic", "quantileTDigest", "quantilesTDigest",
	"quantileTDigestWeighted", "quantilesTDigestWeighted", "quantileBFloat16", "quantilesBFloat16",
	"quantileBFloat16Weighted", "quantilesBFloat16Weighted", "simpleLinearRegression",
	"stochasticLinearRegression", "stochasticLogisticRegression", "categoricalInformationValue",
	"contingency", "cramersV", "cramersVBiasCorrected", "theilsU", "maxIntersections",
	"maxIntersectionsPosition", "meanZTest", "quantileInterpolatedWeighted",
	"quantilesInterpolatedWeighted", "quantileGK", "quantilesGK", "sparkBar", "sumCount",
	"largestTriangleThreeBuckets", "histogram", "sequenceMatch", "sequenceCount", "windowFunnel",
	"retention", "uniqUpTo", "sequenceNextNode", "exponentialTimeDecayedAvg",
}

// clickHouseAggFunctionsSuffixes ports _AGG_FUNCTIONS_SUFFIXES (parsers/clickhouse.py:213-234),
// sorted longest-first so that compound suffixes (e.g. "SimpleState") are matched before their
// sub-suffixes (e.g. "State").
var clickHouseAggFunctionsSuffixes = func() []string {
	suffixes := []string{
		"If", "Array", "ArrayIf", "Map", "SimpleState", "State", "Merge", "MergeState", "ForEach",
		"Distinct", "OrDefault", "OrNull", "Resample", "ArgMin", "ArgMax",
	}
	// Python's sorted is stable: equal-length suffixes keep their declaration order.
	sort.SliceStable(suffixes, func(i, j int) bool { return len(suffixes[i]) > len(suffixes[j]) })
	return suffixes
}()

type clickHouseAggParts struct {
	name   string
	suffix string
}

// clickHouseAggFuncMapping ports _AGG_FUNC_MAPPING (parsers/clickhouse.py:237-239): every 0-
// and 1-suffix aggregate function name.
var clickHouseAggFuncMapping = func() map[string]clickHouseAggParts {
	mapping := map[string]clickHouseAggParts{}
	for _, suffix := range clickHouseAggFunctionsSuffixes {
		for _, f := range clickHouseAggFunctions {
			mapping[f+suffix] = clickHouseAggParts{f, suffix}
		}
	}
	for _, f := range clickHouseAggFunctions {
		mapping[f] = clickHouseAggParts{f, ""}
	}
	return mapping
}()

// resolveClickHouseAgg ports _resolve_clickhouse_agg (parsers/clickhouse.py:275-305).
func resolveClickHouseAgg(name string) (string, []string, bool) {
	var accumulated []string
	for {
		parts, ok := clickHouseAggFuncMapping[name]
		if ok {
			if parts.suffix != "" {
				accumulated = append([]string{parts.suffix}, accumulated...)
			}
			return parts.name, accumulated, true
		}
		stripped := false
		for _, suffix := range clickHouseAggFunctionsSuffixes {
			if strings.HasSuffix(name, suffix) && len(name) != len(suffix) {
				accumulated = append([]string{suffix}, accumulated...)
				name = name[:len(name)-len(suffix)]
				stripped = true
				break
			}
		}
		if !stripped {
			return "", nil, false
		}
	}
}

// parseClickHouseFunction ports the _parse_function override (parsers/clickhouse.py:612-659):
// aggregate functions can be split in 2 parts: <func_name><suffix[es]>, optionally followed
// by a parameter list, e.g. quantileIf(0.5)(x, cond).
func (p *Parser) parseClickHouseFunction(expr exp.Expression) exp.Expression {
	fn := expr
	if expr != nil && expr.Kind() == exp.KindWindow {
		fn = expr.This()
	}
	if fn == nil || fn.Kind() != exp.KindAnonymous {
		return expr
	}
	name, ok := fn.Arg("this").(string)
	if !ok {
		return expr
	}
	_, suffixes, ok := resolveClickHouseAgg(name)
	if !ok {
		return expr
	}
	params := p.parseClickHouseFuncParams(fn)
	var kind exp.Kind
	if len(suffixes) > 0 {
		kind = exp.KindCombinedAggFunc
		if params != nil {
			kind = exp.KindCombinedParameterizedAgg
		}
	} else {
		kind = exp.KindAnonymousAggFunc
		if params != nil {
			kind = exp.KindParameterizedAgg
		}
	}
	instance := exp.New(kind, exp.Args{"this": fn.Arg("this"), "expressions": fn.Arg("expressions")})
	if params != nil {
		instance.Set("params", params)
	}
	instance = p.expression(instance, nil, nil)
	if expr.Kind() == exp.KindWindow {
		// The window's func was parsed as Anonymous in base parser, fix its type to be
		// ClickHouse style CombinedAnonymousAggFunc / AnonymousAggFunc
		expr.Set("this", instance)
		return expr
	}
	if params != nil {
		// Params have blocked the base parser from parsing the following window (if that
		// exists) as they're standing between the function call and the window spec
		return p.parseWindow(instance, false)
	}
	return instance
}

// parseClickHouseFuncParams ports _parse_func_params (parsers/clickhouse.py:661-670).
func (p *Parser) parseClickHouseFuncParams(this exp.Expression) []exp.Expression {
	if p.matchPair(tokens.R_PAREN, tokens.L_PAREN, true) {
		return p.parseCsv(func() exp.Expression { return p.parseLambda(false) })
	}
	if p.match(tokens.L_PAREN) {
		params := p.parseCsv(func() exp.Expression { return p.parseLambda(false) })
		p.matchRParen(this)
		return params
	}
	return nil
}

// parseClickHouseGroupConcat ports _parse_group_concat (parsers/clickhouse.py:672-687).
func (p *Parser) parseClickHouseGroupConcat() exp.Expression {
	args := p.parseCsv(func() exp.Expression { return p.parseLambda(false) })
	params := p.parseClickHouseFuncParams(nil)
	if params != nil {
		// groupConcat(sep [, limit])(expr)
		separator := seqGet(args, 0)
		limit := seqGet(args, 1)
		this := seqGet(params, 0)
		if limit != nil {
			this = exp.New(exp.KindLimit, exp.Args{"this": this, "expression": limit})
		}
		return p.expression(exp.New(exp.KindGroupConcat, exp.Args{"this": this, "separator": separator}), nil, nil)
	}
	// groupConcat(expr)
	return p.expression(exp.New(exp.KindGroupConcat, exp.Args{"this": seqGet(args, 0)}), nil, nil)
}

// parseClickHouseQuantile ports _parse_quantile (parsers/clickhouse.py:689-694).
func (p *Parser) parseClickHouseQuantile() exp.Expression {
	this := p.parseLambda(false)
	params := p.parseClickHouseFuncParams(nil)
	if len(params) > 0 {
		return p.expression(exp.New(exp.KindQuantile, exp.Args{"this": params[0], "quantile": this}), nil, nil)
	}
	return p.expression(exp.New(exp.KindQuantile, exp.Args{"this": this, "quantile": exp.LiteralNumber(0.5)}), nil, nil)
}

// parseClickHouseColumns ports _parse_columns (parsers/clickhouse.py:877-883).
func (p *Parser) parseClickHouseColumns() exp.Expression {
	this := p.expression(exp.New(exp.KindColumns, exp.Args{"this": p.parseLambda(false)}), nil, nil)
	for p.next.IsValid() && p.matchTextSeq(")", "APPLY", "(") {
		p.match(tokens.R_PAREN)
		this = exp.New(exp.KindApply, exp.Args{"this": this, "expression": p.parseVar(true, nil, false)})
	}
	return this
}

// parseClickHouseTypes ports the _parse_types override (parsers/clickhouse.py:420-441): every
// type is marked non-nullable, ClickHouse's default, unless it is already Nullable(...).
func (p *Parser) parseClickHouseTypes(checkFunc, schema, allowIdentifiers, withCollation bool) exp.Expression {
	dtype := p.parseTypesBase(checkFunc, schema, allowIdentifiers, withCollation)
	if dtype != nil && dtype.Kind() == exp.KindDataType && dtype.Arg("nullable") != true {
		// This marker helps us transpile types from other dialects to ClickHouse, so that we
		// can e.g. produce `CAST(x AS Nullable(String))` from `CAST(x AS TEXT)`.
		dtype.Set("nullable", false)
	}
	return dtype
}

// parseClickHouseExtract ports _parse_extract (parsers/clickhouse.py:443-458): the two-argument
// extract(haystack, pattern) is a regex function, kept Anonymous.
func (p *Parser) parseClickHouseExtract() exp.Expression {
	index := p.index
	this := p.parseBitwise()
	if p.match(tokens.FROM) {
		p.retreat(index)
		return nil
	}
	p.match(tokens.COMMA)
	return p.expression(exp.Anonymous(exp.Args{"this": "extract", "expressions": []exp.Expression{this, p.parseBitwise()}}), nil, nil)
}

// parseClickHouseTernary ports the _parse_assignment override (parsers/clickhouse.py:460-472):
// `cond ? a : b` builds an If.
func (p *Parser) parseClickHouseTernary(this exp.Expression) exp.Expression {
	if !p.match(tokens.PLACEHOLDER) {
		return this
	}
	trueExpr := p.parseAssignment()
	var falseExpr exp.Expression
	if p.match(tokens.COLON) {
		falseExpr = p.parseAssignment()
	}
	return p.expression(exp.If(exp.Args{"this": this, "true": trueExpr, "false": falseExpr}), nil, nil)
}

// parseClickHouseQueryParameter ports _parse_query_parameter (parsers/clickhouse.py:474-498):
// a placeholder like {abc: UInt32} or {table: Identifier}.
func (p *Parser) parseClickHouseQueryParameter() exp.Expression {
	index := p.index
	this := p.parseIdVar(true, nil)
	p.match(tokens.COLON)
	var kind any
	if dtype := p.parseTypes(false, false, false, false); dtype != nil {
		kind = dtype
	} else if p.matchTextSeq("IDENTIFIER") {
		kind = "Identifier"
	}
	if kind == nil {
		p.retreat(index)
		return nil
	}
	if !p.match(tokens.R_BRACE) {
		p.raiseError("Expecting }")
	}
	if this != nil && this.Kind() == exp.KindIdentifier && !truthyArg(this.Arg("quoted")) {
		this = exp.Var(exp.Args{"this": this.Name()})
	}
	return p.expression(exp.Placeholder(exp.Args{"this": this, "kind": kind}), nil, nil)
}

func truthyArg(v any) bool {
	b, _ := v.(bool)
	return b
}

// parseClickHouseBracket ports the _parse_bracket override (parsers/clickhouse.py:500-526):
// `x[]` casts to an array of JSON, and `{'k': v}` builds a map.
func (p *Parser) parseClickHouseBracket(this exp.Expression) exp.Expression {
	if this != nil {
		var bracketJSONType exp.Expression
		for p.matchPair(tokens.L_BRACKET, tokens.R_BRACKET, true) {
			inner := bracketJSONType
			if inner == nil {
				inner = exp.DTypeJSON.IntoExpr(exp.Args{"nullable": false})
			}
			bracketJSONType = exp.DataType(exp.Args{"this": exp.DTypeArray, "expressions": []exp.Expression{inner}, "nested": true})
		}
		if bracketJSONType != nil {
			return p.expression(exp.JSONCast(exp.Args{"this": this, "to": bracketJSONType}), nil, nil)
		}
	}
	lBrace := p.match(tokens.L_BRACE, false)
	bracket := p.parseBracketBase(this)
	if lBrace && bracket != nil && bracket.Kind() == exp.KindStruct {
		keys := exp.Array(exp.Args{"expressions": []exp.Expression{}})
		values := exp.Array(exp.Args{"expressions": []exp.Expression{}})
		for _, e := range bracket.Expressions() {
			if e.Kind() != exp.KindPropertyEQ {
				break
			}
			keys.Append("expressions", exp.LiteralString(e.Name()))
			values.Append("expressions", e.Expr())
		}
		return exp.New(exp.KindVarMap, exp.Args{"keys": keys, "values": values})
	}
	return bracket
}

// parseClickHouseGlobalIn ports _parse_global_in (parsers/clickhouse.py:528-534).
func (p *Parser) parseClickHouseGlobalIn(this exp.Expression) exp.Expression {
	isNegated := p.match(tokens.NOT)
	var inExpr exp.Expression
	if p.match(tokens.IN) {
		inExpr = p.parseIn(this, false)
		inExpr.Set("is_global", true)
	}
	if isNegated {
		return p.expression(exp.Not(exp.Args{"this": inExpr}), nil, nil)
	}
	return inExpr
}

// parseClickHouseCTE ports _parse_cte (parsers/clickhouse.py:571-582).
func (p *Parser) parseClickHouseCTE() exp.Expression {
	// WITH <identifier> AS <subquery expression>
	cte := p.tryParse(p.parseCTEBase, false)
	if cte == nil {
		// WITH <expression> AS <identifier>
		cte = p.expression(exp.CTE(exp.Args{
			"this":   p.parseAssignment(),
			"alias":  p.parseTableAlias(nil),
			"scalar": true,
		}), nil, nil)
	}
	return cte
}

// parseClickHouseEngineProperty ports _parse_engine_property (parsers/clickhouse.py:406-411).
func (p *Parser) parseClickHouseEngineProperty() exp.Expression {
	p.match(tokens.EQ)
	return p.expression(exp.EngineProperty(exp.Args{"this": p.parseField(true, nil, true)}), nil, nil)
}

// listArg returns a list-valued arg as expressions (nil when unset).
func listArg(e exp.Expression, key string) []exp.Expression {
	switch v := e.Arg(key).(type) {
	case []exp.Expression:
		return v
	case []any:
		out := make([]exp.Expression, 0, len(v))
		for _, item := range v {
			if expr, ok := item.(exp.Expression); ok {
				out = append(out, expr)
			}
		}
		return out
	}
	return nil
}
