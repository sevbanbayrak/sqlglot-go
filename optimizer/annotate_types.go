package optimizer

import (
	"fmt"
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/dialects"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/schema"
)

// EXTRACT/DATE_PART specifiers that return BIGINT instead of INT
var bigintExtractDateParts = map[string]bool{
	"EPOCH_SECOND":      true,
	"EPOCH_MILLISECOND": true,
	"EPOCH_MICROSECOND": true,
	"EPOCH_NANOSECOND":  true,
	"NANOSECOND":        true,
}

// A type value is what upstream types as `exp.DataType | exp.DType`: nil, an exp.DType or a
// DataType expression. The distinction is observable (_maybe_coerce, _annotate_map), so the port
// keeps it instead of always building a DataType.

// BinaryCoercionFunc takes both sides of a binary operation and returns the resulting type.
type BinaryCoercionFunc func(l, r exp.Expression) any

// BinaryCoercions maps a (left, right) type pair to its coercion function.
type BinaryCoercions map[[2]exp.DType]BinaryCoercionFunc

// CoercesTo maps a type to the set of types it can be coerced into.
type CoercesTo map[exp.DType]map[exp.DType]bool

// AnnotateTypesOpts holds annotate_types' keyword arguments (annotate_types.py:46-53).
type AnnotateTypesOpts struct {
	// Schema is the database schema (dict | Schema | nil).
	Schema schema.SchemaType
	// ExpressionMetadata maps an expression Kind to its annotation spec.
	ExpressionMetadata ExprMetadata
	// CoercesTo maps a type to the set of types it can be coerced into.
	CoercesTo CoercesTo
	// Dialect is consulted when constructing a Schema object, if needed.
	Dialect dialects.DialectType
	// OverwriteTypes re-annotates the existing AST types; nil means true.
	OverwriteTypes *bool
}

// AnnotateTypes infers the types of an expression, annotating its AST accordingly
// (annotate_types.py:46-89).
func AnnotateTypes(expression exp.Expression, opts AnnotateTypesOpts) exp.Expression {
	s, err := schema.EnsureSchema(opts.Schema, opts.Dialect, true)
	if err != nil {
		panic(err)
	}
	overwrite := opts.OverwriteTypes == nil || *opts.OverwriteTypes
	return newTypeAnnotator(s, opts.ExpressionMetadata, opts.CoercesTo, nil, overwrite).Annotate(expression, true)
}

func init() {
	exp.AnnotateTypesFunc = func(e exp.Expression, dialect exp.DialectType) exp.Expression {
		return AnnotateTypes(e, AnnotateTypesOpts{Dialect: dialect})
	}
}

func coerceDateLiteral(l exp.Expression, unit exp.Expression) exp.DType {
	dateText := l.Name()
	isISODate_ := isISODate(dateText)

	if isISODate_ && dialects.IsDateUnit(unit) {
		return exp.DTypeDate
	}

	// An ISO date is also an ISO datetime, but not vice versa
	if isISODate_ || isISODatetime(dateText) {
		return exp.DTypeDatetime
	}

	return exp.DTypeUnknown
}

func coerceDate(l exp.Expression, unit exp.Expression) any {
	if !dialects.IsDateUnit(unit) {
		return exp.DTypeDatetime
	}
	if t := l.Type(); t != nil {
		return t.Arg("this")
	}
	return exp.DTypeUnknown
}

func swapArgs(f BinaryCoercionFunc) BinaryCoercionFunc {
	return func(l, r exp.Expression) any { return f(r, l) }
}

func swapAll(coercions BinaryCoercions) BinaryCoercions {
	out := BinaryCoercions{}
	for k, f := range coercions {
		out[k] = f
	}
	for k, f := range coercions {
		out[[2]exp.DType{k[1], k[0]}] = swapArgs(f)
	}
	return out
}

func buildCoercesTo() CoercesTo {
	// Highest-to-lowest type precedence, as specified in Spark's docs (ANSI):
	// https://spark.apache.org/docs/3.2.0/sql-ref-ansi-compliance.html
	textPrecedence := []exp.DType{exp.DTypeText, exp.DTypeNVarchar, exp.DTypeVarchar, exp.DTypeNChar, exp.DTypeChar}
	numericPrecedence := []exp.DType{
		exp.DTypeDecFloat, exp.DTypeDouble, exp.DTypeFloat, exp.DTypeBigDecimal, exp.DTypeDecimal,
		exp.DTypeBigInt, exp.DTypeInt, exp.DTypeSmallInt, exp.DTypeTinyInt,
	}
	timelikePrecedence := []exp.DType{
		exp.DTypeTimestampLtz, exp.DTypeTimestampTz, exp.DTypeTimestamp, exp.DTypeDatetime, exp.DTypeDate,
	}

	result := CoercesTo{}
	for _, typePrecedence := range [][]exp.DType{textPrecedence, numericPrecedence, timelikePrecedence} {
		coercesTo := map[exp.DType]bool{}
		for _, dataType := range typePrecedence {
			result[dataType] = copyTypeSet(coercesTo)
			coercesTo[dataType] = true
		}
	}
	return result
}

func copyTypeSet(s map[exp.DType]bool) map[exp.DType]bool {
	out := make(map[exp.DType]bool, len(s))
	for k := range s {
		out[k] = true
	}
	return out
}

// copyCoercesTo is the defaultdict(set, deepcopy(TypeAnnotator.COERCES_TO)) the Hive and
// Databricks dialects start from.
func copyCoercesTo(c CoercesTo) CoercesTo {
	out := make(CoercesTo, len(c))
	for k, v := range c {
		out[k] = copyTypeSet(v)
	}
	return out
}

func (c CoercesTo) add(t exp.DType, targets map[exp.DType]bool) {
	if c[t] == nil {
		c[t] = map[exp.DType]bool{}
	}
	for k := range targets {
		c[t][k] = true
	}
}

// TypeAnnotatorCoercesTo ports TypeAnnotator.COERCES_TO.
var TypeAnnotatorCoercesTo = buildCoercesTo()

// TypeAnnotatorBinaryCoercions ports TypeAnnotator.BINARY_COERCIONS.
var TypeAnnotatorBinaryCoercions = buildBinaryCoercions()

func buildBinaryCoercions() BinaryCoercions {
	out := BinaryCoercions{}
	textInterval := BinaryCoercions{}
	for t := range exp.TextTypes {
		textInterval[[2]exp.DType{t, exp.DTypeInterval}] = func(l, r exp.Expression) any {
			return coerceDateLiteral(l, asExpression(r.Arg("unit")))
		}
	}
	textNumeric := BinaryCoercions{}
	for text := range exp.TextTypes {
		for numeric := range exp.NumericTypes {
			// text + numeric will yield the numeric type to match most dialects' semantics
			textNumeric[[2]exp.DType{text, numeric}] = func(l, r exp.Expression) any {
				if t := l.Type(); t != nil && exp.NumericTypes[dtypeOf(t)] {
					return t
				}
				return typeOrNil(r.Type())
			}
		}
	}
	dateInterval := BinaryCoercions{
		{exp.DTypeDate, exp.DTypeInterval}: func(l, r exp.Expression) any {
			return coerceDate(l, asExpression(r.Arg("unit")))
		},
	}
	for _, group := range []BinaryCoercions{swapAll(textInterval), swapAll(textNumeric), swapAll(dateInterval)} {
		for k, f := range group {
			out[k] = f
		}
	}
	return out
}

