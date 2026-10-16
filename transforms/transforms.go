// Package transforms ports sqlglot/transforms.py: AST rewrites a generator applies before
// rendering a node for a dialect that lacks the construct. Each function mutates and returns
// its input like upstream; the generator copies the root once before preprocessing.
package transforms

import (
	"fmt"
	"strings"

	sqlerrors "github.com/sevbanbayrak/sqlglot-go/errors"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
)

func findNewName(taken map[string]bool, base string) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s_%d", base, i)
		if !taken[name] {
			return name
		}
	}
}

func namedSelects(e exp.Expression) map[string]bool {
	taken := map[string]bool{}
	for _, name := range e.NamedSelects() {
		taken[name] = true
	}
	return taken
}

func column(name string, quoted any) exp.Expression {
	args := exp.Args{"this": name}
	if q, ok := quoted.(bool); ok {
		args["quoted"] = q
	}
	return exp.Column(exp.Args{"this": exp.Identifier(args)})
}

func subquery(e exp.Expression, alias string) exp.Expression {
	return exp.New(exp.KindSubquery, exp.Args{
		"this":  e,
		"alias": exp.New(exp.KindTableAlias, exp.Args{"this": exp.ToIdentifier(alias)}),
	})
}

func selectFromWhere(selects []exp.Expression, from exp.Expression, where exp.Expression) exp.Expression {
	return exp.Select(exp.Args{
		"expressions": selects,
		"from_":       exp.New(exp.KindFrom, exp.Args{"this": from}),
		"where":       exp.New(exp.KindWhere, exp.Args{"this": where}),
	})
}

func isStarSelect(e exp.Expression) bool {
	for _, s := range e.Expressions() {
		if s.IsStar() {
			return true
		}
	}
	return false
}

// EliminateDistinctOn ports eliminate_distinct_on (transforms.py:144-198).
func EliminateDistinctOn(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	distinct := e.Arg("distinct")
	d, _ := distinct.(exp.Expression)
	if d == nil {
		return e
	}
	on, _ := d.Arg("on").(exp.Expression)
	if on == nil || on.Kind() != exp.KindTuple {
		return e
	}
	taken := namedSelects(e)
	rowNumberAlias := findNewName(taken, "_row_number")
	d.Pop()
	distinctCols := on.Expressions()
	window := exp.New(exp.KindWindow, exp.Args{"this": exp.New(exp.KindRowNumber, nil), "partition_by": distinctCols})
	if order, _ := e.Arg("order").(exp.Expression); order != nil {
		window.Set("order", order.Pop())
	} else {
		copies := make([]exp.Expression, 0, len(distinctCols))
		for _, c := range distinctCols {
			copies = append(copies, c.Copy())
		}
		window.Set("order", exp.New(exp.KindOrder, exp.Args{"expressions": copies}))
	}
	e.Append("expressions", exp.AliasExpr(window, rowNumberAlias, false))

	var newSelects []exp.Expression
	taken = map[string]bool{rowNumberAlias: true}
	selects := e.Expressions()
	for _, sel := range selects[:len(selects)-1] {
		if sel.IsStar() {
			newSelects = []exp.Expression{exp.Star(nil)}
			break
		}
		if sel.Kind() != exp.KindAlias {
			name := sel.OutputName()
			if name == "" {
				name = "_col"
			}
			alias := findNewName(taken, name)
			var quoted any
			if sel.Kind() == exp.KindColumn {
				quoted = sel.This().Arg("quoted")
			}
			aliased := exp.AliasExpr(sel, alias, false)
			if q, ok := quoted.(bool); ok {
				aliased.Arg("alias").(exp.Expression).Set("quoted", q)
			}
			sel = sel.Replace(aliased)
		}
		taken[sel.OutputName()] = true
		newSelects = append(newSelects, sel.Arg("alias").(exp.Expression).Copy())
	}
	where := exp.EQ(exp.Args{"this": column(rowNumberAlias, nil), "expression": exp.LiteralNumber(1)})
	return selectFromWhere(newSelects, subquery(e, "_t"), where)
}

