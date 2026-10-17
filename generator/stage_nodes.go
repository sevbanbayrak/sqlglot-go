package generator

import (
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/expressions"
)

// Base renderers (generator.py) for the stage, time-travel, sequence and property nodes the
// Snowflake parser builds.
func init() {
	dispatch[expressions.KindTableFromRows] = (*Generator).tableFromRowsSQL
	dispatch[expressions.KindHistoricalData] = func(g *Generator, e expressions.Expression) string {
		// historicaldata_sql (generator.py:2400-2404).
		return g.sqlKey(e, "this") + " (" + g.sqlKey(e, "kind") + " => " + g.sqlKey(e, "expression") + ")"
	}
	dispatch[expressions.KindChanges] = func(g *Generator, e expressions.Expression) string {
		// changes_sql (generator.py:5562-5570).
		information := "INFORMATION => " + g.sqlKey(e, "information")
		atBefore := g.sqlKey(e, "at_before")
		if atBefore != "" {
			atBefore = g.seg("") + atBefore
		}
		end := g.sqlKey(e, "end")
		if end != "" {
			end = g.seg("") + end
		}
		return "CHANGES (" + information + ")" + atBefore + end
	}
	dispatch[expressions.KindAtIndex] = func(g *Generator, e expressions.Expression) string {
		// atindex_sql (generator.py:4046-4049).
		return g.sqlKey(e, "this") + " AT " + g.sqlKey(e, "expression")
	}
	dispatch[expressions.KindSequenceProperties] = (*Generator).sequencePropertiesSQL
	dispatch[expressions.KindGet] = (*Generator).getPutSQL
	dispatch[expressions.KindPut] = (*Generator).getPutSQL
	dispatch[expressions.KindDirectoryStage] = func(g *Generator, e expressions.Expression) string {
		// directorystage_sql (generator.py:6234-6235).
		return g.funcCall("DIRECTORY", []any{e.This()}, "(", ")", true)
	}
	dispatch[expressions.KindSemanticView] = (*Generator).semanticViewSQL
	dispatch[expressions.KindMaskingPolicyColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		// maskingpolicycolumnconstraint_sql (generator.py:5520-5526).
		exprs := g.expressions(exprsOptions{expression: e, flat: true})
		if exprs != "" {
			exprs = " USING (" + exprs + ")"
		}
		return "MASKING POLICY " + g.sqlKey(e, "this") + exprs
	}
	dispatch[expressions.KindProjectionPolicyColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		// generator.py TRANSFORMS ProjectionPolicyColumnConstraint.
		return "PROJECTION POLICY " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindTags] = func(g *Generator, e expressions.Expression) string {
		return "TAG (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
	}
	dispatch[expressions.KindUsingTemplateProperty] = func(g *Generator, e expressions.Expression) string {
		return "USING TEMPLATE " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindCredentialsProperty] = func(g *Generator, e expressions.Expression) string {
		return "CREDENTIALS=(" + g.expressions(exprsOptions{expression: e, key: "expressions", sep: " "}) + ")"
	}
	dispatch[expressions.KindCopyGrantsProperty] = func(*Generator, expressions.Expression) string { return "COPY GRANTS" }
	dispatch[expressions.KindSetProperty] = func(_ *Generator, e expressions.Expression) string {
		if truthy(e.Arg("multi")) {
			return "MULTISET"
		}
		return "SET"
	}
	dispatch[expressions.KindTransientProperty] = func(*Generator, expressions.Expression) string { return "TRANSIENT" }
	dispatch[expressions.KindAlterSession] = func(g *Generator, e expressions.Expression) string {
		// altersession_sql (generator.py:4378-4381).
		keyword := "SET"
		if truthy(e.Arg("unset")) {
			keyword = "UNSET"
		}
		return keyword + " " + g.expressions(exprsOptions{expression: e, flat: true})
	}
	dispatch[expressions.KindUuid] = func(g *Generator, e expressions.Expression) string {
		// uuid_sql (generator.py:6237-6244).
		uuidSQL := g.funcCall("UUID", nil, "(", ")", true)
		if truthy(e.Arg("is_string")) && !g.dialect.UUIDIsStringType {
			return g.gen(expressions.Cast(expressions.Args{"this": expressions.New(expressions.KindUuid, nil), "to": expressions.DTypeVarchar.IntoExpr(nil)}))
		}
		return uuidSQL
	}
	dispatch[expressions.KindSpace] = func(g *Generator, e expressions.Expression) string {
		// space_sql (generator.py:6210-6211).
		return g.gen(expressions.New(expressions.KindRepeat, expressions.Args{"this": expressions.LiteralString(" "), "times": e.This()}))
	}
	if dispatch[expressions.KindExternalProperty] == nil {
		dispatch[expressions.KindExternalProperty] = func(*Generator, expressions.Expression) string { return "EXTERNAL" }
	}
	dispatch[expressions.KindConnectByRoot] = func(g *Generator, e expressions.Expression) string {
		return "CONNECT_BY_ROOT " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindToChar] = func(g *Generator, e expressions.Expression) string {
		// tochar_sql (generator.py:4889-4890).
		return g.gen(expressions.CastTo(e.This(), expressions.DTypeText))
	}
	dispatch[expressions.KindSafeDivide] = func(g *Generator, e expressions.Expression) string {
		// safedivide_sql (generator.py:4472-4475).
		n := expressions.Wrap(e.This(), expressions.TraitBinary)
		d := expressions.Wrap(e.Expr(), expressions.TraitBinary)
		return g.gen(expressions.If(expressions.Args{
			"this":  expressions.BinOp(expressions.KindNEQ, d, 0),
			"true":  expressions.BinOp(expressions.KindDiv, n, d),
			"false": expressions.Null(),
		}))
	}
	dispatch[expressions.KindDateFromUnixDate] = func(g *Generator, e expressions.Expression) string {
		// datefromunixdate_sql (generator.py:6201-6208).
		return g.gen(expressions.New(expressions.KindDateAdd, expressions.Args{
			"this":       expressions.CastTo(expressions.LiteralString("1970-01-01"), expressions.DTypeDate),
			"expression": e.This(),
			"unit":       expressions.Var(expressions.Args{"this": "DAY"}),
		}))
	}
	if dispatch[expressions.KindStrToTime] == nil {
		dispatch[expressions.KindStrToTime] = func(g *Generator, e expressions.Expression) string {
			// strtotime_sql (generator.py:4144-4145).
			return g.funcCall("STR_TO_TIME", []any{e.This(), e.Arg("format")}, "(", ")", true)
		}
	}
	dispatch[expressions.KindToArray] = func(g *Generator, e expressions.Expression) string {
		// toarray_sql (generator.py:5158-5169)
		arg := e.This()
		if arg.Type() == nil {
			arg = g.annotateTypes(arg)
		}
		if arg.IsType(expressions.DTypeArray) {
			return g.gen(arg)
		}
		return g.gen(expressions.If(expressions.Args{
			"this":  expressions.Is(expressions.Args{"this": arg.Copy(), "expression": expressions.Null()}),
			"true":  expressions.Null(),
			"false": expressions.Array(expressions.Args{"expressions": []expressions.Expression{arg}}),
		}))
	}
	dispatch[expressions.KindGetExtract] = func(g *Generator, e expressions.Expression) string {
		// getextract_sql (generator.py:6187-6198)
		this, expr := e.This(), asExpression(e.Arg("expression"))
		if this.Type() == nil || e.Type() == nil {
			this = g.annotateTypes(this)
		}
		if this.IsType(expressions.DTypeArray, expressions.DTypeMap) {
			return g.gen(expressions.Bracket(expressions.Args{"this": this, "expressions": []expressions.Expression{expr}}))
		}
		return g.gen(expressions.New(expressions.KindJSONExtract, expressions.Args{"this": this, "expression": g.dialect.ToJSONPath(expr)}))
	}
	dispatch[expressions.KindSwapTable] = func(g *Generator, e expressions.Expression) string {
		return "SWAP WITH " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindConditionalInsert] = func(g *Generator, e expressions.Expression) string {
		// conditionalinsert_sql (generator.py:5648-5653)
		condition := g.sqlKey(e, "expression")
		if condition != "" {
			condition = "WHEN " + condition + " THEN "
		} else if e.Arg("else_") == true {
			condition = "ELSE "
		}
		return condition + strings.TrimSpace(strings.TrimPrefix(g.sqlKey(e, "this"), "INSERT"))
	}
	dispatch[expressions.KindMultitableInserts] = func(g *Generator, e expressions.Expression) string {
		// multitableinserts_sql (generator.py:5655-5659)
		return "INSERT " + g.sqlKey(e, "kind") + g.seg(g.expressions(exprsOptions{expression: e, sep: " "})) + g.seg(g.sqlKey(e, "source"))
	}
	for kind, keyword := range map[expressions.Kind]string{
		expressions.KindApiProperty:                 "API",
		expressions.KindApplicationProperty:         "APPLICATION",
		expressions.KindCatalogProperty:             "CATALOG",
		expressions.KindComputeProperty:             "COMPUTE",
		expressions.KindDatabaseProperty:            "DATABASE",
		expressions.KindDynamicProperty:             "DYNAMIC",
		expressions.KindHybridProperty:              "HYBRID",
		expressions.KindIcebergProperty:             "ICEBERG",
		expressions.KindMaskingProperty:             "MASKING",
		expressions.KindNetworkProperty:             "NETWORK",
		expressions.KindSecurityIntegrationProperty: "SECURITY",
	} {
		keyword := keyword
		dispatch[kind] = func(*Generator, expressions.Expression) string { return keyword }
	}
	dispatch[expressions.KindRand] = func(g *Generator, e expressions.Expression) string {
		// rand_sql (generator.py:5554-5560)
		lower, upper := g.sqlKey(e, "lower"), g.sqlKey(e, "upper")
		if lower != "" && upper != "" {
			return "(" + upper + " - " + lower + ") * " + g.funcCall("RAND", []any{e.This()}, "(", ")", true) + " + " + lower
		}
		return g.funcCall("RAND", []any{e.This()}, "(", ")", true)
	}
	dispatch[expressions.KindTsOrDsToTime] = func(g *Generator, e expressions.Expression) string {
		// tsordstotime_sql (generator.py:5171-5186)
		this := e.This()
		if g.formatTime(e) != nil {
			strToTime := expressions.New(expressions.KindStrToTime, expressions.Args{"this": this, "format": e.Arg("format")})
			return g.gen(expressions.CastTo(strToTime, expressions.DTypeTime))
		}
		if this.Kind() == expressions.KindTsOrDsToTime || this.IsType(expressions.DTypeTime) {
			return g.gen(this)
		}
		return g.gen(expressions.CastTo(this, expressions.DTypeTime))
	}
	dispatch[expressions.KindToDouble] = func(g *Generator, e expressions.Expression) string {
		// todouble_sql (generator.py:5844-5846)
		kind := expressions.KindCast
		if boolValue(e.Arg("safe")) {
			kind = expressions.KindTryCast
		}
		return g.gen(expressions.New(kind, expressions.Args{"this": e.This(), "to": expressions.DTypeDouble.IntoExpr(nil)}))
	}
	dispatch[expressions.KindDynamicIdentifier] = func(g *Generator, e expressions.Expression) string {
		// dynamicidentifier_sql (generator.py:1986-1998)
		if this := e.This(); this != nil && this.IsString() {
			if _, ok := e.Arg("expressions").([]expressions.Expression); ok {
				// `IDENTIFIER(...)` invoked as a function, e.g. `IDENTIFIER('my_func')(1, 2)`
				// We can't safely emit the call to other dialects since name/arg semantics may differ
				g.unsupported("Transpiling dynamically-invoked IDENTIFIER() functions is unsupported")
			}
			if resolved := expressions.MaybeParse(this.Name(), nil, false); resolved != nil {
				return g.gen(resolved)
			}
		}
		g.unsupported("IDENTIFIER() with non-literal arguments is not supported")
		return g.funcCall("IDENTIFIER", []any{e.This()}, "(", ")", true)
	}
	dispatch[expressions.KindModelAttribute] = func(g *Generator, e expressions.Expression) string {
		// modelattribute_sql (generator.py:6230-6232)
		g.unsupported("The model!attribute syntax is not supported")
		return ""
	}
	dispatch[expressions.KindAllowedValuesProperty] = func(g *Generator, e expressions.Expression) string {
		return "ALLOWED_VALUES " + g.expressions(exprsOptions{expression: e, flat: true})
	}
	dispatch[expressions.KindExecuteAsProperty] = func(g *Generator, e expressions.Expression) string {
		return "EXECUTE AS " + g.sqlKey(e, "this")
	}
	for kind, location := range map[expressions.Kind]propertyLocation{
		expressions.KindAllowedValuesProperty:       propertyLocationPostSchema,
		expressions.KindExecuteAsProperty:           propertyLocationPostSchema,
		expressions.KindApiProperty:                 propertyLocationPostCreate,
		expressions.KindApplicationProperty:         propertyLocationPostCreate,
		expressions.KindCatalogProperty:             propertyLocationPostCreate,
		expressions.KindComputeProperty:             propertyLocationPostCreate,
		expressions.KindCopyGrantsProperty:          propertyLocationPostSchema,
		expressions.KindDatabaseProperty:            propertyLocationPostCreate,
		expressions.KindDynamicProperty:             propertyLocationPostCreate,
		expressions.KindExternalProperty:            propertyLocationPostCreate,
		expressions.KindHybridProperty:              propertyLocationPostCreate,
		expressions.KindIcebergProperty:             propertyLocationPostCreate,
		expressions.KindMaskingProperty:             propertyLocationPostCreate,
		expressions.KindNetworkProperty:             propertyLocationPostCreate,
		expressions.KindRowAccessProperty:           propertyLocationUnsupported,
		expressions.KindSecurityIntegrationProperty: propertyLocationPostCreate,
		expressions.KindSequenceProperties:          propertyLocationPostExpression,
		expressions.KindSetProperty:                 propertyLocationPostCreate,
		expressions.KindTags:                        propertyLocationPostWith,
		expressions.KindTransientProperty:           propertyLocationPostCreate,
		expressions.KindUsingTemplateProperty:       propertyLocationPostSchema,
	} {
		if _, ok := propertyLocations[kind]; !ok {
			propertyLocations[kind] = location
		}
	}
}