type scopeSourceKey struct {
	scope *Scope
	name  string
}

// typeMap is an insertion-ordered column name -> type value map (upstream's dict).
type typeMap struct {
	keys []string
	m    map[string]any
}

func newTypeMap() *typeMap { return &typeMap{m: map[string]any{}} }

func (t *typeMap) set(k string, v any) {
	if _, ok := t.m[k]; !ok {
		t.keys = append(t.keys, k)
	}
	t.m[k] = v
}

func (t *typeMap) get(k string) any {
	if t == nil {
		return nil
	}
	return t.m[k]
}

func (t *typeMap) has(k string) bool {
	if t == nil {
		return false
	}
	_, ok := t.m[k]
	return ok
}

func (t *typeMap) pop(k string) (any, bool) {
	v, ok := t.m[k]
	if !ok {
		return nil, false
	}
	delete(t.m, k)
	for i, key := range t.keys {
		if key == k {
			t.keys = append(t.keys[:i:i], t.keys[i+1:]...)
			break
		}
	}
	return v, true
}

type TypeAnnotator struct {
	schema             schema.Schema
	dialect            *dialects.Dialect
	expressionMetadata ExprMetadata
	coercesTo          CoercesTo
	binaryCoercions    BinaryCoercions

	// Caches the ids of annotated sub-Exprs, to ensure we only visit them once
	visited map[exp.Expression]bool

	// Caches NULL-annotated expressions to set them to UNKNOWN after type inference is completed
	nullExpressions map[exp.Expression]bool

	// Databricks and Spark ≥v3 actually support NULL (i.e., VOID) as a type
	supportsNullType bool

	// Maps an exp.SetOperation's id (e.g. UNION) to its projection types. This is computed if the
	// exp.SetOperation is the expression of a scope source, as selecting from it multiple times
	// would reprocess the entire subtree to coerce the types of its operands' projections
	setopColumnTypes map[exp.Expression]*typeMap

	// When set to False, this enables partial annotation by skipping already-annotated nodes
	overwriteTypes bool

	// Maps (Scope, source_name) to its column projections and types
	scopeSourceSelects map[scopeSourceKey]*typeMap
}

// NewTypeAnnotator is TypeAnnotator(schema) with the default tables.
func NewTypeAnnotator(s schema.Schema) *TypeAnnotator {
	return newTypeAnnotator(s, nil, nil, nil, true)
}

func newTypeAnnotator(s schema.Schema, expressionMetadata ExprMetadata, coercesTo CoercesTo, binaryCoercions BinaryCoercions, overwriteTypes bool) *TypeAnnotator {
	var d *dialects.Dialect
	if s != nil {
		d = s.Dialect()
	}
	if d == nil {
		d = dialects.Base()
	}
	typing := typingFor(d)
	if expressionMetadata == nil {
		expressionMetadata = typing.metadata
	}
	if len(coercesTo) == 0 {
		coercesTo = typing.coercesTo
	}
	if len(coercesTo) == 0 {
		coercesTo = TypeAnnotatorCoercesTo
	}
	if binaryCoercions == nil {
		binaryCoercions = TypeAnnotatorBinaryCoercions
	}
	return &TypeAnnotator{
		schema:             s,
		dialect:            d,
		expressionMetadata: expressionMetadata,
		coercesTo:          coercesTo,
		binaryCoercions:    binaryCoercions,
		visited:            map[exp.Expression]bool{},
		nullExpressions:    map[exp.Expression]bool{},
		supportsNullType:   d.SupportsNullType,
		setopColumnTypes:   map[exp.Expression]*typeMap{},
		overwriteTypes:     overwriteTypes,
		scopeSourceSelects: map[scopeSourceKey]*typeMap{},
	}
}

// Schema returns the annotator's schema.
func (a *TypeAnnotator) Schema() schema.Schema { return a.schema }

// Dialect returns the annotator's dialect.
func (a *TypeAnnotator) Dialect() *dialects.Dialect { return a.dialect }

func (a *TypeAnnotator) Clear() {
	a.visited = map[exp.Expression]bool{}
	a.nullExpressions = map[exp.Expression]bool{}
	a.setopColumnTypes = map[exp.Expression]*typeMap{}
	a.scopeSourceSelects = map[scopeSourceKey]*typeMap{}
}

// Uncache evicts `expression` (or its subtree, if `deep`) from the annotation caches. This must
// be called when an already-annotated tree is about to be mutated, so that a subsequent
// annotation pass doesn't skip it.
func (a *TypeAnnotator) Uncache(expression exp.Expression, deep bool) {
	nodes := []exp.Expression{expression}
	if deep {
		nodes = expression.Walk()
	}
	for _, node := range nodes {
		delete(a.visited, node)
		delete(a.nullExpressions, node)
		delete(a.setopColumnTypes, node)
	}
}

// SetType ports _set_type (annotate_types.py:255-276).
func (a *TypeAnnotator) SetType(expression exp.Expression, targetType any) exp.Expression {
	prevType := expression.Type()

	var dtype exp.Expression
	switch v := targetType.(type) {
	case exp.Expression:
		dtype = v
	case exp.DType:
		if v != "" {
			dtype = v.IntoExpr(nil)
		}
	}
	if dtype == nil {
		dtype = exp.DTypeUnknown.IntoExpr(nil)
	}
	expression.SetType(dtype)
	a.visited[expression] = true

	if !a.supportsNullType && expression.Type().Arg("this") == exp.DTypeNull {
		a.nullExpressions[expression] = true
	} else if prevType != nil && prevType.Arg("this") == exp.DTypeNull {
		delete(a.nullExpressions, expression)
	}

	return expression
}

// Annotate ports TypeAnnotator.annotate (annotate_types.py:278-296).
func (a *TypeAnnotator) Annotate(expression exp.Expression, annotateScope bool) exp.Expression {
	// This flag is used to avoid costly scope traversals when we only care about annotating
	// non-column expressions (partial type inference), e.g., when simplifying in the optimizer
	if annotateScope {
		for _, scope := range traverseScopeForOptimizer(expression) {
			a.AnnotateScope(scope)
		}
	}

	// This takes care of non-traversable expressions
	a.annotateExpression(expression, nil)

	// Replace NULL type with the default type of the targeted dialect, since the former is not an actual type;
	// it is mostly used to aid type coercion, e.g. in query set operations.
	nulls := make([]exp.Expression, 0, len(a.nullExpressions))
	for e := range a.nullExpressions {
		nulls = append(nulls, e)
	}
	for _, e := range nulls {
		a.SetType(e, a.dialect.DefaultNullType)
	}

	return expression
}