// EliminateQualify ports eliminate_qualify (transforms.py:201-262).
func EliminateQualify(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	qualify, _ := e.Arg("qualify").(exp.Expression)
	if qualify == nil {
		return e
	}
	taken := namedSelects(e)
	for _, sel := range e.Expressions() {
		if sel.AliasOrName() == "" {
			alias := findNewName(taken, "_c")
			sel.Replace(exp.AliasExpr(sel, alias, false))
			taken[alias] = true
		}
	}

	var outerSelects []exp.Expression
	for _, sel := range e.Expressions() {
		identifier, _ := sel.Arg("alias").(exp.Expression)
		if identifier == nil {
			identifier = sel.This()
		}
		var quoted any
		if identifier != nil && identifier.Kind() == exp.KindIdentifier {
			quoted = identifier.Arg("quoted")
		}
		outerSelects = append(outerSelects, column(sel.AliasOrName(), quoted))
	}

	qualifyFilters := qualify.Pop().This()
	byAlias := map[string]exp.Expression{}
	for _, sel := range e.Expressions() {
		if sel.Kind() == exp.KindAlias {
			byAlias[sel.Alias()] = sel.This()
		}
	}

	star := isStarSelect(e)
	var candidates []exp.Expression
	for _, node := range qualifyFilters.Walk(true) {
		if node.Kind() == exp.KindWindow || (!star && node.Kind() == exp.KindColumn) {
			candidates = append(candidates, node)
		}
	}
	for _, candidate := range candidates {
		if candidate.Kind() == exp.KindWindow {
			if len(byAlias) > 0 {
				for _, col := range candidate.FindAll(exp.KindColumn) {
					if expr := byAlias[col.Name()]; expr != nil {
						col.Replace(expr)
					}
				}
			}
			alias := findNewName(namedSelects(e), "_w")
			e.Append("expressions", exp.AliasExpr(candidate.Copy(), alias, false))
			col := column(alias, nil)
			if p := candidate.Parent(); p != nil && p.Kind() == exp.KindQualify {
				qualifyFilters = col
			} else {
				candidate.Replace(col)
			}
		} else if !namedSelects(e)[candidate.Name()] {
			e.Append("expressions", candidate.Copy())
		}
	}
	return selectFromWhere(outerSelects, subquery(e, "_t"), qualifyFilters)
}

func udtfKind(unnest exp.Expression, multi bool) exp.Kind {
	if unnest.Arg("offset") != nil {
		return exp.KindPosexplode
	}
	if multi {
		return exp.KindInline
	}
	return exp.KindExplode
}

func offsetColumn(offset any) exp.Expression {
	if o, ok := offset.(exp.Expression); ok && o.Kind() == exp.KindIdentifier {
		return o
	}
	return exp.ToIdentifier("pos")
}

// UnnestToExplode ports unnest_to_explode (transforms.py:297-399): CROSS JOIN UNNEST becomes
// LATERAL VIEW EXPLODE. It returns an *UnsupportedError for the shapes upstream rejects.
func UnnestToExplode(e exp.Expression, unnestUsingArraysZip bool) (exp.Expression, error) {
	if e.Kind() != exp.KindSelect {
		return e, nil
	}
	zip := func(u exp.Expression, exprs []exp.Expression, multi bool) ([]exp.Expression, error) {
		if !multi {
			return exprs, nil
		}
		if !unnestUsingArraysZip {
			return nil, &sqlerrors.UnsupportedError{Msg: "Cannot transpile UNNEST with multiple input arrays"}
		}
		zipped := []exp.Expression{exp.Anonymous(exp.Args{"this": "ARRAYS_ZIP", "expressions": exprs})}
		u.Set("expressions", zipped)
		return zipped, nil
	}

	if from, _ := e.Arg("from_").(exp.Expression); from != nil && from.This() != nil && from.This().Kind() == exp.KindUnnest {
		unnest := from.This()
		alias, _ := unnest.Arg("alias").(exp.Expression)
		exprs := unnest.Expressions()
		multi := len(exprs) > 1
		exprs, err := zip(unnest, exprs, multi)
		if err != nil {
			return e, err
		}
		var columns []exp.Expression
		if alias != nil {
			columns = listArg(alias, "columns")
		}
		if offset := unnest.Arg("offset"); offset != nil {
			columns = append([]exp.Expression{offsetColumn(offset)}, columns...)
		}
		table := exp.Args{"this": exp.New(udtfKind(unnest, multi), exp.Args{"this": exprs[0]})}
		if alias != nil {
			table["alias"] = exp.New(exp.KindTableAlias, exp.Args{"this": alias.This(), "columns": columns})
		}
		unnest.Replace(exp.New(exp.KindTable, table))
	}

	joins := listArg(e, "joins")
	var kept []exp.Expression
	for _, join := range joins {
		joinExpr := join.This()
		isLateral := joinExpr != nil && joinExpr.Kind() == exp.KindLateral
		unnest := joinExpr
		if isLateral {
			unnest = joinExpr.This()
		}
		if unnest == nil || unnest.Kind() != exp.KindUnnest {
			kept = append(kept, join)
			continue
		}
		var alias exp.Expression
		if isLateral {
			alias, _ = joinExpr.Arg("alias").(exp.Expression)
		} else {
			alias, _ = unnest.Arg("alias").(exp.Expression)
		}
		if alias == nil {
			return e, &sqlerrors.UnsupportedError{Msg: "CROSS JOIN UNNEST to LATERAL VIEW EXPLODE transformation requires an alias"}
		}
		exprs := unnest.Expressions()
		multi := len(exprs) > 1
		exprs, err := zip(unnest, exprs, multi)
		if err != nil {
			return e, err
		}
		aliasCols := listArg(alias, "columns")
		if !multi && len(aliasCols) != 1 && len(aliasCols) != 2 {
			return e, &sqlerrors.UnsupportedError{Msg: "CROSS JOIN UNNEST to LATERAL VIEW EXPLODE transformation requires explicit column aliases"}
		}
		if offset := unnest.Arg("offset"); offset != nil {
			aliasCols = append([]exp.Expression{offsetColumn(offset)}, aliasCols...)
		}
		for i := 0; i < len(exprs) && i < len(aliasCols); i++ {
			e.Append("laterals", exp.New(exp.KindLateral, exp.Args{
				"this":  exp.New(udtfKind(unnest, multi), exp.Args{"this": exprs[i]}),
				"view":  true,
				"alias": exp.New(exp.KindTableAlias, exp.Args{"this": alias.This(), "columns": aliasCols}),
			}))
		}
	}
	if len(kept) != len(joins) {
		e.Set("joins", kept)
	}
	return e, nil
}