// tableFromRowsSQL ports tablefromrows_sql (generator.py:2484-2493).
func (g *Generator) tableFromRowsSQL(e expressions.Expression) string {
	table := g.funcCall("TABLE", []any{e.This()}, "(", ")", true)
	alias := g.sqlKey(e, "alias")
	if alias != "" {
		alias = " AS " + alias
	}
	sample := g.sqlKey(e, "sample")
	pivots := g.expressions(exprsOptions{expression: e, key: "pivots", emptySep: true, flat: true})
	joins := g.indent(g.expressions(exprsOptions{expression: e, key: "joins", emptySep: true, flat: true}), 0, nil, true, false)
	return table + alias + pivots + sample + joins
}

// sequencePropertiesSQL ports sequenceproperties_sql (generator.py:1439-1462).
func (g *Generator) sequencePropertiesSQL(e expressions.Expression) string {
	start := g.sqlKey(e, "start")
	if start != "" {
		start = "START WITH " + start
	}
	increment := g.sqlKey(e, "increment")
	if increment != "" {
		increment = " INCREMENT BY " + increment
	}
	minvalue := g.sqlKey(e, "minvalue")
	if minvalue != "" {
		minvalue = " MINVALUE " + minvalue
	}
	maxvalue := g.sqlKey(e, "maxvalue")
	if maxvalue != "" {
		maxvalue = " MAXVALUE " + maxvalue
	}
	owned := g.sqlKey(e, "owned")
	if owned != "" {
		owned = " OWNED BY " + owned
	}
	cache := ""
	switch v := e.Arg("cache").(type) {
	case nil:
	case bool:
		if v {
			cache = " CACHE"
		}
	default:
		cache = " CACHE " + g.gen(v)
	}
	options := g.expressions(exprsOptions{expression: e, key: "options", flat: true, sep: " "})
	if options != "" {
		options = " " + options
	}
	return strings.TrimLeft(start+increment+minvalue+maxvalue+cache+options+owned, " \t\n\r")
}

