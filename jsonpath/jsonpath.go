// Package jsonpath ports sqlglot/jsonpath.py: the JSONPath tokenizer and parse.
package jsonpath

import (
	"fmt"
	"strconv"

	sqlerrors "github.com/sevbanbayrak/sqlglot-go/errors"
	exp "github.com/sevbanbayrak/sqlglot-go/expressions"
	"github.com/sevbanbayrak/sqlglot-go/tokens"
)

// Options carries the dialect state parse reads (jsonpath.py:45-58): the jsonpath tokenizer's
// VAR_TOKENS, JSON_PATH_SINGLE_DOT_IS_WILDCARD, and the dialect settings Tokenizer.__init__
// forwards to the core (tokens.py:569-572). The zero value is the base dialect.
type Options struct {
	VarTokens                       map[tokens.TokenType]bool
	SingleDotIsWildcard             bool
	UnescapedSequences              map[string]string
	NumbersCanBeUnderscoreSeparated bool
	IdentifiersCanStartWithDigit    bool
	// Identifiers ports the dialect's JSONPathTokenizer.IDENTIFIERS; nil keeps the base `"`.
	Identifiers map[rune]string
	// NoDollarToken drops `$` from the JSONPathTokenizer's SINGLE_TOKENS
	// (dialects/snowflake.py:149-151).
	NoDollarToken bool
}

// tokenizerConfig ports JSONPathTokenizer (jsonpath.py:14-42) on top of the base Tokenizer.
var tokenizerConfig = tokens.CompileConfig(rawTokenizerConfig())

var noDollarTokenizerConfig = func() tokens.TokenizerConfig {
	cfg := rawTokenizerConfig()
	delete(cfg.SingleTokens, '$')
	return tokens.CompileConfig(cfg)
}()

func rawTokenizerConfig() tokens.TokenizerConfig {
	cfg := tokens.BaseConfig()
	cfg.SingleTokens = map[rune]tokens.TokenType{
		'(':  tokens.L_PAREN,
		')':  tokens.R_PAREN,
		'[':  tokens.L_BRACKET,
		']':  tokens.R_BRACKET,
		':':  tokens.COLON,
		',':  tokens.COMMA,
		'-':  tokens.DASH,
		'.':  tokens.DOT,
		'?':  tokens.PLACEHOLDER,
		'@':  tokens.PARAMETER,
		'\'': tokens.QUOTE,
		'"':  tokens.QUOTE,
		'$':  tokens.DOLLAR,
		'*':  tokens.STAR,
	}
	cfg.Keywords = map[string]tokens.TokenType{"..": tokens.DOT}
	cfg.IdentifierEscapes = map[rune]bool{'\\': true}
	cfg.StringEscapes = map[rune]bool{'\\': true}
	cfg.NumbersCanHaveDecimals = false
	return cfg
}

var defaultVarTokens = map[tokens.TokenType]bool{tokens.VAR: true}

type parser struct {
	path   []rune
	tokens []tokens.Token
	i      int
	opts   Options
}

// Parse ports parse (jsonpath.py:45-216): it takes in a JSON path string and parses it into a
// JSONPath expression. Errors are *sqlerrors.ParseError or the tokenizer's *sqlerrors.TokenError.
func Parse(path string, opts Options) (result exp.Expression, err error) {
	if opts.VarTokens == nil {
		opts.VarTokens = defaultVarTokens
	}
	cfg := tokenizerConfig
	if opts.NoDollarToken {
		cfg = noDollarTokenizerConfig
	}
	cfg.UnescapedSequences = opts.UnescapedSequences
	cfg.NumbersCanBeUnderscoreSeparated = opts.NumbersCanBeUnderscoreSeparated
	cfg.IdentifiersCanStartWithDigit = opts.IdentifiersCanStartWithDigit
	if opts.Identifiers != nil {
		cfg.Identifiers = opts.Identifiers
	}
	toks, err := tokens.NewTokenizerWithConfig(cfg).Tokenize(path)
	if err != nil {
		return nil, err
	}
	p := &parser{path: []rune(path), tokens: toks, opts: opts}
	defer func() {
		if r := recover(); r != nil {
			perr, ok := r.(*sqlerrors.ParseError)
			if !ok {
				panic(r)
			}
			result, err = nil, perr
		}
	}()
	return p.parse(), nil
}