// AnyToExists ports any_to_exists (transforms.py:974-998).
func AnyToExists(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	for _, anyExpr := range e.FindAll(exp.KindAny) {
		this := anyExpr.This()
		if this == nil || this.Is(exp.TraitQuery) {
			continue
		}
		if p := anyExpr.Parent(); p != nil && (p.Kind() == exp.KindLike || p.Kind() == exp.KindILike) {
			continue
		}
		binop := anyExpr.Parent()
		if binop == nil || !binop.Is(exp.TraitBinary) {
			continue
		}
		lambdaArg := exp.ToIdentifier("x")
		anyExpr.Replace(lambdaArg)
		lambda := exp.New(exp.KindLambda, exp.Args{"this": binop.Copy(), "expressions": []exp.Expression{lambdaArg}})
		binop.Replace(exp.New(exp.KindExists, exp.Args{"this": this.Unnest(), "expression": lambda}))
	}
	return e
}

// MoveCtesToTopLevel ports move_ctes_to_top_level (transforms.py:672-708).
func MoveCtesToTopLevel(e exp.Expression) exp.Expression {
	topLevel, _ := e.Arg("with_").(exp.Expression)
	for _, inner := range e.FindAll(exp.KindWith) {
		if inner.Parent() == e {
			continue
		}
		if topLevel == nil {
			topLevel = inner.Pop()
			e.Set("with_", topLevel)
			continue
		}
		if truthy(inner.Arg("recursive")) {
			topLevel.Set("recursive", true)
		}
		parentCTE := inner.FindAncestor(exp.KindCTE)
		inner.Pop()
		ctes := topLevel.Expressions()
		if parentCTE != nil {
			var merged []exp.Expression
			for _, cte := range ctes {
				if cte == parentCTE {
					merged = append(merged, inner.Expressions()...)
				}
				merged = append(merged, cte)
			}
			topLevel.Set("expressions", merged)
		} else {
			topLevel.Set("expressions", append(ctes, inner.Expressions()...))
		}
	}
	return e
}

// UnnestGenerateSeries ports unnest_generate_series (transforms.py:131-141).
func UnnestGenerateSeries(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindTable {
		return e
	}
	this := e.This()
	if this == nil || this.Kind() != exp.KindGenerateSeries {
		return e
	}
	unnest := exp.New(exp.KindUnnest, exp.Args{"expressions": []exp.Expression{this}})
	if alias := e.Alias(); alias != "" {
		unnest.Set("alias", exp.New(exp.KindTableAlias, exp.Args{
			"this":    exp.ToIdentifier("_u"),
			"columns": []exp.Expression{exp.ToIdentifier(alias)},
		}))
	}
	return unnest
}

