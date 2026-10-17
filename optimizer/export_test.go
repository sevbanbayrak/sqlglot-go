package optimizer

import exp "github.com/sevbanbayrak/sqlglot-go/expressions"

// The internal scope tests cannot import the root or generator packages, which depend on this
// one through the Snowflake generator transforms; scope_hooks_test.go wires them in from the
// external test package.
var (
	TestParseOne func(sql, dialect string) (exp.Expression, error)
	TestGenerate func(expression exp.Expression, dialect string) (string, error)
)
