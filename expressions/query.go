package expressions

func Select(args Args) Expression              { return newNode(KindSelect, args) }
func From(args Args) Expression                { return newNode(KindFrom, args) }
func Join(args Args) Expression                { return newNode(KindJoin, args) }
func Table(args Args) Expression               { return newNode(KindTable, args) }
func TableAlias(args Args) Expression          { return newNode(KindTableAlias, args) }
func Where(args Args) Expression               { return newNode(KindWhere, args) }
func Group(args Args) Expression               { return newNode(KindGroup, args) }
func Order(args Args) Expression               { return newNode(KindOrder, args) }
func Limit(args Args) Expression               { return newNode(KindLimit, args) }
func Offset(args Args) Expression              { return newNode(KindOffset, args) }
func Tuple(args Args) Expression               { return newNode(KindTuple, args) }
func Block(args Args) Expression               { return newNode(KindBlock, args) }
func WhileBlock(args Args) Expression          { return newNode(KindWhileBlock, args) }
func EndStatement(args Args) Expression        { return newNode(KindEndStatement, args) }
func Heredoc(args Args) Expression             { return newNode(KindHeredoc, args) }
func Semicolon(args Args) Expression           { return newNode(KindSemicolon, args) }
func IfBlock(args Args) Expression             { return newNode(KindIfBlock, args) }
func LoopBlock(args Args) Expression           { return newNode(KindLoopBlock, args) }
func RepeatBlock(args Args) Expression         { return newNode(KindRepeatBlock, args) }
func CaseBlock(args Args) Expression           { return newNode(KindCaseBlock, args) }
func Declare(args Args) Expression             { return newNode(KindDeclare, args) }
func DeclareItem(args Args) Expression         { return newNode(KindDeclareItem, args) }
func With(args Args) Expression                { return newNode(KindWith, args) }
func CTE(args Args) Expression                 { return newNode(KindCTE, args) }
func RecursiveWithSearch(args Args) Expression { return newNode(KindRecursiveWithSearch, args) }
func Union(args Args) Expression               { return newNode(KindUnion, args) }
func Except(args Args) Expression              { return newNode(KindExcept, args) }
func Intersect(args Args) Expression           { return newNode(KindIntersect, args) }
func Subquery(args Args) Expression            { return newNode(KindSubquery, args) }
func Having(args Args) Expression              { return newNode(KindHaving, args) }
func Qualify(args Args) Expression             { return newNode(KindQualify, args) }
func Connect(args Args) Expression             { return newNode(KindConnect, args) }
func Prior(args Args) Expression               { return newNode(KindPrior, args) }
func Cube(args Args) Expression                { return newNode(KindCube, args) }
func Rollup(args Args) Expression              { return newNode(KindRollup, args) }
func GroupingSets(args Args) Expression        { return newNode(KindGroupingSets, args) }
func Ordered(args Args) Expression             { return newNode(KindOrdered, args) }
func Distinct(args Args) Expression            { return newNode(KindDistinct, args) }
func Into(args Args) Expression                { return newNode(KindInto, args) }
func Window(args Args) Expression              { return newNode(KindWindow, args) }
func WindowSpec(args Args) Expression          { return newNode(KindWindowSpec, args) }
func Filter(args Args) Expression              { return newNode(KindFilter, args) }
func Lambda(args Args) Expression              { return newNode(KindLambda, args) }
func LimitOptions(args Args) Expression        { return newNode(KindLimitOptions, args) }
func Fetch(args Args) Expression               { return newNode(KindFetch, args) }
func Schema(args Args) Expression              { return newNode(KindSchema, args) }
func Lateral(args Args) Expression             { return newNode(KindLateral, args) }
func Pivot(args Args) Expression               { return newNode(KindPivot, args) }
func Values(args Args) Expression              { return newNode(KindValues, args) }
func ColumnDef(args Args) Expression           { return newNode(KindColumnDef, args) }
func Lock(args Args) Expression                { return newNode(KindLock, args) }
func PreWhere(args Args) Expression            { return newNode(KindPreWhere, args) }
func Cluster(args Args) Expression             { return newNode(KindCluster, args) }
func Distribute(args Args) Expression          { return newNode(KindDistribute, args) }
func Sort(args Args) Expression                { return newNode(KindSort, args) }
func WithinGroup(args Args) Expression         { return newNode(KindWithinGroup, args) }
func IgnoreNulls(args Args) Expression         { return newNode(KindIgnoreNulls, args) }
func RespectNulls(args Args) Expression        { return newNode(KindRespectNulls, args) }
func PivotAny(args Args) Expression            { return newNode(KindPivotAny, args) }
func PivotAlias(args Args) Expression          { return newNode(KindPivotAlias, args) }
func JoinHint(args Args) Expression            { return newNode(KindJoinHint, args) }
func TableColumn(args Args) Expression         { return newNode(KindTableColumn, args) }
func Final(args Args) Expression               { return newNode(KindFinal, args) }