func listArg(e exp.Expression, key string) []exp.Expression {
	switch v := e.Arg(key).(type) {
	case []exp.Expression:
		return v
	case []any:
		var out []exp.Expression
		for _, item := range v {
			if x, ok := item.(exp.Expression); ok && x != nil {
				out = append(out, x)
			}
		}
		return out
	}
	return nil
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return strings.TrimSpace(x) != ""
	case exp.Expression:
		return x != nil
	}
	return true
}

// RemoveWithinGroupForPercentiles ports remove_within_group_for_percentiles (transforms.py:570-581).
func RemoveWithinGroupForPercentiles(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindWithinGroup {
		return e
	}
	this, order := e.This(), e.Expr()
	if this == nil || (this.Kind() != exp.KindPercentileCont && this.Kind() != exp.KindPercentileDisc) || order == nil || order.Kind() != exp.KindOrder {
		return e
	}
	ordered := e.Find(exp.KindOrdered)
	if ordered == nil {
		return e
	}
	return e.Replace(exp.New(exp.KindApproxQuantile, exp.Args{"this": ordered.This(), "quantile": this.This()}))
}

// EpochCastToTs ports epoch_cast_to_ts (transforms.py:603-612).
func EpochCastToTs(e exp.Expression) exp.Expression {
	if (e.Kind() == exp.KindCast || e.Kind() == exp.KindTryCast) && strings.EqualFold(e.Name(), "epoch") {
		if to := asExpression(e.Arg("to")); to != nil {
			if d, ok := to.Arg("this").(exp.DType); !ok || !exp.TemporalTypes[d] {
				return e
			}
			e.This().Replace(exp.LiteralString("1970-01-01 00:00:00"))
		}
	}
	return e
}

// EliminateSemiAndAntiJoins ports eliminate_semi_and_anti_joins (transforms.py:615-629).
func EliminateSemiAndAntiJoins(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	for _, join := range listArg(e, "joins") {
		on := asExpression(join.Arg("on"))
		kind := strings.ToUpper(join.Text("kind"))
		if on == nil || (kind != "SEMI" && kind != "ANTI") {
			continue
		}
		sub := exp.Select(exp.Args{
			"expressions": []exp.Expression{exp.LiteralNumber(1)},
			"from_":       exp.New(exp.KindFrom, exp.Args{"this": join.This()}),
			"where":       exp.New(exp.KindWhere, exp.Args{"this": on}),
		})
		var exists exp.Expression = exp.Exists(exp.Args{"this": sub})
		if kind == "ANTI" {
			exists = exp.Not(exp.Args{"this": exists})
		}
		join.Pop()
		andWhere(e, exists)
	}
	return e
}

func andWhere(e exp.Expression, cond exp.Expression) {
	if where := asExpression(e.Arg("where")); where != nil && where.This() != nil {
		where.Set("this", exp.And_(where.This(), cond))
		return
	}
	e.Set("where", exp.New(exp.KindWhere, exp.Args{"this": cond}))
}

func asExpression(v any) exp.Expression {
	if x, ok := v.(exp.Expression); ok && x != nil {
		return x
	}
	return nil
}

// EliminateFullOuterJoin ports eliminate_full_outer_join (transforms.py:632-669): a single FULL
// OUTER join becomes LEFT JOIN ... UNION ALL ... RIGHT JOIN ... WHERE NOT EXISTS(left match).
func EliminateFullOuterJoin(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	fullIndex, count := -1, 0
	var fullJoin exp.Expression
	for i, join := range listArg(e, "joins") {
		if strings.EqualFold(join.Text("side"), "FULL") {
			fullIndex, fullJoin = i, join
			count++
		}
	}
	if count != 1 {
		return e
	}
	expressionCopy := e.Copy()
	e.Set("limit", nil)
	from := asExpression(e.Arg("from_"))
	tables := [2]string{from.This().AliasOrName(), fullJoin.This().AliasOrName()}
	joinConditions := asExpression(fullJoin.Arg("on"))
	if joinConditions == nil {
		var conds []exp.Expression
		for _, col := range listArg(fullJoin, "using") {
			conds = append(conds, exp.EQ(exp.Args{
				"this":       exp.Column_(col.Name(), tables[0], nil, nil, nil),
				"expression": exp.Column_(col.Name(), tables[1], nil, nil, nil),
			}))
		}
		joinConditions = exp.And_(conds...)
	}
	fullJoin.Set("side", "LEFT")
	antiJoin := selectFromWhere([]exp.Expression{exp.LiteralNumber(1)}, from.This().Copy(), joinConditions.Copy())
	listArg(expressionCopy, "joins")[fullIndex].Set("side", "RIGHT")
	andWhere(expressionCopy, exp.Not(exp.Args{"this": exp.Exists(exp.Args{"this": antiJoin})}))
	expressionCopy.Set("with_", nil)
	e.Set("order", nil)
	return exp.Union(exp.Args{"this": e, "expression": expressionCopy, "distinct": false})
}