func (a *TypeAnnotator) getScopeSourceSelects(scope *Scope, sourceName string) *typeMap {
	key := scopeSourceKey{scope, sourceName}
	selects := a.scopeSourceSelects[key]

	if selects == nil {
		selects = newTypeMap()
		source := scope.Sources[sourceName]

		if sourceScope, ok := source.(*Scope); ok {
			selects = a.getSourceScopeSelects(sourceScope)
		} else {
			var pivots []exp.Expression
			if t, ok := source.(exp.Expression); ok && t != nil && t.Kind() == exp.KindTable {
				pivots = expressionsFor(t, "pivots")
			} else {
				pivots = scope.Pivots()
			}
			// Only the last operator in a chain carries the alias that names the
			// resulting source, so match on it and fold the whole chain in order
			if len(pivots) > 0 && pivots[len(pivots)-1].AliasOrName() == sourceName {
				parent := pivots[len(pivots)-1].Parent()
				var parentSource any
				if parent != nil {
					parentSource = scope.Sources[parent.AliasOrName()]
				}

				if _, isScope := parentSource.(*Scope); !isScope && parent != nil && parent.Kind() == exp.KindTable && parent.SchemaName() == "" {
					// A chain aliased like the CTE it reads from shadows it in
					// `scope.sources`, so reach for the CTE's scope directly
					parentSource = scope.CTESources[parent.Name()]
				}

				var srcTypes *typeMap
				if parentScope, ok := parentSource.(*Scope); ok {
					srcTypes = a.getSourceScopeSelects(parentScope)
				} else if ms, ok := a.schema.(*schema.MappingSchema); ok && parent != nil && parent.Kind() == exp.KindTable {
					srcTypes = newTypeMap()
					if found, err := ms.Find(parent, false, true); err == nil && found != nil {
						for _, k := range found.Keys() {
							v, _ := found.Get(k)
							srcTypes.set(k, v)
						}
					} else if err != nil {
						panic(err)
					}
				} else {
					srcTypes = newTypeMap()
				}

				for _, pivot := range pivots {
					if truthy(pivot.Arg("unpivot")) {
						srcTypes = a.getUnpivotColumnTypes(pivot, srcTypes)
					} else {
						srcTypes = a.getPivotColumnTypes(pivot, srcTypes)
					}
				}

				selects = srcTypes
			}
		}

		a.scopeSourceSelects[key] = selects
	}

	return selects
}

func isExplodeKind(e exp.Expression) bool {
	if e == nil {
		return false
	}
	switch e.Kind() {
	case exp.KindExplode, exp.KindPosexplode, exp.KindPosexplodeOuter, exp.Kind_ExplodeOuter:
		return true
	}
	return false
}

// isSelectable reports isinstance(expression, exp.Selectable).
func isSelectable(e exp.Expression) bool {
	if e.Is(exp.TraitQuery) || e.Is(exp.TraitUDTF) || e.Is(exp.TraitDerivedTable) || e.Is(exp.TraitDDL) {
		return true
	}
	switch e.Kind() {
	case exp.KindTable, exp.KindCTE, exp.KindInsert:
		return true
	}
	return false
}

func (a *TypeAnnotator) getSourceScopeSelects(source *Scope) *typeMap {
	expression := source.Expression
	out := newTypeMap()

	if expression.Is(exp.TraitUDTF) {
		var values []exp.Expression

		switch {
		case expression.Kind() == exp.KindLateral:
			if this := expression.This(); isExplodeKind(this) {
				values = []exp.Expression{this.This()}
			}
		case expression.Kind() == exp.KindUnnest:
			values = []exp.Expression{expression}
		case expression.Kind() != exp.KindTableFromRows:
			if first, ok := seqGet(expression.Expressions(), 0); ok {
				values = first.Expressions()
			}
		}

		if len(values) == 0 {
			return out
		}

		aliasColumnNames := expression.AliasColumnNames()

		var expType exp.Expression
		if expression.Kind() == exp.KindUnnest {
			expType = expression.Type()
		} else if expression.Kind() == exp.KindLateral && isExplodeKind(expression.This()) {
			expType = expression.This().Type()
		}

		if expType != nil && expType.IsType(exp.DTypeStruct) {
			for _, colDef := range expType.Expressions() {
				if colDef.Kind() == exp.KindColumnDef {
					if kind := asExpression(colDef.Arg("kind")); kind != nil {
						out.set(colDef.Name(), kind)
					}
				}
			}
			return out
		}

		for i := 0; i < len(aliasColumnNames) && i < len(values); i++ {
			out.set(aliasColumnNames[i], typeOrNil(values[i].Type()))
		}
		return out
	}

	if expression.Is(exp.TraitSetOperation) {
		if l, r := expression.This(), expression.Expr(); truthy(expression.Arg("by_name")) || (l != nil && r != nil && len(l.Selects()) == len(r.Selects())) {
			return a.getSetopColumnTypes(expression)
		}
	}

	if isSelectable(expression) {
		for _, s := range expression.Selects() {
			if t := s.Type(); t != nil {
				out.set(s.AliasOrName(), t)
			}
		}
		return out
	}

	return out
}

// AnnotateScope ports annotate_scope (annotate_types.py:379-433).
func (a *TypeAnnotator) AnnotateScope(scope *Scope) {
	if ms, ok := a.schema.(*schema.MappingSchema); ok {
		for _, tableColumn := range scope.TableColumns() {
			source := scope.Sources[tableColumn.Name()]

			if table, ok := source.(exp.Expression); ok && table != nil && table.Kind() == exp.KindTable {
				found, err := ms.Find(table, false, true)
				if err != nil {
					panic(err)
				}
				if found == nil {
					continue
				}

				var columns []exp.Expression
				for _, c := range found.Keys() {
					kind, _ := found.Get(c)
					columns = append(columns, exp.New(exp.KindColumnDef, exp.Args{"this": exp.ToIdentifier(c), "kind": kind}))
				}
				structType := exp.DataType(exp.Args{"this": exp.DTypeStruct, "expressions": columns, "nested": true})
				a.SetType(tableColumn, structType)
			} else if sourceScope, ok := source.(*Scope); ok && sourceScope.Expression.Is(exp.TraitQuery) {
				queryType, _ := sourceScope.Expression.MetaGet("query_type").(exp.Expression)
				if queryType == nil {
					queryType = exp.DTypeUnknown.IntoExpr(nil)
				}
				if queryType.IsType(exp.DTypeStruct) {
					a.SetType(tableColumn, sourceScope.Expression.Meta()["query_type"])
				}
			}
		}
	}

	// Iterate through all the expressions of the current scope in post-order, and annotate
	a.annotateExpression(scope.Expression, scope)
	a.fixupOrderByAliases(scope)

	if a.dialect.QueryResultsAreStructs && scope.Expression.Is(exp.TraitQuery) {
		var columns []exp.Expression
		for _, sel := range scope.Expression.Selects() {
			var kind any
			if t := sel.Type(); t != nil {
				kind = t.Copy()
			}
			columns = append(columns, exp.New(exp.KindColumnDef, exp.Args{"this": exp.ToIdentifier(sel.OutputName()), "kind": kind}))
		}
		structType := exp.DataType(exp.Args{"this": exp.DTypeStruct, "expressions": columns, "nested": true})

		anyUnknown := false
		for _, cd := range structType.Expressions() {
			if kind := asExpression(cd.Arg("kind")); kind != nil && kind.IsType(exp.DTypeUnknown) {
				anyUnknown = true
				break
			}
		}
		if !anyUnknown {
			// We don't use `_set_type` on purpose here. If we annotated the query directly, then
			// using it in other contexts (e.g., ARRAY(<query>)) could result in incorrect type
			// annotations, i.e., it shouldn't be interpreted as a STRUCT value.
			scope.Expression.Meta()["query_type"] = structType
		}
	}
}

