package transforms

import (
	"strings"

	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/optimizer"
)

// EliminateWindowClause ports eliminate_window_clause (transforms.py:1001-1029): the WINDOW
// query clause is removed by inlining each named window.
func EliminateWindowClause(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindSelect || !exp.HasArg(e, "windows") || e.Arg("windows") == nil {
		return e
	}
	windows := listArg(e, "windows")
	e.Set("windows", nil)

	windowExpression := map[string]exp.Expression{}
	inlineInheritedWindow := func(window exp.Expression) {
		inherited := windowExpression[strings.ToLower(window.Alias())]
		if inherited == nil {
			return
		}
		window.Set("alias", nil)
		for _, key := range []string{"partition_by", "order", "spec"} {
			switch arg := inherited.Arg(key).(type) {
			case exp.Expression:
				if arg != nil {
					window.Set(key, arg.Copy())
				}
			case []exp.Expression:
				copied := make([]exp.Expression, len(arg))
				for i, item := range arg {
					copied[i] = item.Copy()
				}
				window.Set(key, copied)
			}
		}
	}
	for _, window := range windows {
		inlineInheritedWindow(window)
		windowExpression[strings.ToLower(window.Name())] = window
	}
	for _, window := range optimizer.FindAllInScope(e, exp.KindWindow) {
		inlineInheritedWindow(window)
	}
	return e
}

// InheritStructFieldNames ports inherit_struct_field_names (transforms.py:1032-1084): the field
// names of the first struct in an array are made explicit on the following unnamed structs.
func InheritStructFieldNames(e exp.Expression) exp.Expression {
	if e.Kind() != exp.KindArray || !truthy(e.Arg("struct_name_inheritance")) {
		return e
	}
	items := e.Expressions()
	if len(items) == 0 || items[0].Kind() != exp.KindStruct {
		return e
	}
	var fieldNames []exp.Expression
	for _, field := range items[0].Expressions() {
		if field.Kind() != exp.KindPropertyEQ {
			return e
		}
		fieldNames = append(fieldNames, field.This())
	}
	for _, structNode := range items[1:] {
		if structNode.Kind() != exp.KindStruct || len(structNode.Expressions()) != len(fieldNames) {
			continue
		}
		newExpressions := []exp.Expression{}
		for i, expr := range structNode.Expressions() {
			if expr.Kind() != exp.KindPropertyEQ {
				// Create PropertyEQ: field_name := value, preserving the type from the inner expression
				propertyEQ := exp.PropertyEQ(exp.Args{"this": fieldNames[i].Copy(), "expression": expr})
				propertyEQ.SetType(expr.Type())
				newExpressions = append(newExpressions, propertyEQ)
			} else {
				newExpressions = append(newExpressions, expr)
			}
		}
		structNode.Set("expressions", newExpressions)
	}
	return e
}

var explodeKinds = exp.AnyKind{exp.KindExplode, exp.KindPosexplode, exp.Kind_ExplodeOuter, exp.KindPosexplodeOuter}

func qualifiedColumn(name, table string) exp.Expression {
	return exp.Column(exp.Args{"this": exp.ToIdentifier(name), "table": exp.ToIdentifier(table)})
}