// UnnestGenerateDateArrayUsingRecursiveCte ports
// unnest_generate_date_array_using_recursive_cte (transforms.py:72-128).
func UnnestGenerateDateArrayUsingRecursiveCte(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect {
		return e
	}
	castDate := func(x exp.Expression) exp.Expression {
		// builders.py cast(): an existing cast to the same type is kept as is.
		if x.Kind() == exp.KindCast && exp.IsType(asExpression(x.Arg("to")), exp.DTypeDate) {
			return x
		}
		return exp.New(exp.KindCast, exp.Args{"this": x, "to": exp.DTypeDate.IntoExpr(nil)})
	}
	count := 0
	var recursiveCtes []exp.Expression
	for _, unnest := range e.FindAll(exp.KindUnnest) {
		parent := unnest.Parent()
		exprs := unnest.Expressions()
		if parent == nil || (parent.Kind() != exp.KindFrom && parent.Kind() != exp.KindJoin) || len(exprs) != 1 || exprs[0].Kind() != exp.KindGenerateDateArray {
			continue
		}
		gda := exprs[0]
		start, end, step := asExpression(gda.Arg("start")), asExpression(gda.Arg("end")), asExpression(gda.Arg("step"))
		if start == nil || end == nil || step == nil || step.Kind() != exp.KindInterval {
			continue
		}
		columnName := "date_value"
		if alias := asExpression(unnest.Arg("alias")); alias != nil && alias.Kind() == exp.KindTableAlias {
			if cols := listArg(alias, "columns"); len(cols) > 0 {
				columnName = cols[0].Name()
			}
		}
		dateAdd := exp.New(exp.KindDateAdd, exp.Args{"this": column(columnName, nil), "expression": exp.LiteralNumber(step.Name()), "unit": asExpression(step.Arg("unit")).Copy()})
		castDateAdd := castDate(dateAdd)
		cteName := "_generated_dates"
		if count > 0 {
			cteName = fmt.Sprintf("%s_%d", cteName, count)
		}
		baseQuery := exp.Select(exp.Args{"expressions": []exp.Expression{exp.AliasExpr(castDate(start.Copy()), columnName, false)}})
		recursiveQuery := selectFromWhere(
			[]exp.Expression{castDateAdd},
			exp.Table_(cteName, nil, nil, nil, nil),
			exp.New(exp.KindLTE, exp.Args{"this": castDateAdd.Copy(), "expression": castDate(end.Copy())}),
		)
		cteQuery := exp.Union(exp.Args{"this": baseQuery, "expression": recursiveQuery, "distinct": false})
		generateDates := exp.Select(exp.Args{
			"expressions": []exp.Expression{column(columnName, nil)},
			"from_":       exp.New(exp.KindFrom, exp.Args{"this": exp.Table_(cteName, nil, nil, nil, nil)}),
		})
		unnest.Replace(subquery(generateDates, cteName))
		recursiveCtes = append(recursiveCtes, exp.CTE(exp.Args{
			"this":  cteQuery,
			"alias": exp.New(exp.KindTableAlias, exp.Args{"this": exp.ToIdentifier(cteName), "columns": []exp.Expression{exp.ToIdentifier(columnName)}}),
		}))
		count++
	}
	if len(recursiveCtes) > 0 {
		with := asExpression(e.Arg("with_"))
		if with == nil {
			with = exp.With(nil)
		}
		with.Set("recursive", true)
		with.Set("expressions", append(recursiveCtes, with.Expressions()...))
		e.Set("with_", with)
	}
	return e
}

// AddWithinGroupForPercentiles ports add_within_group_for_percentiles (transforms.py:555-567).
func AddWithinGroupForPercentiles(e exp.Expression) exp.Expression {
	if (e.Kind() != exp.KindPercentileCont && e.Kind() != exp.KindPercentileDisc) || e.Expr() == nil {
		return e
	}
	if p := e.Parent(); p != nil && p.Kind() == exp.KindWithinGroup {
		return e
	}
	column := e.This().Pop()
	e.Set("this", e.Expr().Pop())
	order := exp.Order(exp.Args{"expressions": []exp.Expression{exp.Ordered(exp.Args{"this": column})}})
	return exp.New(exp.KindWithinGroup, exp.Args{"this": e, "expression": order})
}