type annotateItem struct {
	expr              exp.Expression
	childrenAnnotated bool
}

func (a *TypeAnnotator) annotateExpression(expression exp.Expression, scope *Scope) {
	stack := []annotateItem{{expression, false}}

	for len(stack) > 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		expr := item.expr

		if a.visited[expr] || (!a.overwriteTypes && expr.Type() != nil && !expr.IsType(exp.DTypeUnknown)) {
			continue // We've already inferred the expression's type
		}

		if !item.childrenAnnotated {
			stack = append(stack, annotateItem{expr, true})
			for _, child := range exp.IterExpressions(expr) {
				stack = append(stack, annotateItem{child, false})
			}
			continue
		}

		if scope != nil && expr.Kind() == exp.KindColumn && expr.TableName() != "" {
			a.annotateColumn(expr, scope)
			continue
		}

		spec, ok := a.expressionMetadata[expr.Kind()]
		switch {
		case ok && spec.Annotator != nil:
			spec.Annotator(a, expr)
		case ok && spec.Returns != nil:
			a.SetType(expr, spec.Returns)
		default:
			a.SetType(expr, exp.DTypeUnknown)
		}
	}
}

// annotateColumn is the qualified-column branch of _annotate_expression (annotate_types.py:470-541).
func (a *TypeAnnotator) annotateColumn(expr exp.Expression, scope *Scope) {
	table := expr.TableName()
	var source any
	sourceScope := scope
	for sourceScope != nil && source == nil {
		if s, ok := sourceScope.Sources[table]; ok && s != nil {
			source = s
		}
		if source == nil {
			sourceScope = sourceScope.Parent
		}
	}

	sourceTable, isTable := source.(exp.Expression)
	isTable = isTable && sourceTable != nil && sourceTable.Kind() == exp.KindTable

	switch {
	case isTable:
		tableColType, err := a.schema.GetColumnType(sourceTable, expr, nil, nil)
		if err != nil {
			panic(err)
		}
		var colType any = tableColType
		if tableColType != nil && tableColType.IsType(exp.DTypeUnknown) && len(expressionsFor(sourceTable, "pivots")) > 0 {
			s := sourceScope
			if s == nil {
				s = scope
			}
			colType = a.getScopeSourceSelects(s, table).get(expr.Name())
			if colType == nil {
				colType = exp.DTypeUnknown
			}
		}
		a.SetType(expr, colType)
	case source != nil && sourceScope != nil:
		if colType := a.getScopeSourceSelects(sourceScope, table).get(expr.Name()); colType != nil {
			a.SetType(expr, colType)
		} else if se := sourceExpression(source); se != nil && se.Kind() == exp.KindUnnest {
			a.SetType(expr, se.Type())
		} else {
			a.SetType(expr, exp.DTypeUnknown)
		}
	case source == nil && len(scope.Pivots()) > 0 && a.getScopeSourceSelects(scope, table).get(expr.Name()) != nil:
		a.SetType(expr, a.getScopeSourceSelects(scope, table).get(expr.Name()))
	default:
		a.SetType(expr, exp.DTypeUnknown)
	}

	if dotParts, _ := expr.MetaGet("dot_parts").([]string); expr.IsType(exp.DTypeJSON) && len(dotParts) > 0 {
		// JSON dot access is case sensitive across all dialects, so we need to undo the normalization.
		i := 0
		parent := expr.Parent()
		for parent != nil && parent.Kind() == exp.KindDot && i < len(dotParts) {
			identifier := asExpression(parent.Arg("expression"))
			if identifier != nil && identifier.Kind() == exp.KindIdentifier {
				// Rename in place to preserve the identifier's meta, e.g. token positions
				identifier.Set("this", dotParts[i])
				identifier.Set("quoted", true)
			} else if identifier != nil {
				identifier.Replace(exp.ToIdentifier(dotParts[i], true))
			}
			i++
			parent = parent.Parent()
		}

		delete(expr.Meta(), "dot_parts")
	}

	if t := expr.Type(); t != nil {
		if nullable, ok := t.Arg("nullable").(bool); ok && !nullable {
			expr.Meta()["nonnull"] = true
		}
	}
}

// sourceExpression is `source.expression` for a scope source (a Scope or an expression).
func sourceExpression(source any) exp.Expression {
	switch s := source.(type) {
	case *Scope:
		return s.Expression
	case exp.Expression:
		if s != nil {
			return s.Expr()
		}
	}
	return nil
}

func (a *TypeAnnotator) fixupOrderByAliases(scope *Scope) {
	query := scope.Expression
	if query == nil || !query.Is(exp.TraitQuery) {
		return
	}

	order := asExpression(query.Arg("order"))
	if order == nil {
		return
	}

	// Build alias -> type map from fully-annotated projections (last match wins,
	// consistent with how _expand_alias_refs handles duplicate aliases).
	aliasTypes := map[string]exp.Expression{}
	for _, sel := range query.Selects() {
		if sel.Kind() == exp.KindAlias || sel.Kind() == exp.KindPivotAlias {
			if this := sel.This(); this != nil && this.Type() != nil && !this.IsType(exp.DTypeUnknown) {
				aliasTypes[sel.Alias()] = this.Type()
			}
		}
	}

	if len(aliasTypes) == 0 {
		return
	}

	for _, ordered := range order.Expressions() {
		var aliasCols []exp.Expression
		for _, c := range ordered.FindAll(exp.KindColumn) {
			if c.TableName() == "" {
				if _, ok := aliasTypes[c.Name()]; ok {
					aliasCols = append(aliasCols, c)
				}
			}
		}
		for _, col := range aliasCols {
			a.SetType(col, aliasTypes[col.Name()])
		}

		if len(aliasCols) > 0 {
			for _, node := range ordered.WalkWithPrune(true, func(n exp.Expression) bool { return n.Kind() == exp.KindSubquery }) {
				if node.Kind() != exp.KindColumn && node.Kind() != exp.KindLiteral {
					delete(a.visited, node)
				}
			}
			a.annotateExpression(ordered, scope)
		}
	}
}

// typeValue splits a type value into its DataType (nil for a bare DType) and its `this`.
func typeValue(t any) (exp.Expression, any) {
	switch v := t.(type) {
	case exp.Expression:
		if v == nil {
			return nil, nil
		}
		return v, v.Arg("this")
	case exp.DType:
		if v == "" {
			return nil, nil
		}
		return nil, v
	}
	return nil, nil
}

