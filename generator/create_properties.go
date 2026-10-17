package generator

import (
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/expressions"
)

// propertyLocation mirrors exp.Properties.Location (expressions/properties.py:572-586).
// Only locations reached by the base, MySQL, and Postgres CREATE paths are needed here, but
// POST_INDEX remains explicit because createSQL preserves its upstream assembly position.
type propertyLocation uint8

const (
	propertyLocationUnsupported propertyLocation = iota
	propertyLocationPostCreate
	propertyLocationPostName
	propertyLocationPostSchema
	propertyLocationPostWith
	propertyLocationPostAlias
	propertyLocationPostExpression
	propertyLocationPostIndex
)

// propertyLocations is the worklist-scoped port of Generator.PROPERTIES_LOCATION
// (generator.py:672-787). PartitionedByProperty is dialect-dependent: Postgres uses
// PARTITION BY after the schema, while base uses PARTITIONED_BY= inside WITH (...).
var propertyLocations = map[expressions.Kind]propertyLocation{
	expressions.KindAlgorithmProperty:          propertyLocationPostCreate,
	expressions.KindDefinerProperty:            propertyLocationPostCreate,
	expressions.KindMaterializedProperty:       propertyLocationPostCreate,
	expressions.KindTemporaryProperty:          propertyLocationPostCreate,
	expressions.KindUnloggedProperty:           propertyLocationPostCreate,
	expressions.KindAutoIncrementProperty:      propertyLocationPostSchema,
	expressions.KindCalledOnNullInputProperty:  propertyLocationPostSchema,
	expressions.KindCharacterSetProperty:       propertyLocationPostSchema,
	expressions.KindClusterProperty:            propertyLocationPostSchema,
	expressions.KindEnviromentProperty:         propertyLocationPostSchema,
	expressions.KindHandlerProperty:            propertyLocationPostSchema,
	expressions.KindParameterStyleProperty:     propertyLocationPostSchema,
	expressions.KindStreamingTableProperty:     propertyLocationPostCreate,
	expressions.KindCollateProperty:            propertyLocationPostSchema,
	expressions.KindEngineProperty:             propertyLocationPostSchema,
	expressions.KindInheritsProperty:           propertyLocationPostSchema,
	expressions.KindLanguageProperty:           propertyLocationPostSchema,
	expressions.KindLikeProperty:               propertyLocationPostSchema,
	expressions.KindLockProperty:               propertyLocationPostSchema,
	expressions.KindPartitionByListProperty:    propertyLocationPostSchema,
	expressions.KindPartitionByRangeProperty:   propertyLocationPostSchema,
	expressions.KindPartitionedOfProperty:      propertyLocationPostSchema,
	expressions.KindReturnsProperty:            propertyLocationPostSchema,
	expressions.KindRowFormatDelimitedProperty: propertyLocationPostSchema,
	expressions.KindRowFormatProperty:          propertyLocationPostSchema,
	expressions.KindRowFormatSerdeProperty:     propertyLocationPostSchema,
	expressions.KindSchemaCommentProperty:      propertyLocationPostSchema,
	expressions.KindSerdeProperties:            propertyLocationPostSchema,
	expressions.KindSetConfigProperty:          propertyLocationPostSchema,
	expressions.KindSqlReadWriteProperty:       propertyLocationPostSchema,
	expressions.KindSqlSecurityProperty:        propertyLocationPostSchema,
	expressions.KindStabilityProperty:          propertyLocationPostSchema,
	expressions.KindStrictProperty:             propertyLocationPostSchema,
	expressions.KindFileFormatProperty:         propertyLocationPostWith,
	expressions.KindProperty:                   propertyLocationPostWith,
	expressions.KindLockingProperty:            propertyLocationPostAlias,
	expressions.KindNoPrimaryIndexProperty:     propertyLocationPostExpression,
	expressions.KindOnCommitProperty:           propertyLocationPostExpression,
	expressions.KindOnProperty:                 propertyLocationPostSchema,
	expressions.KindTriggerProperties:          propertyLocationPostExpression,
	expressions.KindWithDataProperty:           propertyLocationPostExpression,
	expressions.KindAutoRefreshProperty:        propertyLocationPostSchema,
	expressions.KindCluster:                    propertyLocationPostSchema,
	expressions.KindDictRange:                  propertyLocationPostSchema,
	expressions.KindDictProperty:               propertyLocationPostSchema,
	expressions.KindEmptyProperty:              propertyLocationPostSchema,
	expressions.KindOrder:                      propertyLocationPostSchema,
	expressions.KindPrimaryKey:                 propertyLocationPostSchema,
	expressions.KindSampleProperty:             propertyLocationPostSchema,
	expressions.KindSecureProperty:             propertyLocationPostCreate,
	expressions.KindSettingsProperty:           propertyLocationPostSchema,
	expressions.KindToTableProperty:            propertyLocationPostSchema,
	expressions.KindMergeTreeTTL:               propertyLocationPostSchema,
	expressions.KindVolatileProperty:           propertyLocationPostCreate,
}