func (p *parser) curr() tokens.TokenType {
	if p.i < len(p.tokens) {
		return p.tokens[p.i].TokenType
	}
	return 0
}

func (p *parser) prev() tokens.Token { return p.tokens[p.i-1] }

func (p *parser) advance() tokens.Token {
	p.i++
	return p.prev()
}

func (p *parser) error(msg string) {
	panic(sqlerrors.NewParseError(fmt.Sprintf("%s at index %d: %s", msg, p.i, string(p.path))))
}

func (p *parser) match(tokenType tokens.TokenType, raiseUnmatched bool) bool {
	if p.i < len(p.tokens) && p.curr() == tokenType {
		p.advance()
		return true
	}
	if raiseUnmatched {
		p.error("Expected " + tokenType.String())
	}
	return false
}

func (p *parser) matchSet(types map[tokens.TokenType]bool) bool {
	if p.i < len(p.tokens) && types[p.curr()] {
		p.advance()
		return true
	}
	return false
}

// parseLiteral returns a string, an int, a part expression, or false (jsonpath.py:86-118).
func (p *parser) parseLiteral() any {
	if p.match(tokens.STRING, false) || p.match(tokens.IDENTIFIER, false) {
		return p.prev().Text
	}
	if p.match(tokens.STAR, false) {
		return exp.New(exp.KindJSONPathWildcard, nil)
	}
	if p.match(tokens.PLACEHOLDER, false) || p.match(tokens.L_PAREN, false) {
		script := p.prev().Text == "("
		start := p.i

		for {
			if p.match(tokens.L_BRACKET, false) {
				p.parseBracket() // nested call which we can throw away
			}
			if p.i >= len(p.tokens) || p.curr() == tokens.R_BRACKET {
				break
			}
			p.advance()
		}

		kind := exp.KindJSONPathFilter
		if script {
			kind = exp.KindJSONPathScript
		}
		end := p.tokens[len(p.tokens)-1].End
		if p.i < len(p.tokens) {
			end = p.tokens[p.i].End
		}
		// divergence: upstream indexes tokens[start] unguarded (IndexError for `$[?`); clamp.
		from := p.tokens[len(p.tokens)-1].End + 1
		if start < len(p.tokens) {
			from = p.tokens[start].Start
		}
		return exp.New(kind, exp.Args{"this": p.slice(from, end)})
	}

	number := ""
	if p.match(tokens.DASH, false) {
		number = "-"
	}
	if p.match(tokens.NUMBER, false) {
		number += p.prev().Text
	}
	if number != "" {
		n, err := strconv.Atoi(number)
		if err != nil {
			// divergence: int() raises ValueError upstream (jsonpath.py:116); Go reports it as a
			// ParseError so callers keep a single error type.
			p.error("Invalid number " + number)
		}
		return n
	}
	return false
}

func (p *parser) slice(from, to int) string {
	from = min(max(from, 0), len(p.path))
	to = min(max(to, from), len(p.path))
	return string(p.path[from:to])
}

// parseSlice ports _parse_slice (jsonpath.py:120-129); nil stands for Python's None.
func (p *parser) parseSlice() any {
	start := p.parseLiteral()
	var end, step any
	if p.match(tokens.COLON, false) {
		end = p.parseLiteral()
	}
	if p.match(tokens.COLON, false) {
		step = p.parseLiteral()
	}
	if end == nil && step == nil {
		return start
	}
	args := exp.Args{}
	for key, value := range map[string]any{"start": start, "end": end, "step": step} {
		if value != nil {
			args[key] = value
		}
	}
	return exp.New(exp.KindJSONPathSlice, args)
}

