// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file implements ActiveRecord's StatementCache — the prepared-statement
// caching the README used to defer. ActiveRecord builds a SQL template once, with
// bind substitutes standing in for the values, caches it keyed by the query
// shape, and on each call supplies the actual binds instead of regenerating the
// SQL (the fast path behind Model.find / find_by and association loading):
//
//	stmt = StatementCache.create(conn) { |p| Book.where(id: p.bind).limit(1) }
//	stmt.execute([id], conn)
//
// The port mirrors this: a [PreparedStatement] holds the template SQL (with the
// dialect's bind markers — "?" for sqlite/mysql, "$n" for postgres) and an
// ordered list of bind slots; [StatementCache] memoizes statements by key. A
// host that wires a driver-level prepared statement implements [PreparedAdapter]
// and gets a true server-side prepare; a plain [Adapter] transparently gets the
// binds inlined into the SQL, so both work.

import (
	"strconv"
	"strings"
	"sync"
)

// Substitute is ActiveRecord's bind placeholder (StatementCache::Substitute, i.e.
// `params.bind`): it marks where a value will be supplied at execute time rather
// than baked into the template.
type Substitute struct{}

// bindSlot is one position in a prepared statement's bind list. A fixed slot
// carries a value baked into the statement (e.g. the LIMIT 1 that find always
// binds); a non-fixed slot is filled positionally from the values passed to
// Execute.
type bindSlot struct {
	fixed bool
	value any
}

// PreparedStatement is a cached SQL template plus its ordered bind slots. It is
// immutable and safe to reuse across calls and goroutines.
type PreparedStatement struct {
	model *Model
	// SQL is the template with dialect bind markers.
	SQL   string
	slots []bindSlot
}

// StatementCache memoizes prepared statements by a string key (the query shape),
// mirroring the per-model statement cache ActiveRecord keeps for find/find_by.
type StatementCache struct {
	mu    sync.Mutex
	items map[string]*PreparedStatement
}

// NewStatementCache returns an empty statement cache.
func NewStatementCache() *StatementCache {
	return &StatementCache{items: map[string]*PreparedStatement{}}
}

// Fetch returns the cached statement for key, building and caching it with build
// on a miss (ActiveRecord's StatementCache.create-then-cache pattern).
func (c *StatementCache) Fetch(key string, build func() *PreparedStatement) *PreparedStatement {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.items[key]; ok {
		return s
	}
	s := build()
	c.items[key] = s
	return s
}

// statementCache lazily creates the model's own statement cache.
func (m *Model) statementCache() *StatementCache {
	if m.stmts == nil {
		m.stmts = NewStatementCache()
	}
	return m.stmts
}

// FindStatement returns the cached prepared statement for Model.find(id):
// SELECT … WHERE pk = ? LIMIT ?, binding [id, 1].
func (m *Model) FindStatement() *PreparedStatement {
	return m.statementCache().Fetch("find", func() *PreparedStatement {
		d := m.Dialect
		sql := "SELECT " + m.starSelect() + " FROM " + d.quoteTableName(m.TableName) +
			" WHERE " + d.qualify(m.TableName, m.PrimaryKey) + " = " + d.placeholder(1) +
			" LIMIT " + d.placeholder(2)
		return &PreparedStatement{model: m, SQL: sql, slots: []bindSlot{{}, {fixed: true, value: 1}}}
	})
}

// FindByStatement returns the cached prepared statement for Model.find_by over
// the given attribute names (order-significant): SELECT … WHERE a = ? AND b = ?
// … LIMIT ?, binding the supplied values then 1.
func (m *Model) FindByStatement(attrs ...string) *PreparedStatement {
	key := "find_by:" + strings.Join(attrs, ",")
	return m.statementCache().Fetch(key, func() *PreparedStatement {
		d := m.Dialect
		preds := make([]string, len(attrs))
		slots := make([]bindSlot, 0, len(attrs)+1)
		for i, a := range attrs {
			preds[i] = d.qualify(m.TableName, a) + " = " + d.placeholder(i+1)
			slots = append(slots, bindSlot{})
		}
		slots = append(slots, bindSlot{fixed: true, value: 1})
		sql := "SELECT " + m.starSelect() + " FROM " + d.quoteTableName(m.TableName) +
			" WHERE " + strings.Join(preds, " AND ") +
			" LIMIT " + d.placeholder(len(attrs)+1)
		return &PreparedStatement{model: m, SQL: sql, slots: slots}
	})
}

// binds assembles the full ordered bind list, filling non-fixed slots from
// supplied in order and using fixed slots' baked values.
func (s *PreparedStatement) binds(supplied []any) []any {
	out := make([]any, 0, len(s.slots))
	si := 0
	for _, slot := range s.slots {
		if slot.fixed {
			out = append(out, slot.value)
			continue
		}
		if si < len(supplied) {
			out = append(out, supplied[si])
			si++
		} else {
			out = append(out, nil)
		}
	}
	return out
}

// Execute runs the statement with the supplied bind values and materializes the
// result rows into persisted records. A [PreparedAdapter] host gets a true
// prepared execution; a plain [Adapter] gets the binds inlined into the SQL.
func (s *PreparedStatement) Execute(a Adapter, supplied ...any) ([]*Record, error) {
	binds := s.binds(supplied)
	var rows []Row
	var err error
	if pa, ok := a.(PreparedAdapter); ok {
		rows, err = pa.ExecutePrepared(s.SQL, binds)
	} else {
		rows, err = a.Execute(s.model.Dialect.inlineBinds(s.SQL, binds))
	}
	if err != nil {
		return nil, err
	}
	out := make([]*Record, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.model.Load(row))
	}
	return out, nil
}

// ExecuteOne runs the statement and returns the first materialized record, or nil
// when no row matched (the shape find/find_by consume).
func (s *PreparedStatement) ExecuteOne(a Adapter, supplied ...any) (*Record, error) {
	recs, err := s.Execute(a, supplied...)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return recs[0], nil
}

// PreparedAdapter is the optional host seam for true driver-level prepared
// statements. An [Adapter] that also implements it receives the SQL template and
// the ordered bind values, keeping the binds out of the SQL string.
type PreparedAdapter interface {
	Adapter
	// ExecutePrepared runs a prepared statement with positional bind values.
	ExecutePrepared(sql string, binds []any) ([]Row, error)
}

// placeholder renders the dialect's bind marker for the 1-based position n:
// "$n" for postgres, "?" for sqlite/mysql.
func (d Dialect) placeholder(n int) string {
	if d == Postgres {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

// inlineBinds substitutes bind values into a template's markers so a plain
// Adapter (which takes a SQL string) can run a prepared statement. Postgres "$n"
// markers are replaced by index (highest first, so "$1" never clobbers "$10");
// "?" markers are replaced left to right.
func (d Dialect) inlineBinds(sql string, binds []any) string {
	if d == Postgres {
		for i := len(binds); i >= 1; i-- {
			sql = strings.ReplaceAll(sql, "$"+strconv.Itoa(i), d.quote(binds[i-1]))
		}
		return sql
	}
	var b strings.Builder
	bi := 0
	for i := 0; i < len(sql); i++ {
		if sql[i] == '?' && bi < len(binds) {
			b.WriteString(d.quote(binds[bi]))
			bi++
			continue
		}
		b.WriteByte(sql[i])
	}
	return b.String()
}