// MaybeCoerce returns type2 if type1 can be coerced into it, otherwise type1.
//
// If either type is parameterized (e.g. DECIMAL(18, 2) contains two parameters),
// we assume type1 does not coerce into type2, so we also return it in this case.
func (a *TypeAnnotator) MaybeCoerce(type1, type2 any) any {
	var type1Value, type2Value any
	if dt, this := typeValue(type1); dt != nil {
		if _, isDType := this.(exp.DType); len(dt.Expressions()) > 0 || !isDType {
			return type1
		}
		type1Value = this
	} else {
		type1Value = this
	}

	if dt, this := typeValue(type2); dt != nil {
		if _, isDType := this.(exp.DType); len(dt.Expressions()) > 0 || !isDType {
			return type2
		}
		type2Value = this
	} else {
		type2Value = this
	}

	// We propagate the UNKNOWN type upwards if found
	if type1Value == exp.DTypeUnknown || type2Value == exp.DTypeUnknown {
		return exp.DTypeUnknown
	}

	if type1Value == exp.DTypeNull {
		return type2Value
	}
	if type2Value == exp.DTypeNull {
		return type1Value
	}

	t1, _ := type1Value.(exp.DType)
	t2, _ := type2Value.(exp.DType)
	if t2 != "" && a.coercesTo[t1][t2] {
		return type2Value
	}
	return type1Value
}

func (a *TypeAnnotator) getSetopColumnTypes(setop exp.Expression) *typeMap {
	if cached, ok := a.setopColumnTypes[setop]; ok {
		return cached
	}

	colTypes := newTypeMap()

	// Validate that left and right have same number of projections (BY NAME
	// operations match columns by name, so their counts are allowed to differ)
	l, r := setop.This(), setop.Expr()
	if !(setop.Is(exp.TraitSetOperation) && l != nil && r != nil && len(l.Selects()) > 0 && len(r.Selects()) > 0 &&
		(truthy(setop.Arg("by_name")) || len(l.Selects()) == len(r.Selects()))) {
		return colTypes
	}

	// Process a chain / sub-tree of set operations
	prune := func(n exp.Expression) bool {
		return !n.Is(exp.TraitSetOperation) && n.Kind() != exp.KindSubquery
	}
	for _, setOp := range setop.WalkWithPrune(true, prune) {
		if !setOp.Is(exp.TraitSetOperation) {
			continue
		}

		setopCols := newTypeMap()
		left, right := setOp.This(), setOp.Expr()
		if left == nil || right == nil {
			continue
		}
		if truthy(setOp.Arg("by_name")) {
			// Columns missing from one side are filled with NULLs, so the other
			// side's type is preserved (NULL is the identity for _maybe_coerce)
			rTypeBySelect := newTypeMap()
			for _, s := range right.Selects() {
				rTypeBySelect.set(s.AliasOrName(), typeOrNil(s.Type()))
			}
			for _, s := range left.Selects() {
				var rType any = exp.DTypeNull
				if v, ok := rTypeBySelect.pop(s.AliasOrName()); ok {
					rType = v
				}
				if rType == nil {
					rType = exp.DTypeUnknown
				}
				setopCols.set(s.AliasOrName(), a.MaybeCoerce(typeOrNil(s.Type()), rType))
			}
			for _, name := range rTypeBySelect.keys {
				rType := rTypeBySelect.m[name]
				if rType == nil {
					rType = exp.DTypeUnknown
				}
				setopCols.set(name, rType)
			}
		} else {
			ls, rs := left.Selects(), right.Selects()
			for i := 0; i < len(ls) && i < len(rs); i++ {
				setopCols.set(ls[i].AliasOrName(), a.MaybeCoerce(typeOrNil(ls[i].Type()), typeOrNil(rs[i].Type())))
			}
		}

		// Coerce intermediate results with the previously registered types, if they exist
		for _, colName := range setopCols.keys {
			prev := colTypes.get(colName)
			if !colTypes.has(colName) {
				prev = exp.DTypeNull
			}
			colTypes.set(colName, a.MaybeCoerce(setopCols.m[colName], prev))
		}
	}

	a.setopColumnTypes[setop] = colTypes
	return colTypes
}

func (a *TypeAnnotator) getUnpivotColumnTypes(pivot exp.Expression, srcTypes *typeMap) *typeMap {
	newTypes := map[string]any{}

	for _, field := range expressionsFor(pivot, "fields") {
		fieldCol := field.This()
		first, _ := seqGet(field.Expressions(), 0)

		var inSrc exp.Expression
		if alias := aliasNode(first); first != nil && first.Kind() == exp.KindPivotAlias && alias != nil {
			newTypes[fieldCol.Name()] = typeOrNil(alias.Type())
			inSrc = first.This()
		} else {
			newTypes[fieldCol.Name()] = exp.DTypeVarchar.IntoExpr(nil)
			inSrc = first
		}

		inCols := []exp.Expression{inSrc}
		if inSrc != nil && inSrc.Kind() == exp.KindTuple {
			inCols = inSrc.Expressions()
		}
		valExpr, _ := seqGet(pivot.Expressions(), 0)
		valCols := []exp.Expression{valExpr}
		if valExpr != nil && valExpr.Kind() == exp.KindTuple {
			valCols = valExpr.Expressions()
		}
		for i := 0; i < len(valCols) && i < len(inCols); i++ {
			valCol, inCol := valCols[i], inCols[i]
			if valCol == nil || inCol == nil {
				continue
			}
			// A chained operator's IN-list may name columns an earlier operator
			// produced, which carry no annotation of their own
			var inType any = typeOrNil(inCol.Type())
			if inCol.Type() == nil || inCol.Type().IsType(exp.DTypeUnknown) {
				if v := srcTypes.get(inCol.OutputName()); v != nil {
					inType = v
				}
			}
			newTypes[valCol.OutputName()] = inType
		}
	}

	out := newTypeMap()
	names, _ := exp.PivotOutputColumns(pivot, srcTypes.keys)
	for _, name := range names {
		typ := newTypes[name]
		if typ == nil {
			typ = srcTypes.get(name)
		}
		if typ != nil {
			out.set(name, typ)
		}
	}
	return out
}

func aliasNode(e exp.Expression) exp.Expression {
	if e == nil {
		return nil
	}
	return asExpression(e.Arg("alias"))
}

