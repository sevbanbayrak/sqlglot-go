package optimizer_test

import (
	sqlglot "github.com/sevbanbayrak/sqlglot-go"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/generator"
	"github.com/sevbanbayrak/sqlglot-go/optimizer"
)

func init() {
	optimizer.TestParseOne = func(sql, dialect string) (exp.Expression, error) { return sqlglot.ParseOne(sql, dialect) }
	optimizer.TestGenerate = func(expression exp.Expression, dialect string) (string, error) {
		return sqlglot.Generate(expression, dialect, generator.Options{})
	}
}