// propertyNames is the worklist-scoped port of Properties.PROPERTY_TO_NAME. Kinds with
// dedicated renderers (CharacterSet/FileFormat/RowFormat and PartitionedBy on Postgres) do
// not need duplicate entries here.
var propertyNames = map[expressions.Kind]string{
	expressions.KindAlgorithmProperty:     "ALGORITHM",
	expressions.KindAutoIncrementProperty: "AUTO_INCREMENT",
	expressions.KindCollateProperty:       "COLLATE",
	expressions.KindDefinerProperty:       "DEFINER",
	expressions.KindEngineProperty:        "ENGINE",
	expressions.KindLockProperty:          "LOCK",
	expressions.KindSchemaCommentProperty: "COMMENT",
	expressions.KindPartitionedByProperty: "PARTITIONED_BY",
}

func init() {
	// withfill_sql (generator.py:3085-3101).
	dispatch[expressions.KindWithFill] = func(g *Generator, e expressions.Expression) string {
		out := "WITH FILL"
		if from := g.sqlKey(e, "from_"); from != "" {
			out += " FROM " + from
		}
		if to := g.sqlKey(e, "to"); to != "" {
			out += " TO " + to
		}
		if step := g.sqlKey(e, "step"); step != "" {
			out += " STEP " + step
		}
		var values []string
		for _, item := range listFromValue(e.Arg("interpolate")) {
			ie, _ := item.(expressions.Expression)
			if ie == nil {
				continue
			}
			if ie.Kind() == expressions.KindAlias {
				values = append(values, g.sqlKey(ie, "alias")+" AS "+g.sqlKey(ie, "this"))
			} else {
				values = append(values, g.sqlKey(ie, "this"))
			}
		}
		if len(values) > 0 {
			out += " INTERPOLATE (" + strings.Join(values, ", ") + ")"
		}
		return out
	}
	// skipjsoncolumn_sql (generator.py:5644-5646).
	dispatch[expressions.KindSkipJSONColumn] = func(g *Generator, e expressions.Expression) string {
		regexp := ""
		if truthy(e.Arg("regexp")) {
			regexp = " REGEXP"
		}
		return "SKIP" + regexp + " " + g.gen(e.Expr())
	}
	dispatch[expressions.KindAutoRefreshProperty] = func(g *Generator, e expressions.Expression) string {
		return "AUTO REFRESH " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindEmptyProperty] = func(*Generator, expressions.Expression) string { return "EMPTY" }
	dispatch[expressions.KindSampleProperty] = func(g *Generator, e expressions.Expression) string {
		return "SAMPLE BY " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindSecureProperty] = func(*Generator, expressions.Expression) string { return "SECURE" }
	dispatch[expressions.KindSettingsProperty] = func(g *Generator, e expressions.Expression) string {
		return "SETTINGS" + g.seg("") + g.expressions(exprsOptions{expression: e})
	}
	dispatch[expressions.KindToTableProperty] = func(g *Generator, e expressions.Expression) string {
		return "TO " + g.gen(e.This())
	}
	dispatch[expressions.KindVolatileProperty] = func(*Generator, expressions.Expression) string { return "VOLATILE" }
	// oncluster_sql (generator.py:4940-4941).
	dispatch[expressions.KindOnCluster] = func(*Generator, expressions.Expression) string { return "" }
	// uuidproperty_sql (generator.py:2120-2121).
	dispatch[expressions.KindUuidProperty] = func(g *Generator, e expressions.Expression) string {
		return "UUID " + g.sqlKey(e, "this")
	}
	// mergetreettlaction_sql / mergetreettl_sql (generator.py:4179-4199).
	dispatch[expressions.KindMergeTreeTTLAction] = func(g *Generator, e expressions.Expression) string {
		out := g.sqlKey(e, "this")
		if truthy(e.Arg("delete")) {
			out += " DELETE"
		}
		if recompress := g.sqlKey(e, "recompress"); recompress != "" {
			out += " RECOMPRESS " + recompress
		}
		if toDisk := g.sqlKey(e, "to_disk"); toDisk != "" {
			out += " TO DISK " + toDisk
		}
		if toVolume := g.sqlKey(e, "to_volume"); toVolume != "" {
			out += " TO VOLUME " + toVolume
		}
		return out
	}
	dispatch[expressions.KindMergeTreeTTL] = func(g *Generator, e expressions.Expression) string {
		where := g.sqlKey(e, "where")
		group := g.sqlKey(e, "group")
		aggregates := g.expressions(exprsOptions{expression: e, key: "aggregates"})
		if aggregates != "" {
			aggregates = g.seg("SET") + g.seg(aggregates)
		}
		if where == "" && group == "" && aggregates == "" && len(e.Expressions()) == 1 {
			return "TTL " + g.expressions(exprsOptions{expression: e, flat: true})
		}
		return "TTL" + g.seg(g.expressions(exprsOptions{expression: e})) + where + group + aggregates
	}
	// dictproperty_sql / dictrange_sql / dictsubproperty_sql (generator.py:4905-4919).
	dispatch[expressions.KindDictProperty] = func(g *Generator, e expressions.Expression) string {
		settings := g.expressions(exprsOptions{expression: e, key: "settings", sep: " "})
		args := "()"
		if settings != "" {
			args = "(" + g.sep("") + settings + g.seg(")", "")
		}
		return g.sqlKey(e, "this") + "(" + g.sqlKey(e, "kind") + args + ")"
	}
	dispatch[expressions.KindDictRange] = func(g *Generator, e expressions.Expression) string {
		return g.sqlKey(e, "this") + "(MIN " + g.sqlKey(e, "min") + " MAX " + g.sqlKey(e, "max") + ")"
	}
	dispatch[expressions.KindDictSubProperty] = func(g *Generator, e expressions.Expression) string {
		return g.sqlKey(e, "this") + " " + g.sqlKey(e, "value")
	}
	dispatch[expressions.KindProperty] = (*Generator).propertySQL
	dispatch[expressions.KindAlgorithmProperty] = (*Generator).propertySQL
	dispatch[expressions.KindAutoIncrementProperty] = (*Generator).propertySQL
	dispatch[expressions.KindCollateProperty] = (*Generator).propertySQL
	dispatch[expressions.KindDefinerProperty] = (*Generator).propertySQL
	dispatch[expressions.KindEngineProperty] = (*Generator).propertySQL
	dispatch[expressions.KindLockProperty] = (*Generator).propertySQL
	dispatch[expressions.KindSchemaCommentProperty] = (*Generator).propertySQL
	dispatch[expressions.KindTemporaryProperty] = (*Generator).temporaryPropertySQL
	dispatch[expressions.KindMaterializedProperty] = (*Generator).materializedPropertySQL
	dispatch[expressions.KindUnloggedProperty] = (*Generator).unloggedPropertySQL
	dispatch[expressions.KindInheritsProperty] = (*Generator).inheritsPropertySQL
	dispatch[expressions.KindLikeProperty] = (*Generator).likePropertySQL
	dispatch[expressions.KindNoPrimaryIndexProperty] = (*Generator).noPrimaryIndexPropertySQL
	dispatch[expressions.KindOnCommitProperty] = (*Generator).onCommitPropertySQL
	dispatch[expressions.KindOnProperty] = (*Generator).onPropertySQL
	dispatch[expressions.KindSqlReadWriteProperty] = (*Generator).sqlReadWritePropertySQL
	dispatch[expressions.KindLockingProperty] = (*Generator).lockingPropertySQL
	dispatch[expressions.KindPartitionedByProperty] = (*Generator).partitionedByPropertySQL
	dispatch[expressions.KindPartitionBoundSpec] = (*Generator).partitionBoundSpecSQL
	dispatch[expressions.KindPartitionedOfProperty] = (*Generator).partitionedOfPropertySQL
	dispatch[expressions.KindWithDataProperty] = (*Generator).withDataPropertySQL
	dispatch[expressions.KindClusterProperty] = (*Generator).clusterPropertySQL
	dispatch[expressions.KindEnviromentProperty] = func(g *Generator, e expressions.Expression) string {
		return "ENVIRONMENT (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
	}
	dispatch[expressions.KindHandlerProperty] = func(g *Generator, e expressions.Expression) string { return "HANDLER " + g.sqlKey(e, "this") }
	dispatch[expressions.KindParameterStyleProperty] = func(g *Generator, e expressions.Expression) string {
		return "PARAMETER STYLE " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindStreamingTableProperty] = func(*Generator, expressions.Expression) string { return "STREAMING" }
	// clone_sql (generator.py:1511-1515); SUPPORTS_TABLE_COPY is True for every ported dialect.
	dispatch[expressions.KindClone] = func(g *Generator, e expressions.Expression) string {
		shallow, keyword := "", "CLONE"
		if boolValue(e.Arg("shallow")) {
			shallow = "SHALLOW "
		}
		if boolValue(e.Arg("copy")) {
			keyword = "COPY"
		}
		return shallow + keyword + " " + g.sqlKey(e, "this")
	}
	// timeserieskey_sql (generator.py:3768-3770).
	dispatch[expressions.KindTimeseriesKey] = func(g *Generator, e expressions.Expression) string {
		g.unsupported("TIMESERIES primary key columns are not supported")
		return g.sqlKey(e, "this")
	}
	dispatch[expressions.KindStream] = func(g *Generator, e expressions.Expression) string { return "STREAM " + g.sqlKey(e, "this") }
}

