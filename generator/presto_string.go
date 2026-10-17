package generator

import (
	"regexp"
	"strings"

	"github.com/sevbanbayrak/sqlglot-go/expressions"
)

// escapedUnicodeRE ports ESCAPED_UNICODE_RE (generator.py:28).
var escapedUnicodeRE = regexp.MustCompile(`\\(\d+)`)

// unicodeStringSQL ports unicodestring_sql (generator.py:1653-1682): dialects without
// UNICODE_START rewrite the escapes to \u and render an ordinary string.
func (g *Generator) unicodeStringSQL(e expressions.Expression) string {
	this := e.Name()
	escape := asExpression(e.Arg("escape"))
	unicodeStart := g.dialect.UnicodeStart != ""
	escapeSubstitute := `\u$1`
	leftQuote, rightQuote := g.quoteStart, g.quoteEnd
	if unicodeStart {
		escapeSubstitute = `\$1`
		leftQuote, rightQuote = g.dialect.UnicodeStart, "'"
	}
	supportsUescape := !g.dialect.UescapeUnsupported
	escapePattern := escapedUnicodeRE
	escapeSQL := ""
	if escape != nil {
		escapePattern = regexp.MustCompile(regexp.QuoteMeta(escape.Name()) + `(\d+)`)
		if supportsUescape {
			escapeSQL = " UESCAPE " + g.gen(escape)
		}
	}
	if !unicodeStart || (escape != nil && !supportsUescape) {
		this = escapePattern.ReplaceAllString(this, escapeSubstitute)
	}
	if unicodeStart {
		// A Unicode literal only escapes its delimiter by doubling it; the escape character
		// introduces a code point, so the dialect's ordinary string escapes don't apply here
		this = strings.ReplaceAll(g.replaceLineBreaks(this), rightQuote, rightQuote+rightQuote)
	} else {
		this = g.escapeStrOpts(this, escapeStrOptions{})
	}
	return leftQuote + this + rightQuote + escapeSQL
}

// structSQL ports struct_sql (generator.py:5267-5278): PropertyEQ fields render as aliases.
func (g *Generator) structSQL(e expressions.Expression) string {
	args := []any{}
	for _, field := range e.Expressions() {
		if field.Kind() == expressions.KindPropertyEQ {
			var alias any = field.This()
			if field.This().IsString() {
				alias = field.Name()
			}
			field = expressions.AliasExpr(field.Arg("expression"), alias, false)
		}
		args = append(args, field)
	}
	return g.funcCall(g.sqlName(e.Kind()), args, "(", ")", true)
}

// versionSQL ports version_sql (generator.py:2649-2653).
func (g *Generator) versionSQL(e expressions.Expression) string {
	sql := "FOR " + e.Name() + " " + e.Text("kind")
	if expression := g.sqlKey(e, "expression"); expression != "" {
		sql += " " + expression
	}
	return sql
}

func init() {
	dispatch[expressions.KindUnicodeString] = (*Generator).unicodeStringSQL
	dispatch[expressions.KindVersion] = (*Generator).versionSQL
	dispatch[expressions.KindStruct] = (*Generator).structSQL
}