func (a *TypeAnnotator) getPivotColumnTypes(pivot exp.Expression, srcTypes *typeMap) *typeMap {
	firstField, _ := seqGet(expressionsFor(pivot, "fields"), 0)
	if firstField == nil || firstField.Kind() != exp.KindIn {
		panic(fmt.Errorf("Expected In expression for pivot field, got %v", firstField))
	}

	pivotConstants := firstField.Expressions()

	// The first agg_cols_offset entries are source columns that pass through the PIVOT unchanged;
	// the rest are the aggregated columns, one per combination of IN value and aggregate function.
	outputNames, outputToSrc := exp.PivotOutputColumns(pivot, srcTypes.keys)

	var aggTypes []any
	for _, agg := range pivot.Expressions() {
		if agg.Kind() == exp.KindAlias {
			aggTypes = append(aggTypes, typeOrNil(agg.This().Type()))
		} else {
			aggTypes = append(aggTypes, typeOrNil(agg.Type()))
		}
	}
	aggColsOffset := len(outputNames) - len(pivotConstants)*len(aggTypes)
	if aggColsOffset < 0 {
		panic(fmt.Errorf("Negative pivot column offset: %d", aggColsOffset))
	}

	newTypes := newTypeMap()

	for _, name := range outputNames[:aggColsOffset] {
		if typ := srcTypes.get(outputToSrc[name]); typ != nil {
			newTypes.set(name, typ)
		}
	}

	var flat []any
	for range pivotConstants {
		flat = append(flat, aggTypes...)
	}
	rest := outputNames[aggColsOffset:]
	for i := 0; i < len(rest) && i < len(flat); i++ {
		if flat[i] != nil {
			newTypes.set(rest[i], flat[i])
		}
	}

	return newTypes
}

// typeOrNil turns a nil Expression into an untyped nil so it compares equal to nil as `any`.
func typeOrNil(e exp.Expression) any {
	if e == nil {
		return nil
	}
	return e
}

// thisOf is `x.type.this if x.type else exp.DType.UNKNOWN`.
func thisOf(e exp.Expression) any {
	if e == nil {
		return exp.DTypeUnknown
	}
	if t := e.Type(); t != nil {
		return t.Arg("this")
	}
	return exp.DTypeUnknown
}

// dtypeOf is the DType of a type value, or "" when it has none.
func dtypeOf(t any) exp.DType {
	_, this := typeValue(t)
	d, _ := this.(exp.DType)
	return d
}

func (a *TypeAnnotator) AnnotateBinary(expression exp.Expression) exp.Expression {
	left, right := expression.Left(), expression.Right()
	if left == nil || right == nil {
		// upstream logs a warning here: Failed to annotate badly formed binary expression
		a.SetType(expression, nil)
		return expression
	}

	leftType, rightType := thisOf(left), thisOf(right)

	lt, _ := leftType.(exp.DType)
	rt, _ := rightType.(exp.DType)
	if expression.Is(exp.TraitConnector) || expression.Is(exp.TraitPredicate) {
		a.SetType(expression, exp.DTypeBoolean)
	} else if f, ok := a.binaryCoercions[[2]exp.DType{lt, rt}]; ok && lt != "" && rt != "" {
		a.SetType(expression, f(left, right))
	} else {
		a.AnnotateByArgs(expression, []any{left, right}, false, false)
	}

	if expression.Kind() == exp.KindIs || (left.MetaGet("nonnull") == true && right.MetaGet("nonnull") == true) {
		expression.Meta()["nonnull"] = true
	}

	return expression
}

func (a *TypeAnnotator) annotateUnary(expression exp.Expression) exp.Expression {
	if expression.Kind() == exp.KindNot {
		a.SetType(expression, exp.DTypeBoolean)
	} else {
		var t any
		if this := expression.This(); this != nil {
			t = typeOrNil(this.Type())
		}
		a.SetType(expression, t)
	}

	if this := expression.This(); this != nil && this.MetaGet("nonnull") == true {
		expression.Meta()["nonnull"] = true
	}

	return expression
}

func (a *TypeAnnotator) annotateLiteral(expression exp.Expression) exp.Expression {
	switch {
	case expression.IsString():
		a.SetType(expression, exp.DTypeVarchar)
	case expression.IsInt():
		a.SetType(expression, exp.DTypeInt)
	default:
		a.SetType(expression, exp.DTypeDouble)
	}

	expression.Meta()["nonnull"] = true

	return expression
}

// AnnotateByArgs ports _annotate_by_args (annotate_types.py:822-893). Each arg is an arg key
// (string) or an expression.
func (a *TypeAnnotator) AnnotateByArgs(expression exp.Expression, args []any, promote bool, array bool) exp.Expression {
	var literalType, nonLiteralType, nestedType any

	for _, arg := range args {
		var expressions []exp.Expression
		switch v := arg.(type) {
		case string:
			expressions = exprList(expression.Arg(v))
		default:
			expressions = exprList(v)
		}

		for _, expr := range expressions {
			exprType := expr.Type()

			if exprType == nil || exprType.IsType(exp.DTypeUnknown) {
				a.SetType(expression, exp.DTypeUnknown)
				return expression
			}

			if nestedType != nil {
				continue
			}

			// Stop coercing at the first nested data type found
			if truthy(exprType.Arg("nested")) {
				nestedType = exprType
			} else if expr.Kind() == exp.KindLiteral {
				if literalType == nil {
					literalType = exprType
				}
				literalType = a.MaybeCoerce(literalType, exprType)
			} else {
				if nonLiteralType == nil {
					nonLiteralType = exprType
				}
				nonLiteralType = a.MaybeCoerce(nonLiteralType, exprType)
			}
		}
	}

	var resultType any

	switch {
	case nestedType != nil:
		resultType = nestedType
	case literalType != nil && nonLiteralType != nil:
		if a.dialect.PrioritizeNonLiteralTypes {
			_, literalThisType := typeValue(literalType)
			_, nonLiteralThisType := typeValue(nonLiteralType)
			lt, _ := literalThisType.(exp.DType)
			nt, _ := nonLiteralThisType.(exp.DType)
			if (exp.IntegerTypes[lt] && exp.IntegerTypes[nt]) || (exp.RealTypes[lt] && exp.RealTypes[nt]) {
				resultType = nonLiteralType
			}
		}

		if resultType == nil {
			resultType = a.MaybeCoerce(nonLiteralType, literalType)
		}
	default:
		resultType = literalType
		if resultType == nil {
			resultType = nonLiteralType
		}
		if resultType == nil {
			resultType = exp.DTypeUnknown
		}
	}

	a.SetType(expression, resultType)

	if promote {
		thisType := dtypeOf(resultType)
		if exp.IntegerTypes[thisType] {
			a.SetType(expression, exp.DTypeBigInt)
		} else if exp.FloatTypes[thisType] {
			a.SetType(expression, exp.DTypeDouble)
		}
	}

	if array {
		a.SetType(expression, exp.DataType(exp.Args{"this": exp.DTypeArray, "expressions": []exp.Expression{expression.Type()}, "nested": true}))
	}

	return expression
}

// exprList is helper.ensure_list over an arg value, keeping only expressions.
func exprList(v any) []exp.Expression {
	switch x := v.(type) {
	case nil:
		return nil
	case exp.Expression:
		if x == nil {
			return nil
		}
		return []exp.Expression{x}
	case []exp.Expression:
		out := make([]exp.Expression, 0, len(x))
		for _, e := range x {
			if e != nil {
				out = append(out, e)
			}
		}
		return out
	case []any:
		var out []exp.Expression
		for _, item := range x {
			out = append(out, exprList(item)...)
		}
		return out
	}
	return nil
}