// clusterPropertySQL ports clusterproperty_sql (generator.py:3106-3111).
func (g *Generator) clusterPropertySQL(e expressions.Expression) string {
	if e.Arg("this") != nil {
		g.unsupported("Unsupported CLUSTER BY " + g.sqlKey(e, "this"))
		return ""
	}
	return "CLUSTER BY (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}

func (g *Generator) propertyLocation(property expressions.Expression) (propertyLocation, bool) {
	if location, ok := g.lookupPropertyLocation(property.Kind()); ok {
		return location, true
	}
	if property.Kind() == expressions.KindPartitionedByProperty {
		switch g.dialect.Name {
		case "postgres":
			return propertyLocationPostSchema, true
		case "mysql":
			return propertyLocationUnsupported, true
		default:
			return propertyLocationPostWith, true
		}
	}

	// generators/mysql.py:630-640 moves SQL SECURITY from POST_SCHEMA to POST_CREATE for
	// views only; functions and procedures retain the base POST_SCHEMA placement.
	if property.Kind() == expressions.KindSqlSecurityProperty && g.dialect.Name == "mysql" {
		properties := property.Parent()
		if properties != nil {
			create := properties.Parent()
			if create != nil && create.Kind() == expressions.KindCreate && create.Text("kind") == "VIEW" {
				return propertyLocationPostCreate, true
			}
		}
	}

	location, ok := propertyLocations[property.Kind()]
	return location, ok
}

// locateProperties ports locate_properties (generator.py:2067-2076). Unknown properties are
// fail-closed: report them as unsupported and omit them instead of guessing a placement.
func (g *Generator) locateProperties(properties expressions.Expression) map[propertyLocation][]expressions.Expression {
	locations := map[propertyLocation][]expressions.Expression{}
	if properties == nil {
		return locations
	}
	for _, property := range properties.Expressions() {
		location, ok := g.propertyLocation(property)
		if !ok || location == propertyLocationUnsupported {
			g.unsupported("Unsupported property " + strings.ToLower(expressions.ClassName(property.Kind())))
			continue
		}
		locations[location] = append(locations[location], property)
	}
	return locations
}

func (g *Generator) propertiesExpression(items []expressions.Expression, parent expressions.Expression) expressions.Expression {
	properties := expressions.Properties(expressions.Args{"expressions": items})
	if parent != nil {
		properties.SetParent(parent, "properties", -1)
	}
	return properties
}

// rootPropertiesSQL ports root_properties (generator.py:2044-2047).
func (g *Generator) rootPropertiesSQL(properties expressions.Expression) string {
	if properties == nil || len(properties.Expressions()) == 0 {
		return ""
	}
	return g.expressions(exprsOptions{expression: properties, noIndent: true, sep: " "})
}

// renderProperties ports properties (generator.py:2049-2062).
func (g *Generator) renderProperties(properties expressions.Expression, prefix, sep, suffix string, wrapped bool) string {
	if properties == nil || len(properties.Expressions()) == 0 {
		return ""
	}
	sql := g.expressions(exprsOptions{expression: properties, noIndent: true, sep: sep})
	if sql == "" {
		return ""
	}
	if wrapped {
		sql = g.wrap(sql)
	}
	space := ""
	if strings.TrimSpace(prefix) != "" {
		space = " "
	}
	return prefix + space + sql + suffix
}

// withPropertiesSQL ports with_properties (generator.py:2064-2065). Base, MySQL, and
// Postgres all use the base WITH_PROPERTIES_PREFIX of "WITH".
func (g *Generator) withPropertiesSQL(properties expressions.Expression) string {
	if g.isDialect("snowflake") {
		// generators/snowflake.py:661-662.
		return g.renderProperties(properties, g.sep(""), " ", "", false)
	}
	return g.renderProperties(properties, g.seg("WITH", ""), ", ", "", true)
}

// nakedPropertySQL ports naked_property (generator.py:4787-4791).
func (g *Generator) nakedPropertySQL(e expressions.Expression) string {
	name, ok := propertyNames[e.Kind()]
	if !ok {
		g.unsupported("Unsupported property " + expressions.ClassName(e.Kind()))
	}
	return name + " " + g.sqlKey(e, "this")
}

// propertyName ports property_name (generator.py:2078-2081).
func (g *Generator) propertyName(e expressions.Expression, stringKey bool) string {
	if this := asExpression(e.Arg("this")); this != nil && this.Kind() == expressions.KindDot {
		return g.sqlKey(e, "this")
	}
	name := e.Name()
	if stringKey {
		return "'" + strings.ReplaceAll(name, "'", "''") + "'"
	}
	return name
}

// propertySQL ports property_sql (generator.py:2083-2092).
func (g *Generator) propertySQL(e expressions.Expression) string {
	if e.Kind() == expressions.KindProperty {
		return g.propertyName(e, false) + "=" + g.sqlKey(e, "value")
	}
	if e.Kind() == expressions.KindSchemaCommentProperty && g.dialect.Name == "postgres" {
		g.unsupported("Table comments are not supported in the CREATE statement")
		return ""
	}
	name, ok := propertyNames[e.Kind()]
	if !ok {
		g.unsupported("Unsupported property " + strings.ToLower(expressions.ClassName(e.Kind())))
		return ""
	}
	return name + "=" + g.sqlKey(e, "this")
}

func (g *Generator) temporaryPropertySQL(expressions.Expression) string    { return "TEMPORARY" }
func (g *Generator) materializedPropertySQL(expressions.Expression) string { return "MATERIALIZED" }
func (g *Generator) unloggedPropertySQL(expressions.Expression) string     { return "UNLOGGED" }

func (g *Generator) inheritsPropertySQL(e expressions.Expression) string {
	return "INHERITS (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}

// likePropertySQL ports likeproperty_sql (generator.py:2097-2112). All in-scope dialects
// support CREATE TABLE LIKE; Postgres requires a LIKE property outside Schema to be wrapped.
func (g *Generator) likePropertySQL(e expressions.Expression) string {
	options := make([]string, 0, len(e.Expressions()))
	for _, option := range e.Expressions() {
		optionSQL := strings.TrimSpace(option.Name() + " " + g.sqlKey(option, "value"))
		if optionSQL != "" {
			options = append(options, optionSQL)
		}
	}
	optionsSQL := ""
	if len(options) > 0 {
		optionsSQL = " " + strings.Join(options, " ")
	}
	like := "LIKE " + g.sqlKey(e, "this") + optionsSQL
	if g.dialect.Name == "postgres" && (e.Parent() == nil || e.Parent().Kind() != expressions.KindSchema) {
		like = "(" + like + ")"
	}
	return like
}

func (g *Generator) noPrimaryIndexPropertySQL(expressions.Expression) string {
	return "NO PRIMARY INDEX"
}

func (g *Generator) onCommitPropertySQL(e expressions.Expression) string {
	rows := "PRESERVE"
	if boolValue(e.Arg("delete")) {
		rows = "DELETE"
	}
	return "ON COMMIT " + rows + " ROWS"
}

// onPropertySQL ports OnProperty (generator.py:227): `ON <this>`, e.g. the `ON <table>` target of
// MySQL `DROP INDEX <idx> ON <table>` (carried in Drop.cluster) and the CREATE `ON <cluster>` property.
func (g *Generator) onPropertySQL(e expressions.Expression) string {
	return "ON " + g.sqlKey(e, "this")
}

func (g *Generator) sqlReadWritePropertySQL(e expressions.Expression) string { return e.Name() }

func (g *Generator) lockingPropertySQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	forOrIn := stringValue(e.Arg("for_or_in"))
	if forOrIn != "" {
		forOrIn = " " + forOrIn
	}
	override := ""
	if boolValue(e.Arg("override")) {
		override = " OVERRIDE"
	}
	return "LOCKING " + stringValue(e.Arg("kind")) + this + forOrIn + " " + stringValue(e.Arg("lock_type")) + override
}

func (g *Generator) partitionedByPropertySQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return "PARTITION BY " + g.sqlKey(e, "this")
	}
	return propertyNames[expressions.KindPartitionedByProperty] + "=" + g.sqlKey(e, "this")
}

