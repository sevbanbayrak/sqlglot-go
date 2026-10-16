#!/usr/bin/env python3
"""Extract the cross-dialect validate_all() corpus from upstream tobymao/sqlglot's dialect test
suite into testdata/transpile_cases.jsonl.

Validator.validate_all(sql, read={d: s}, write={d: s}) asserts two directions: every read pair
parses under its dialect and generates `sql` under the test dialect, and `sql` parsed under the
test dialect generates each write pair under its dialect. Like extract_dialect_corpus.py this
monkeypatches the helper and runs the test classes for real, so loop/f-string expansions are
captured too. Only pairs whose read AND write dialect are ported are kept; a write expecting
UnsupportedError is recorded with "unsupported": true.

Run manually (after scripts/fetch-reference.sh):

    PYTHONPATH=.reference/sqlglot-v30.17.0 python3 scripts/extract_transpile_corpus.py
"""

import json
import sys
import unittest

if sys.version_info < (3, 11):
    sys.exit("extract_transpile_corpus.py requires Python >= 3.11")

try:
    import sqlglot  # noqa: F401
except ImportError:
    sys.exit("Run scripts/fetch-reference.sh first, then PYTHONPATH=.reference/sqlglot-v30.17.0 python3 ...")

from sqlglot.errors import UnsupportedError  # noqa: E402
from tests.dialects.test_dialect import TestDialect, Validator  # noqa: E402
from tests.dialects.test_athena import TestAthena  # noqa: E402
from tests.dialects.test_hive import TestHive  # noqa: E402
from tests.dialects.test_mysql import TestMySQL  # noqa: E402
from tests.dialects.test_postgres import TestPostgres  # noqa: E402
from tests.dialects.test_presto import TestPresto  # noqa: E402
from tests.dialects.test_trino import TestTrino  # noqa: E402
from tests.dialects.test_spark import TestSpark  # noqa: E402

SUPPORTED = {"", "mysql", "postgres", "presto", "trino", "hive", "athena", "spark2", "spark"}
OUT_PATH = "testdata/transpile_cases.jsonl"

captured = []


def _name(dialect):
    return "" if dialect is None else str(dialect)


def patched_validate_all(self, sql, read=None, write=None, pretty=False, identify=False):
    me = _name(self.dialect)
    for read_dialect, read_sql in (read or {}).items():
        rd = _name(read_dialect)
        if rd in SUPPORTED and me in SUPPORTED and rd != me:
            captured.append({"read": rd, "sql": read_sql, "write": me, "want": sql,
                             "pretty": bool(pretty), "identify": bool(identify)})
    for write_dialect, write_sql in (write or {}).items():
        wd = _name(write_dialect)
        if wd in SUPPORTED and me in SUPPORTED and wd != me:
            rec = {"read": me, "sql": sql, "write": wd, "pretty": bool(pretty),
                   "identify": bool(identify)}
            if write_sql is UnsupportedError:
                rec["want"] = ""
                rec["unsupported"] = True
            else:
                rec["want"] = write_sql
            captured.append(rec)
    try:
        self.parse_one(sql)
    except Exception:
        pass
    return None


def main():
    Validator.validate_all = patched_validate_all
    runner = unittest.TextTestRunner(verbosity=0, stream=open("/dev/null", "w"))
    for cls in (TestDialect, TestMySQL, TestPostgres, TestAthena, TestTrino, TestPresto, TestHive):
        runner.run(unittest.defaultTestLoader.loadTestsFromTestCase(cls))

    records = {}
    for rec in captured:
        key = (rec["read"], rec["sql"], rec["write"], rec["want"], rec["pretty"], rec["identify"])
        records[key] = rec
    deduped = sorted(records.values(), key=lambda r: (r["read"], r["write"], r["sql"], r["want"]))
    with open(OUT_PATH, "w") as f:
        for rec in deduped:
            f.write(json.dumps(rec) + "\n")

    counts = {}
    for rec in deduped:
        counts[f"{rec['read'] or 'base'}->{rec['write'] or 'base'}"] = counts.get(
            f"{rec['read'] or 'base'}->{rec['write'] or 'base'}", 0) + 1
    print(f"captured {len(captured)} pair(s), wrote {len(deduped)} deduped to {OUT_PATH}", file=sys.stderr)
    for k in sorted(counts):
        print(f"  {k}: {counts[k]}", file=sys.stderr)


if __name__ == "__main__":
    main()
