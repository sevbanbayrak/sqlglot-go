package generator

import (
	"github.com/sevbanbayrak/sqlglot-go/expressions"
)

// Postgres overrides follow generators/postgres.py TRANSFORMS; the shared tables already carry
// the postgres branches written before per-dialect dispatch existed.
func init() {
	registerDialectDispatch("postgres", map[expressions.Kind]func(*Generator, expressions.Expression) string{
		// generators/postgres.py:408
		expressions.KindUuid: func(*Generator, expressions.Expression) string { return "GEN_RANDOM_UUID()" },
		// dialect.py:2626-2644 getbit_sql; the integer-typed branch needs annotate_types, which
		// never runs here, so only the GET_BIT spelling is reached.
		expressions.KindGetbit: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("GET_BIT", []any{e.This(), e.Expr()}, "(", ")", true)
		},
	})
}
