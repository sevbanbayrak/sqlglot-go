package sqlglot_test

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	sqlglot "github.com/sevbanbayrak/sqlglot-go"
	sqlerrors "github.com/sevbanbayrak/sqlglot-go/errors"
	"github.com/sevbanbayrak/sqlglot-go/generator"
)

// transpileRecord is one cross-dialect case from upstream Validator.validate_all: parse Sql under
// Read, generate under Write, expect Want. Unsupported mirrors a write expecting UnsupportedError
// (generated with unsupported_level=RAISE). Produced by scripts/extract_transpile_corpus.py.
type transpileRecord struct {
	Read        string `json:"read"`
	Sql         string `json:"sql"`
	Write       string `json:"write"`
	Want        string `json:"want"`
	Pretty      bool   `json:"pretty"`
	Identify    bool   `json:"identify,omitempty"`
	Unsupported bool   `json:"unsupported,omitempty"`
}

type transpileGapKey struct {
	Read, Write, Sql string
}

type transpileGapRecord struct {
	Read     string `json:"read"`
	Write    string `json:"write"`
	Category string `json:"category"`
	Sql      string `json:"sql"`
}

const (
	transpileCasesPath = "testdata/transpile_cases.jsonl"
	transpileGapsPath  = "testdata/transpile_gaps.txt"
)

// Monotonic floors over the whole corpus: raise them, never lower them.
const (
	minTranspilePass  = 2611
	minTranspileTotal = 2693
)

func loadTranspileCorpus(t *testing.T) []transpileRecord {
	t.Helper()
	data, err := os.ReadFile(transpileCasesPath)
	if err != nil {
		t.Fatalf("read %s: %v", transpileCasesPath, err)
	}
	var records []transpileRecord
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var rec transpileRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("parse %s line %q: %v", transpileCasesPath, line, err)
		}
		records = append(records, rec)
	}
	return records
}

func loadTranspileGaps(t *testing.T) map[transpileGapKey]bool {
	t.Helper()
	gaps := map[transpileGapKey]bool{}
	data, err := os.ReadFile(transpileGapsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return gaps
		}
		t.Fatalf("read %s: %v", transpileGapsPath, err)
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var rec transpileGapRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("parse %s line %q: %v", transpileGapsPath, line, err)
		}
		gaps[transpileGapKey{rec.Read, rec.Write, rec.Sql}] = true
	}
	return gaps
}

func writeTranspileGaps(fails map[transpileGapKey]string) error {
	entries := make([]transpileGapRecord, 0, len(fails))
	for k, category := range fails {
		entries = append(entries, transpileGapRecord{Read: k.Read, Write: k.Write, Category: category, Sql: k.Sql})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Read != b.Read {
			return a.Read < b.Read
		}
		if a.Write != b.Write {
			return a.Write < b.Write
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Sql < b.Sql
	})
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return os.WriteFile(transpileGapsPath, []byte(b.String()), 0o644)
}

// transpileOne runs one record and reports whether it passes plus a category for the gap file.
func transpileOne(rec transpileRecord) (ok bool, category string) {
	level := sqlerrors.IGNORE
	if rec.Unsupported {
		level = sqlerrors.RAISE
	}
	opts := generator.Options{Pretty: rec.Pretty, UnsupportedLevel: &level}
	if rec.Identify {
		opts.Identify = true
	}
	expression, err := sqlglot.ParseOne(rec.Sql, rec.Read)
	if err != nil {
		return false, "parse: " + firstPhrase(err.Error())
	}
	got, err := sqlglot.Generate(expression, rec.Write, opts)
	if rec.Unsupported {
		if _, isUnsupported := err.(*sqlerrors.UnsupportedError); isUnsupported {
			return true, ""
		}
		return false, "generate: expected unsupported"
	}
	if err != nil {
		return false, "generate: error"
	}
	if got != rec.Want {
		return false, "generate: mismatch"
	}
	return true, ""
}

// TestTranspileCorpus is the cross-dialect counterpart of TestCorpus: every failing case must be
// listed in testdata/transpile_gaps.txt, the pass count may only rise and the corpus may only
// grow. SQLGLOT_CORPUS_UPDATE=1 rewrites the gap file and prints the counts to set the floors.
func TestTranspileCorpus(t *testing.T) {
	records := loadTranspileCorpus(t)
	pass := 0
	fails := map[transpileGapKey]string{}
	pairPass, pairTotal := map[string]int{}, map[string]int{}
	for _, rec := range records {
		pair := name(rec.Read) + "->" + name(rec.Write)
		pairTotal[pair]++
		ok, category := transpileOne(rec)
		if ok {
			pass++
			pairPass[pair]++
			continue
		}
		fails[transpileGapKey{rec.Read, rec.Write, rec.Sql}] = category
	}

	if os.Getenv("SQLGLOT_CORPUS_UPDATE") != "" {
		if err := writeTranspileGaps(fails); err != nil {
			t.Fatalf("write %s: %v", transpileGapsPath, err)
		}
		pairs := make([]string, 0, len(pairTotal))
		for p := range pairTotal {
			pairs = append(pairs, p)
		}
		sort.Strings(pairs)
		fmt.Fprintf(os.Stderr, "updated %s: pass=%d/%d\n", transpileGapsPath, pass, len(records))
		for _, p := range pairs {
			fmt.Fprintf(os.Stderr, "  %s: %d/%d\n", p, pairPass[p], pairTotal[p])
		}
		return
	}

	known := loadTranspileGaps(t)
	for k, category := range fails {
		if !known[k] {
			t.Errorf("untracked transpile failure [%s] %s -> %s: %s", category, name(k.Read), name(k.Write), k.Sql)
		}
	}
	if pass < minTranspilePass {
		t.Errorf("transpile pass count %d fell below floor %d", pass, minTranspilePass)
	}
	if len(records) < minTranspileTotal {
		t.Errorf("transpile corpus size %d fell below floor %d", len(records), minTranspileTotal)
	}
	t.Logf("transpile corpus: pass=%d/%d", pass, len(records))
}

func name(dialect string) string {
	if dialect == "" {
		return "base"
	}
	return dialect
}