func (g *Generator) partitionBoundSpecSQL(e expressions.Expression) string {
	switch e.Arg("this").(type) {
	case []expressions.Expression, []any:
		return "IN (" + g.expressions(exprsOptions{sqls: listFromValue(e.Arg("this")), flat: true}) + ")"
	}
	if this := g.sqlKey(e, "this"); this != "" {
		return "WITH (MODULUS " + this + ", REMAINDER " + g.sqlKey(e, "expression") + ")"
	}
	fromExpressions := g.expressions(exprsOptions{expression: e, key: "from_expressions", flat: true})
	toExpressions := g.expressions(exprsOptions{expression: e, key: "to_expressions", flat: true})
	return "FROM (" + fromExpressions + ") TO (" + toExpressions + ")"
}

func (g *Generator) partitionedOfPropertySQL(e expressions.Expression) string {
	bound := " DEFAULT"
	if expression := asExpression(e.Arg("expression")); expression != nil && expression.Kind() == expressions.KindPartitionBoundSpec {
		bound = " FOR VALUES " + g.gen(expression)
	}
	return "PARTITION OF " + g.sqlKey(e, "this") + bound
}

func (g *Generator) withDataPropertySQL(e expressions.Expression) string {
	no := ""
	if boolValue(e.Arg("no")) {
		no = "NO "
	}
	sql := "WITH " + no + "DATA"
	if statistics := e.Arg("statistics"); statistics != nil {
		statisticsNo := ""
		if !boolValue(statistics) {
			statisticsNo = "NO "
		}
		sql += " AND " + statisticsNo + "STATISTICS"
	}
	return sql
}