// getPutSQL ports get_put_sql (generator.py:6123-6135).
func (g *Generator) getPutSQL(e expressions.Expression) string {
	props := asExpression(e.Arg("properties"))
	propsSQL := ""
	if props != nil {
		propsSQL = g.renderProperties(props, " ", " ", "", false)
	}
	this := g.sqlKey(e, "this")
	target := g.sqlKey(e, "target")
	if e.Kind() == expressions.KindPut {
		return "PUT " + this + " " + target + propsSQL
	}
	return "GET " + target + " " + this + propsSQL
}

// semanticViewSQL ports semanticview_sql (generator.py:6169-6185).
func (g *Generator) semanticViewSQL(e expressions.Expression) string {
	this := g.seg(g.sqlKey(e, "this"), "")
	section := func(key, keyword string) string {
		sql := g.expressions(exprsOptions{expression: e, key: key, dynamic: true, skipFirst: true, skipLast: true})
		if sql == "" {
			return ""
		}
		return g.seg(keyword + " " + sql)
	}
	metrics := section("metrics", "METRICS")
	dimensions := section("dimensions", "DIMENSIONS")
	facts := section("facts", "FACTS")
	where := g.sqlKey(e, "where")
	if where != "" {
		where = g.seg("WHERE " + where)
	}
	body := g.indent(this+metrics+dimensions+facts+where, 0, nil, true, false)
	return "SEMANTIC_VIEW(" + body + g.seg(")", "")
}