// PivotOutputColumns ports Pivot.output_columns (query.py:1780-1848). Go has no ordered dict,
// so it returns the post-rename output names in order plus each one's pre-rename source name.
func PivotOutputColumns(pivot Expression, prePivotColumns []string) ([]string, map[string]string) {
	excluded := map[string]bool{}
	var outputs []string
	if truthy(pivot.Arg("unpivot")) {
		var nameColumns, valueColumns []Expression
		for _, field := range ExpressionsOf(pivot, "fields") {
			if field.Kind() != KindIn {
				continue
			}
			if this := field.This(); this != nil && this.Kind() == KindIdentifier {
				nameColumns = append(nameColumns, this)
			}
			for _, e := range field.Expressions() {
				for _, c := range e.FindAll(KindColumn) {
					excluded[c.OutputName()] = true
				}
			}
		}
		for _, e := range pivot.Expressions() {
			idents := []Expression{e}
			if e.Kind() == KindTuple {
				idents = e.Expressions()
			}
			for _, ident := range idents {
				if ident.Kind() == KindIdentifier {
					valueColumns = append(valueColumns, ident)
				}
			}
		}
		ordered := append(append([]Expression{}, nameColumns...), valueColumns...)
		if truthy(pivot.Arg("value_columns_first")) {
			ordered = append(append([]Expression{}, valueColumns...), nameColumns...)
		}
		for _, i := range ordered {
			outputs = append(outputs, i.Name())
		}
	} else {
		for _, c := range pivot.FindAll(KindColumn) {
			excluded[c.OutputName()] = true
		}
		for _, c := range ExpressionsOf(pivot, "columns") {
			outputs = append(outputs, c.OutputName())
		}
		if len(outputs) == 0 {
			for _, c := range pivot.Expressions() {
				outputs = append(outputs, c.AliasOrName())
			}
		}
	}

	if len(excluded) == 0 || len(outputs) == 0 {
		return nil, map[string]string{}
	}

	preRename := []string{}
	for _, c := range prePivotColumns {
		if !excluded[c] {
			preRename = append(preRename, c)
		}
	}
	preRename = append(preRename, outputs...)

	postRename := preRename
	if alias := asExpression(pivot.Arg("alias")); alias != nil {
		if renames := ExpressionsOf(alias, "columns"); len(renames) > 0 {
			postRename = []string{}
			for _, r := range renames {
				postRename = append(postRename, r.Name())
			}
			if len(postRename) < len(preRename) {
				postRename = append(postRename, preRename[len(postRename):]...)
			}
		}
	}

	names := []string{}
	sources := map[string]string{}
	for i := 0; i < len(postRename) && i < len(preRename); i++ {
		if _, seen := sources[postRename[i]]; !seen {
			names = append(names, postRename[i])
		}
		sources[postRename[i]] = preRename[i]
	}
	return names, sources
}