// Generator methods for nodes the ClickHouse parser builds (generator.py).
func init() {
	// assume (generator.py:147).
	dispatch[expressions.KindAssumeColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "ASSUME (" + g.sqlKey(e, "this") + ")"
	}
	// parameterizedagg_sql / anonymousaggfunc_sql / combinedaggfunc_sql /
	// combinedparameterizedagg_sql (generator.py:6102-6113).
	parameterized := func(g *Generator, e expressions.Expression) string {
		params := g.expressions(exprsOptions{expression: e, key: "params", flat: true})
		return g.funcCall(e.Name(), anySlice(e.Expressions()), "(", ")", true) + "(" + params + ")"
	}
	anonymousAgg := func(g *Generator, e expressions.Expression) string {
		return g.funcCall(e.Name(), anySlice(e.Expressions()), "(", ")", true)
	}
	dispatch[expressions.KindParameterizedAgg] = parameterized
	dispatch[expressions.KindCombinedParameterizedAgg] = parameterized
	dispatch[expressions.KindAnonymousAggFunc] = anonymousAgg
	dispatch[expressions.KindCombinedAggFunc] = anonymousAgg
	// apply_sql (generator.py:5763-5767).
	dispatch[expressions.KindApply] = func(g *Generator, e expressions.Expression) string {
		return g.sqlKey(e, "this") + " APPLY(" + g.sqlKey(e, "expression") + ")"
	}
	// columns_sql (generator.py:5827-5832).
	dispatch[expressions.KindColumns] = func(g *Generator, e expressions.Expression) string {
		fn := g.functionFallbackSQL(e)
		if truthy(e.Arg("unpack")) {
			fn = "*" + fn
		}
		return fn
	}
	// detach_sql (generator.py:5909-5924).
	dispatch[expressions.KindDetach] = func(g *Generator, e expressions.Expression) string {
		kind := g.sqlKey(e, "kind")
		if kind != "" {
			kind = " " + kind
		}
		exists := ""
		if truthy(e.Arg("exists")) {
			exists = " IF EXISTS"
			if kind == "" {
				kind = " DATABASE"
			}
		}
		this := g.sqlKey(e, "this")
		if this != "" {
			this = " " + this
		}
		cluster := g.sqlKey(e, "cluster")
		if cluster != "" {
			cluster = " " + cluster
		}
		permanent, sync := "", ""
		if truthy(e.Arg("permanent")) {
			permanent = " PERMANENTLY"
		}
		if truthy(e.Arg("sync")) {
			sync = " SYNC"
		}
		return "DETACH" + kind + exists + this + cluster + permanent + sync
	}
	// altermodifysqlsecurity_sql (generator.py:6350-6352).
	dispatch[expressions.KindAlterModifySqlSecurity] = func(g *Generator, e expressions.Expression) string {
		return "MODIFY " + g.expressions(exprsOptions{expression: e, sep: " "})
	}
}