func (a *TypeAnnotator) annotateTimeunit(expression exp.Expression) exp.Expression {
	this := expression.This()
	thisType := dtypeOf(typeOrNil(typeOfOrNil(this)))
	unit := asExpression(expression.Arg("unit"))
	var datatype any
	switch {
	case exp.TextTypes[thisType]:
		datatype = coerceDateLiteral(this, unit)
	case exp.TemporalTypes[thisType]:
		datatype = coerceDate(this, unit)
	default:
		datatype = exp.DTypeUnknown
	}

	a.SetType(expression, datatype)
	return expression
}

func typeOfOrNil(e exp.Expression) exp.Expression {
	if e == nil {
		return nil
	}
	return e.Type()
}

func (a *TypeAnnotator) annotateBracket(expression exp.Expression) exp.Expression {
	bracketArg, _ := seqGet(expression.Expressions(), 0)
	this := expression.This()

	switch {
	case bracketArg != nil && bracketArg.Kind() == exp.KindSlice:
		a.SetType(expression, typeOrNil(this.Type()))
	case this.Type() != nil && this.Type().IsType(exp.DTypeArray):
		first, _ := seqGet(this.Type().Expressions(), 0)
		a.SetType(expression, typeOrNil(first))
	case (this.Kind() == exp.KindMap || this.Kind() == exp.KindVarMap) && indexOfExpr(mapKeys(this), bracketArg) >= 0:
		index := indexOfExpr(mapKeys(this), bracketArg)
		value, _ := seqGet(mapValues(this), index)
		if value != nil {
			a.SetType(expression, typeOrNil(value.Type()))
		} else {
			a.SetType(expression, nil)
		}
	default:
		a.SetType(expression, exp.DTypeUnknown)
	}

	return expression
}

func mapKeys(e exp.Expression) []exp.Expression {
	if keys := asExpression(e.Arg("keys")); keys != nil {
		return keys.Expressions()
	}
	return nil
}

func mapValues(e exp.Expression) []exp.Expression {
	if values := asExpression(e.Arg("values")); values != nil {
		return values.Expressions()
	}
	return nil
}

func indexOfExpr(list []exp.Expression, e exp.Expression) int {
	if e == nil {
		return -1
	}
	for i, x := range list {
		if x != nil && x.Equal(e) {
			return i
		}
	}
	return -1
}

func (a *TypeAnnotator) AnnotateDiv(expression exp.Expression) exp.Expression {
	leftType, rightType := thisOf(expression.Left()), thisOf(expression.Right())
	lt, _ := leftType.(exp.DType)
	rt, _ := rightType.(exp.DType)

	if truthy(expression.Arg("typed")) && exp.IntegerTypes[lt] && exp.IntegerTypes[rt] {
		a.SetType(expression, exp.DTypeBigInt)
	} else {
		a.SetType(expression, a.MaybeCoerce(leftType, rightType))
		if t := expression.Type(); t != nil {
			if d, _ := t.Arg("this").(exp.DType); !exp.RealTypes[d] {
				a.SetType(expression, a.MaybeCoerce(t, exp.DTypeDouble))
			}
		}
	}

	return expression
}

func (a *TypeAnnotator) annotateDot(expression exp.Expression) exp.Expression {
	a.SetType(expression, nil)

	// Propagate type from qualified UDF calls (e.g., db.my_udf(...))
	if e := expression.Expr(); e != nil && e.Kind() == exp.KindAnonymous {
		a.SetType(expression, typeOrNil(e.Type()))
		return expression
	}

	thisType := typeOfOrNil(expression.This())

	if thisType != nil && thisType.IsType(exp.DTypeStruct) {
		name := ""
		if e := expression.Expr(); e != nil {
			name = e.Name()
		}
		for _, e := range thisType.Expressions() {
			if e.Name() == name {
				a.SetType(expression, asExpression(e.Arg("kind")))
				break
			}
		}
	}

	return expression
}

func (a *TypeAnnotator) annotateExplode(expression exp.Expression) exp.Expression {
	inputType := typeOfOrNil(expression.This())
	if inputType != nil && inputType.IsType(exp.DTypeArray) {
		first, _ := seqGet(inputType.Expressions(), 0)
		a.SetType(expression, typeOrNil(first))
	} else {
		a.SetType(expression, nil)
	}
	return expression
}

func (a *TypeAnnotator) annotateUnnest(expression exp.Expression) exp.Expression {
	child, _ := seqGet(expression.Expressions(), 0)

	var exprType any
	if child != nil && child.IsType(exp.DTypeArray) {
		first, _ := seqGet(child.Type().Expressions(), 0)
		exprType = typeOrNil(first)
	}

	a.SetType(expression, exprType)
	return expression
}

func (a *TypeAnnotator) annotateSubquery(expression exp.Expression) exp.Expression {
	// For scalar subqueries (subqueries with a single projection), infer the type
	// from that single projection. This allows type propagation in cases like:
	// SELECT (SELECT 1 AS c) AS c
	query := expression.Unnest()

	if query != nil && query.Is(exp.TraitQuery) {
		if selects := query.Selects(); len(selects) == 1 {
			a.SetType(expression, typeOrNil(selects[0].Type()))
			return expression
		}
	}

	a.SetType(expression, exp.DTypeUnknown)
	return expression
}

// annotateStructValue returns the struct field's type: nil, a DataType or a ColumnDef.
func (a *TypeAnnotator) annotateStructValue(expression exp.Expression) exp.Expression {
	// Case: STRUCT(key AS value)
	var this exp.Expression
	kind := expression.Type()

	if alias := asExpression(expression.Arg("alias")); alias != nil {
		this = alias.Copy()
	} else if e := expression.Expr(); e != nil {
		// Case: STRUCT(key = value) or STRUCT(key := value)
		this = expression.This().Copy()
		kind = e.Type()
	} else if expression.Kind() == exp.KindColumn {
		// Case: STRUCT(c)
		this = expression.This().Copy()
	}

	if kind != nil && kind.IsType(exp.DTypeUnknown) {
		return nil
	}

	if this != nil {
		return exp.New(exp.KindColumnDef, exp.Args{"this": this, "kind": typeOrNil(kind)})
	}

	return kind
}

func (a *TypeAnnotator) annotateStruct(expression exp.Expression) exp.Expression {
	var expressions []exp.Expression
	for _, expr := range expression.Expressions() {
		structFieldType := a.annotateStructValue(expr)
		if structFieldType == nil {
			a.SetType(expression, nil)
			return expression
		}

		expressions = append(expressions, structFieldType)
	}

	a.SetType(expression, exp.DataType(exp.Args{"this": exp.DTypeStruct, "expressions": expressions, "nested": true}))
	return expression
}

func (a *TypeAnnotator) annotateMap(expression exp.Expression) exp.Expression {
	keys := asExpression(expression.Arg("keys"))
	values := asExpression(expression.Arg("values"))

	mapType := exp.DataType(exp.Args{"this": exp.DTypeMap})
	if keys != nil && values != nil && keys.Kind() == exp.KindArray && values.Kind() == exp.KindArray {
		keyType, _ := seqGet(typeExpressions(keys), 0)
		valueType, _ := seqGet(typeExpressions(values), 0)

		// upstream compares `DataType | DType.UNKNOWN` to DType.UNKNOWN: only a missing element type is unknown
		if keyType != nil && valueType != nil {
			mapType.Set("expressions", []exp.Expression{keyType, valueType})
			mapType.Set("nested", true)
		}
	}

	a.SetType(expression, mapType)
	return expression
}

