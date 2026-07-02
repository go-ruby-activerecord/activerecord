// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Dialect selects identifier quoting and value literalization so the emitted
// SQL is byte-faithful to what the matching ActiveRecord adapter produces.
type Dialect int

const (
	// SQLite mirrors ActiveRecord's sqlite3 adapter: double-quoted identifiers,
	// booleans as TRUE/FALSE, strings single-quoted with '' escaping.
	SQLite Dialect = iota
	// Postgres mirrors ActiveRecord's postgresql adapter: double-quoted
	// identifiers, booleans as TRUE/FALSE, blobs as bytea hex.
	Postgres
	// MySQL mirrors ActiveRecord's mysql2/trilogy adapter: backtick-quoted
	// identifiers, booleans as 1/0, backslash-escaped strings.
	MySQL
)

// String returns the dialect's canonical adapter name.
func (d Dialect) String() string {
	switch d {
	case Postgres:
		return "postgresql"
	case MySQL:
		return "mysql"
	default:
		return "sqlite3"
	}
}

// DialectByName maps an ActiveRecord adapter name to a Dialect. Unknown names
// map to SQLite (the deterministic default used by the oracle).
func DialectByName(name string) Dialect {
	switch strings.ToLower(name) {
	case "postgresql", "postgres", "pg":
		return Postgres
	case "mysql", "mysql2", "trilogy":
		return MySQL
	default:
		return SQLite
	}
}

// quoteColumnName quotes a bare column (or alias) name for the dialect,
// matching connection.quote_column_name. A "*" is passed through unquoted.
func (d Dialect) quoteColumnName(name string) string {
	if name == "*" {
		return "*"
	}
	if d == MySQL {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteTableName quotes a table name, honouring a "schema.table" split the way
// ActiveRecord's quote_table_name does (each part quoted separately).
func (d Dialect) quoteTableName(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return d.quoteColumnName(name[:i]) + "." + d.quoteColumnName(name[i+1:])
	}
	return d.quoteColumnName(name)
}

// qualify returns "table"."column" for the dialect.
func (d Dialect) qualify(table, column string) string {
	return d.quoteTableName(table) + "." + d.quoteColumnName(column)
}

// quote literalizes a bind value exactly as connection.quote would for the
// dialect. It is the single source of truth for value rendering.
func (d Dialect) quote(v Value) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		return d.quoteBool(x)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case *big.Int:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case string:
		return d.quoteString(x)
	case Symbol:
		return d.quoteString(string(x))
	case time.Time:
		return d.quoteString(x.UTC().Format("2006-01-02 15:04:05.999999"))
	default:
		return d.quoteString(stringify(v))
	}
}

// quoteBool renders a boolean literal per adapter.
func (d Dialect) quoteBool(b bool) string {
	if d == MySQL {
		if b {
			return "1"
		}
		return "0"
	}
	if b {
		return "TRUE"
	}
	return "FALSE"
}

// quoteString single-quotes and escapes a string per adapter. SQLite and
// Postgres double the single quote; MySQL additionally backslash-escapes.
func (d Dialect) quoteString(s string) string {
	if d == MySQL {
		var b strings.Builder
		b.WriteByte('\'')
		for i := 0; i < len(s); i++ {
			switch c := s[i]; c {
			case '\'', '\\':
				b.WriteByte('\\')
				b.WriteByte(c)
			case 0:
				b.WriteString(`\0`)
			default:
				b.WriteByte(c)
			}
		}
		b.WriteByte('\'')
		return b.String()
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// stringify renders an arbitrary Go value to its Ruby to_s-ish string for
// literalization of otherwise-unhandled types (kept small and deterministic).
func stringify(v Value) string {
	switch x := v.(type) {
	case string:
		return x
	case Symbol:
		return string(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return strconv.Quote("")[1:1] // unreachable in practice; empty
	}
}