func anySlice(list []expressions.Expression) []any {
	out := make([]any, len(list))
	for i, e := range list {
		out[i] = e
	}
	return out
}

func init() {
	// generator.py:188-190.
	dispatch[expressions.KindEphemeralColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		if this := g.sqlKey(e, "this"); this != "" {
			return "EPHEMERAL " + this
		}
		return "EPHEMERAL"
	}
	// generator.py:284-286.
	dispatch[expressions.KindUtcTimestamp] = func(g *Generator, e expressions.Expression) string {
		return g.gen(expressions.New(expressions.KindCurrentTimestamp, expressions.Args{"this": expressions.LiteralString("UTC")}))
	}
}

func init() {
	// tsordstotimestamp_sql (generator.py:5188-5193); the is_type(TIMESTAMP) test is reduced
	// to an explicit TIMESTAMP cast, since annotate_types never runs here.
	dispatch[expressions.KindTsOrDsToTimestamp] = func(g *Generator, e expressions.Expression) string {
		this := e.This()
		if this != nil && (this.Kind() == expressions.KindTsOrDsToTimestamp ||
			(this.Kind() == expressions.KindCast && expressions.IsType(asExpression(this.Arg("to")), expressions.DTypeTimestamp))) {
			return g.gen(this)
		}
		return g.gen(expressions.Cast(expressions.Args{"this": this, "to": expressions.DTypeTimestamp.IntoExpr(nil)}))
	}
}