// ExplodeProjectionToUnnest ports explode_projection_to_unnest (transforms.py:402-552): explode
// and posexplode projections become unnests.
func ExplodeProjectionToUnnest(indexOffset int) func(exp.Expression) exp.Expression {
	return func(e exp.Expression) exp.Expression {
		if e.Kind() != exp.KindSelect {
			return e
		}
		takenSelectNames := namedSelects(e)
		takenSourceNames := map[string]bool{}
		for _, ref := range optimizer.NewScope(e).References() {
			takenSourceNames[ref.Name] = true
		}
		newName := func(names map[string]bool, name string) string {
			name = findNewName(names, name)
			names[name] = true
			return name
		}

		var arrays []exp.Expression
		seriesAlias := newName(takenSelectNames, "pos")
		seriesTableAlias := newName(takenSourceNames, "_u")
		series := exp.New(exp.KindUnnest, exp.Args{
			"expressions": []exp.Expression{exp.New(exp.KindGenerateSeries, exp.Args{"start": exp.LiteralNumber(indexOffset)})},
			"alias":       exp.TableAlias(exp.Args{"this": exp.ToIdentifier(seriesTableAlias), "columns": []exp.Expression{exp.ToIdentifier(seriesAlias)}}),
		})

		// we use a copy here because expression.selects is mutated inside the loop
		for _, sel := range append([]exp.Expression(nil), e.Expressions()...) {
			explode := sel.Find(explodeKinds)
			if explode == nil {
				continue
			}
			var posAlias, explodeAlias exp.Expression
			var alias exp.Expression
			switch sel.Kind() {
			case exp.KindAlias:
				explodeAlias = asExpression(sel.Arg("alias"))
				alias = sel
			case exp.KindAliases:
				aliases := listArg(sel, "expressions")
				posAlias, explodeAlias = aliases[0], aliases[1]
				alias = sel.Replace(exp.AliasNode(exp.Args{"this": sel.This(), "alias": exp.ToIdentifier("")}))
			default:
				alias = sel.Replace(exp.AliasNode(exp.Args{"this": sel.Copy(), "alias": exp.ToIdentifier("")}))
				explode = alias.Find(explodeKinds)
			}

			isPosexplode := explode.Kind() == exp.KindPosexplode || explode.Kind() == exp.KindPosexplodeOuter
			explodeArg := explode.This()

			if explode.Kind() == exp.Kind_ExplodeOuter || explode.Kind() == exp.KindPosexplodeOuter {
				bracket := exp.New(exp.KindBracket, exp.Args{"this": explodeArg.Copy(), "expressions": []exp.Expression{exp.LiteralNumber(0)}})
				bracket.Set("safe", true)
				bracket.Set("offset", true)
				size := exp.Anonymous(exp.Args{"this": "ARRAY_SIZE", "expressions": []exp.Expression{
					exp.Anonymous(exp.Args{"this": "COALESCE", "expressions": []exp.Expression{explodeArg.Copy(), exp.Array(nil)}}),
				}})
				explodeArg = exp.Anonymous(exp.Args{"this": "IF", "expressions": []exp.Expression{
					exp.BinOp(exp.KindEQ, size, 0),
					exp.Array(exp.Args{"expressions": []exp.Expression{bracket}}),
					explodeArg,
				}})
			}

			// This ensures that we won't use [POS]EXPLODE's argument as a new selection
			if explodeArg.Kind() == exp.KindColumn {
				takenSelectNames[explodeArg.OutputName()] = true
			}

			unnestSourceAlias := newName(takenSourceNames, "_u")

			if explodeAlias == nil || explodeAlias.Name() == "" {
				explodeAlias = exp.ToIdentifier(newName(takenSelectNames, "col"))
				if isPosexplode {
					posAlias = exp.ToIdentifier(newName(takenSelectNames, "pos"))
				}
			}
			if posAlias == nil || posAlias.Name() == "" {
				posAlias = exp.ToIdentifier(newName(takenSelectNames, "pos"))
			}

			alias.Set("alias", exp.ToIdentifier(explodeAlias.Name()))

			column := exp.If(exp.Args{
				"this": exp.BinOp(exp.KindEQ, qualifiedColumn(seriesAlias, seriesTableAlias), qualifiedColumn(posAlias.Name(), unnestSourceAlias)),
				"true": qualifiedColumn(explodeAlias.Name(), unnestSourceAlias),
			})
			explode.Replace(column)

			if isPosexplode {
				expressions := e.Expressions()
				index := 0
				for i, item := range expressions {
					if item == alias {
						index = i
						break
					}
				}
				posColumn := exp.AliasNode(exp.Args{
					"this": exp.If(exp.Args{
						"this": exp.BinOp(exp.KindEQ, qualifiedColumn(seriesAlias, seriesTableAlias), qualifiedColumn(posAlias.Name(), unnestSourceAlias)),
						"true": qualifiedColumn(posAlias.Name(), unnestSourceAlias),
					}),
					"alias": exp.ToIdentifier(posAlias.Name()),
				})
				updated := append(append(append([]exp.Expression(nil), expressions[:index+1]...), posColumn), expressions[index+1:]...)
				e.Set("expressions", updated)
			}

			if len(arrays) == 0 {
				if e.Arg("from_") != nil {
					appendJoin(e, exp.Join(exp.Args{"this": series, "kind": "CROSS"}))
				} else {
					e.Set("from_", exp.New(exp.KindFrom, exp.Args{"this": series}))
				}
			}

			var size exp.Expression = exp.New(exp.KindArraySize, exp.Args{"this": explodeArg.Copy()})
			arrays = append(arrays, size)

			// trino doesn't support left join unnest with on conditions
			// if it did, this would be much simpler
			appendJoin(e, exp.Join(exp.Args{
				"this": exp.New(exp.KindUnnest, exp.Args{
					"expressions": []exp.Expression{explodeArg.Copy()},
					"offset":      exp.ToIdentifier(posAlias.Name()),
					"alias":       exp.TableAlias(exp.Args{"this": exp.ToIdentifier(unnestSourceAlias), "columns": []exp.Expression{exp.ToIdentifier(explodeAlias.Name())}}),
				}),
				"kind": "CROSS",
			}))

			if indexOffset != 1 {
				size = exp.BinOp(exp.KindSub, size, 1)
			}

			andWhere(e, exp.Or_(
				exp.BinOp(exp.KindEQ, qualifiedColumn(seriesAlias, seriesTableAlias), qualifiedColumn(posAlias.Name(), unnestSourceAlias)),
				exp.And_(
					exp.BinOp(exp.KindGT, qualifiedColumn(seriesAlias, seriesTableAlias), size),
					exp.BinOp(exp.KindEQ, qualifiedColumn(posAlias.Name(), unnestSourceAlias), size),
				),
			))
		}

		if len(arrays) > 0 {
			var end exp.Expression = exp.New(exp.KindGreatest, exp.Args{"this": arrays[0], "expressions": arrays[1:]})
			if indexOffset != 1 {
				end = exp.BinOp(exp.KindSub, end, 1-indexOffset)
			}
			series.Expressions()[0].Set("end", end)
		}
		return e
	}
}

func appendJoin(e exp.Expression, join exp.Expression) {
	e.Set("joins", append(listArg(e, "joins"), join))
}