func typeExpressions(e exp.Expression) []exp.Expression {
	if t := e.Type(); t != nil {
		return t.Expressions()
	}
	return nil
}

func (a *TypeAnnotator) annotateToMap(expression exp.Expression) exp.Expression {
	mapType := exp.DataType(exp.Args{"this": exp.DTypeMap})
	arg := expression.This()
	if arg != nil && arg.IsType(exp.DTypeStruct) {
		// upstream's `kind != exp.DType.UNKNOWN` compares a DataType to a DType, so the first field wins
		if coldef, ok := seqGet(arg.Type().Expressions(), 0); ok {
			kind := asExpression(coldef.Arg("kind"))
			mapType.Set("expressions", []exp.Expression{exp.DTypeVarchar.IntoExpr(nil), kind})
			mapType.Set("nested", true)
		}
	}

	a.SetType(expression, mapType)
	return expression
}

func (a *TypeAnnotator) annotateExtract(expression exp.Expression) exp.Expression {
	part := expression.Name()
	switch {
	case part == "TIME":
		a.SetType(expression, exp.DTypeTime)
	case part == "DATE":
		a.SetType(expression, exp.DTypeDate)
	case bigintExtractDateParts[part]:
		a.SetType(expression, exp.DTypeBigInt)
	default:
		a.SetType(expression, exp.DTypeInt)
	}
	return expression
}

func (a *TypeAnnotator) annotateWithinGroup(expression exp.Expression) exp.Expression {
	if this := expression.This(); this != nil && this.Kind() == exp.KindPercentileDisc {
		order := expression.Expr()
		var sortType any = exp.DTypeUnknown
		if order != nil && len(order.Expressions()) > 0 {
			sortType = typeOrNil(order.Expressions()[0].This().Type())
		}
		a.SetType(expression, sortType)
		return expression
	}

	return a.AnnotateByArgs(expression, []any{"this"}, false, false)
}

func (a *TypeAnnotator) annotateByArrayElement(expression exp.Expression) exp.Expression {
	arrayArg := expression.This()
	if t := typeOfOrNil(arrayArg); t != nil && t.IsType(exp.DTypeArray) {
		first, _ := seqGet(t.Expressions(), 0)
		if first != nil {
			a.SetType(expression, first)
		} else {
			a.SetType(expression, exp.DTypeUnknown)
		}
	} else {
		a.SetType(expression, exp.DTypeUnknown)
	}

	return expression
}

// isISODate ports helper.is_iso_date (helper.py:432), i.e. Python's date.fromisoformat.
func isISODate(text string) bool {
	n, ok := isoDatePrefix(text)
	return ok && n == len(text)
}

// isISODatetime ports helper.is_iso_datetime (helper.py:440), i.e. Python's
// datetime.fromisoformat: an ISO date, optionally followed by any one separator and an ISO time.
func isISODatetime(text string) bool {
	n, ok := isoDatePrefix(text)
	if !ok {
		return false
	}
	if n == len(text) {
		return true
	}
	return isISOTime(text[n+1:])
}

func isoDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isoAtoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// isoDatePrefix parses the date component and returns its length.
func isoDatePrefix(s string) (int, bool) {
	if len(s) < 7 || !isoDigits(s[:4]) {
		return 0, false
	}
	year := isoAtoi(s[:4])
	if year < 1 {
		return 0, false
	}
	rest := s[4:]
	extended := strings.HasPrefix(rest, "-")
	if extended {
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "W") {
		if len(rest) < 3 || !isoDigits(rest[1:3]) {
			return 0, false
		}
		week := isoAtoi(rest[1:3])
		length := 4 + boolInt(extended) + 3
		day := 1
		tail := rest[3:]
		if extended && strings.HasPrefix(tail, "-") && len(tail) >= 2 && isoDigits(tail[1:2]) {
			day = isoAtoi(tail[1:2])
			length += 2
		} else if !extended && len(tail) >= 1 && isoDigits(tail[:1]) {
			day = isoAtoi(tail[:1])
			length++
		}
		maxWeek := 52
		jan1 := isoWeekday(year, 1, 1)
		if jan1 == 4 || (jan1 == 3 && isLeapYear(year)) {
			maxWeek = 53
		}
		if week < 1 || week > maxWeek || day < 1 || day > 7 {
			return 0, false
		}
		return length, true
	}
	var month, day int
	var length int
	if extended {
		if len(rest) < 5 || !isoDigits(rest[:2]) || rest[2] != '-' || !isoDigits(rest[3:5]) {
			return 0, false
		}
		month, day, length = isoAtoi(rest[:2]), isoAtoi(rest[3:5]), 10
	} else {
		if len(rest) < 4 || !isoDigits(rest[:4]) {
			return 0, false
		}
		month, day, length = isoAtoi(rest[:2]), isoAtoi(rest[2:4]), 8
	}
	if month < 1 || month > 12 || day < 1 || day > daysInMonth(year, month) {
		return 0, false
	}
	return length, true
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isLeapYear(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func daysInMonth(y, m int) int {
	switch m {
	case 2:
		if isLeapYear(y) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// isoWeekday is the ISO weekday (Monday=1) of a proleptic Gregorian date.
func isoWeekday(y, m, d int) int {
	t := []int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	if m < 3 {
		y--
	}
	w := (y + y/4 - y/100 + y/400 + t[m-1] + d) % 7 // 0 = Sunday
	if w == 0 {
		return 7
	}
	return w
}

// isISOTime ports the time half of datetime.fromisoformat: HH[:MM[:SS[.f+]]] (or the basic
// HHMMSS form) and an optional Z / ±HH[:MM[:SS[.f+]]] offset.
func isISOTime(s string) bool {
	if s == "" {
		return false
	}
	if i := strings.IndexAny(s, "+-Z"); i >= 0 {
		if s[i] == 'Z' {
			if i != len(s)-1 {
				return false
			}
		} else if !isISOClock(s[i+1:]) {
			return false
		}
		s = s[:i]
	}
	return isISOClock(s)
}

func isISOClock(s string) bool {
	frac := ""
	if i := strings.IndexAny(s, ".,"); i >= 0 {
		frac, s = s[i+1:], s[:i]
		if !isoDigits(frac) {
			return false
		}
	}
	var parts []string
	if strings.Contains(s, ":") {
		parts = strings.Split(s, ":")
	} else {
		if len(s)%2 != 0 {
			return false
		}
		for i := 0; i < len(s); i += 2 {
			parts = append(parts, s[i:i+2])
		}
	}
	if len(parts) == 0 || len(parts) > 3 || (frac != "" && len(parts) != 3) {
		return false
	}
	limits := []int{23, 59, 59}
	for i, p := range parts {
		if len(p) != 2 || !isoDigits(p) || isoAtoi(p) > limits[i] {
			return false
		}
	}
	return true
}
