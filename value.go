// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package activerecord is a pure-Go (CGO-free) reimplementation of Rails'
// ActiveRecord ORM: the interpreter-independent layers that turn a model +
// relation description into SQL, run it, and materialize records, exactly as
// MRI's activerecord gem does. Talking to the database is a host seam — an
// Adapter the host injects (wired to go-ruby-sqlite3 / go-ruby-pg) — so this
// package is 100% Ruby-free and produces byte-faithful SQL that a differential
// oracle compares to ActiveRecord's own output across adapters.
//
// # Scope
//
//   - Relation: lazy, chainable query building (where/not/or/order/limit/offset/
//     group/having/joins/left_joins/select/distinct + find/find_by/first/last/
//     take, aggregates, pluck, exists?) rendered to a per-Dialect SQL string via
//     [Relation.ToSQL], byte-faithful to ActiveRecord, plus the executable
//     counterparts ([Relation.ToArray]/[Relation.Pluck]/[Relation.Exists]/…).
//   - Schema & migrations: create_table / add_column / add_index /
//     add_foreign_key / drop_table / remove_column / rename_column DDL and a
//     [Migrator] that runs migrations through the adapter with a
//     schema_migrations ledger.
//   - Associations & eager loading: belongs_to / has_many / has_one /
//     has_and_belongs_to_many (and :through) join SQL, plus [Includes]/[Preload]
//     multi-query materialization.
//   - Validations: presence / length / format / numericality / inclusion /
//     exclusion / uniqueness — producing an [Errors] shaped like
//     ActiveModel::Errors with ActiveRecord's default messages.
//   - Attributes & persistence: readers/writers, dirty tracking (changed?/
//     changes), type casting, and [Save]/[Model.Create]/[Destroy] through the
//     adapter with timestamps and generated-key assignment.
//   - Lifecycle: the full before/after callback chain, [Transaction]s with nested
//     savepoints, a prepared-statement [StatementCache], and single-table
//     inheritance ([Model.STI]/[Model.Subclass]).
//
// Only the database round-trip itself — [Adapter.Execute]/[Adapter.ExecuteDML]
// and connection pooling — is the host's; everything ActiveRecord does around it
// lives here.
//
// # Ruby value model
//
// Attribute and bind values are represented by an [any] drawn from a small,
// fixed set of Go types so a host (go-embedded-ruby) can map its object graph to
// and from this package:
//
//	Ruby            Go
//	----            --
//	nil             nil
//	true / false    bool
//	Integer         int, int64, *big.Int
//	Float           float64, float32
//	String          string
//	Symbol          Symbol
//	Array           []any
//	Time            time.Time
package activerecord

// Value is the interface satisfied by every Ruby value this package handles. It
// is purely documentary — the public API uses any.
type Value = any

// Symbol is a Ruby Symbol (`:name`), used for column and association names as a
// host may hand them over. Its string form is the bare name.
type Symbol string

// String returns the symbol's bare name.
func (s Symbol) String() string { return string(s) }

// symbolName normalizes a name argument that may be a Go string or a [Symbol]
// (as a host binds Ruby symbols) to its bare string.
func symbolName(v any) (string, bool) {
	switch n := v.(type) {
	case string:
		return n, true
	case Symbol:
		return string(n), true
	}
	return "", false
}