// parseBracket ports _parse_bracket (jsonpath.py:131-158).
func (p *parser) parseBracket() exp.Expression {
	literal := p.parseSlice()

	var node exp.Expression
	if literal != any(false) {
		indexes := []any{literal}
		for p.match(tokens.COMMA, false) {
			literal = p.parseSlice()
			if literal != any(false) {
				indexes = append(indexes, literal)
			}
		}

		if len(indexes) == 1 {
			first := indexes[0]
			if s, ok := first.(string); ok {
				node = exp.New(exp.KindJSONPathKey, exp.Args{"this": s})
			} else if e, ok := first.(exp.Expression); ok && (e.Kind() == exp.KindJSONPathScript || e.Kind() == exp.KindJSONPathFilter) {
				node = exp.New(exp.KindJSONPathSelector, exp.Args{"this": e})
			} else {
				node = exp.New(exp.KindJSONPathSubscript, exp.Args{"this": first})
			}
		} else {
			node = exp.New(exp.KindJSONPathUnion, exp.Args{"expressions": indexes})
		}
	} else {
		p.error("Cannot have empty segment")
	}

	p.match(tokens.R_BRACKET, true)
	return node
}

// parseVarText ports _parse_var_text (jsonpath.py:160-182): consecutive vars are merged until
// a key separator is found or the path is exhausted, so '$. a b c ' yields the key ' a b c '.
func (p *parser) parseVarText() string {
	prevIndex := p.i - 2

	for p.matchSet(p.opts.VarTokens) {
	}

	start := 0
	if prevIndex >= 0 {
		start = p.tokens[prevIndex].End + 1
	}

	if p.i >= len(p.tokens) {
		// This key is the last token for the path, so its text is the remaining path
		return p.slice(start, len(p.path))
	}
	return p.slice(start, p.tokens[p.i].Start)
}

func (p *parser) parse() exp.Expression {
	// We canonicalize the JSON path AST so that it always starts with a
	// "root" element, so paths like "field" will be generated as "$.field"
	p.match(tokens.DOLLAR, false)
	expressions := []exp.Expression{exp.New(exp.KindJSONPathRoot, nil)}

	for p.i < len(p.tokens) {
		if p.match(tokens.DOT, false) || p.match(tokens.COLON, false) {
			recursive := p.prev().Text == ".."

			var value any
			if p.matchSet(p.opts.VarTokens) {
				value = p.parseVarText()
			} else if p.match(tokens.IDENTIFIER, false) {
				value = p.prev().Text
			} else if p.match(tokens.STAR, false) {
				value = exp.New(exp.KindJSONPathWildcard, nil)
			}

			if recursive {
				args := exp.Args{}
				if value != nil {
					args["this"] = value
				}
				expressions = append(expressions, exp.New(exp.KindJSONPathRecursive, args))
			} else if value != nil && value != any("") {
				expressions = append(expressions, exp.New(exp.KindJSONPathKey, exp.Args{"this": value}))
			} else if !p.opts.SingleDotIsWildcard {
				p.error("Expected key name or * after DOT")
			}
		} else if p.match(tokens.L_BRACKET, false) {
			expressions = append(expressions, p.parseBracket())
		} else if p.matchSet(p.opts.VarTokens) {
			expressions = append(expressions, exp.New(exp.KindJSONPathKey, exp.Args{"this": p.parseVarText()}))
		} else if p.match(tokens.IDENTIFIER, false) {
			expressions = append(expressions, exp.New(exp.KindJSONPathKey, exp.Args{"this": p.prev().Text}))
		} else if p.match(tokens.STAR, false) {
			expressions = append(expressions, exp.New(exp.KindJSONPathWildcard, nil))
		} else {
			p.error("Unexpected " + p.tokens[p.i].TokenType.String())
		}
	}

	return exp.New(exp.KindJSONPath, exp.Args{"expressions": expressions})
}