func init() {
	// converttimezone_sql (generator.py:5600-5615).
	dispatch[expressions.KindConvertTimezone] = func(g *Generator, e expressions.Expression) string {
		if g.dialect.SupportsConvertTimezone {
			return g.functionFallbackSQL(e)
		}
		sourceTZ, targetTZ := asExpression(e.Arg("source_tz")), asExpression(e.Arg("target_tz"))
		timestamp := asExpression(e.Arg("timestamp"))
		if sourceTZ != nil && timestamp != nil {
			timestamp = expressions.New(expressions.KindAtTimeZone, expressions.Args{"this": expressions.CastTo(timestamp, expressions.DTypeTimestampNtz), "zone": sourceTZ})
		}
		return g.gen(expressions.New(expressions.KindAtTimeZone, expressions.Args{"this": timestamp, "zone": targetTZ}))
	}
	// lastday_sql (generator.py:5232-5240).
	dispatch[expressions.KindLastDay] = func(g *Generator, e expressions.Expression) string {
		if g.dialect.LastDaySupportsDatePart {
			return g.functionFallbackSQL(e)
		}
		if unit := asExpression(e.Arg("unit")); unit != nil && strings.ToUpper(unit.Name()) != "MONTH" {
			g.unsupported("Date parts are not supported in LAST_DAY.")
		}
		return g.funcCall("LAST_DAY", []any{e.This()}, "(", ")", true)
	}
	// tonumber_sql (generator.py:4893-4903).
	dispatch[expressions.KindToNumber] = func(g *Generator, e expressions.Expression) string {
		toDouble := func() string {
			return g.gen(expressions.Cast(expressions.Args{"this": e.This(), "to": expressions.DTypeDouble.IntoExpr(nil)}))
		}
		if !g.dialect.SupportsToNumber {
			g.unsupported("Unsupported TO_NUMBER function")
			return toDouble()
		}
		format := e.Arg("format")
		if format == nil {
			g.unsupported("Conversion format is required for TO_NUMBER")
			return toDouble()
		}
		return g.funcCall("TO_NUMBER", []any{e.This(), format}, "(", ")", true)
	}
}