// UnqualifyColumns ports unqualify_columns (transforms.py:732-738).
func UnqualifyColumns(e exp.Expression) exp.Expression {
	for _, col := range e.FindAll(exp.KindColumn) {
		for _, key := range []string{"table", "db", "catalog"} {
			if part := asExpression(col.Arg(key)); part != nil {
				part.Pop()
			}
		}
	}
	return e
}

// UnqualifyPivotFields ports unqualify_pivot_fields (transforms.py:741-755).
func UnqualifyPivotFields(e exp.Expression) exp.Expression {
	if e.Kind() == exp.KindPivot {
		fields := listArg(e, "fields")
		for i, field := range fields {
			fields[i] = UnqualifyColumns(field)
		}
		e.Set("fields", fields)
	}
	return e
}

// RemoveUniqueConstraints ports remove_unique_constraints (transforms.py:758-764).
func RemoveUniqueConstraints(e exp.Expression) exp.Expression {
	for _, constraint := range e.FindAll(exp.KindUniqueColumnConstraint) {
		if constraint.Parent() != nil {
			constraint.Parent().Pop()
		}
	}
	return e
}

// CtasWithTmpTablesToCreateTmpView ports ctas_with_tmp_tables_to_create_tmp_view
// (transforms.py:767-788); tmpStorageProvider may be nil.
func CtasWithTmpTablesToCreateTmpView(e exp.Expression, tmpStorageProvider func(exp.Expression) exp.Expression) exp.Expression {
	temporary := false
	if props := asExpression(e.Arg("properties")); props != nil {
		for _, prop := range props.Expressions() {
			if prop.Kind() == exp.KindTemporaryProperty {
				temporary = true
			}
		}
	}
	if e.Text("kind") == "TABLE" && temporary {
		if expression := asExpression(e.Arg("expression")); expression != nil {
			return exp.New(exp.KindCreate, exp.Args{"kind": "TEMPORARY VIEW", "this": e.This(), "expression": expression})
		}
		if tmpStorageProvider != nil {
			return tmpStorageProvider(e)
		}
	}
	return e
}

// MoveSchemaColumnsToPartitionedBy ports move_schema_columns_to_partitioned_by
// (transforms.py:791-811).
func MoveSchemaColumnsToPartitionedBy(e exp.Expression) exp.Expression {
	schema := e.This()
	kind := e.Text("kind")
	if schema == nil || schema.Kind() != exp.KindSchema || (kind != "TABLE" && kind != "VIEW") {
		return e
	}
	prop := e.Find(exp.KindPartitionedByProperty)
	if prop == nil || prop.This() == nil || prop.This().Kind() == exp.KindSchema {
		return e
	}
	columns := map[string]bool{}
	for _, v := range prop.This().Expressions() {
		columns[strings.ToUpper(v.Name())] = true
	}
	var partitions, kept []exp.Expression
	for _, col := range schema.Expressions() {
		if columns[strings.ToUpper(col.Name())] {
			partitions = append(partitions, col)
		} else {
			kept = append(kept, col)
		}
	}
	schema.Set("expressions", kept)
	prop.Replace(exp.New(exp.KindPartitionedByProperty, exp.Args{"this": exp.Schema(exp.Args{"expressions": partitions})}))
	e.Set("this", schema)
	return e
}

// MovePartitionedByToSchemaColumns ports move_partitioned_by_to_schema_columns
// (transforms.py:814-835).
func MovePartitionedByToSchemaColumns(e exp.Expression) exp.Expression {
	prop := e.Find(exp.KindPartitionedByProperty)
	if prop == nil || prop.This() == nil || prop.This().Kind() != exp.KindSchema {
		return e
	}
	cols := prop.This().Expressions()
	var names []exp.Expression
	for _, col := range cols {
		if col.Kind() != exp.KindColumnDef || !truthy(col.Arg("kind")) {
			return e
		}
		names = append(names, exp.ToIdentifier(col.This()))
	}
	schema := e.This()
	if schema == nil {
		return e
	}
	schema.Set("expressions", append(schema.Expressions(), cols...))
	prop.Set("this", exp.Tuple(exp.Args{"expressions": names}))
	return e
}
