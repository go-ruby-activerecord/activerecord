// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file completes the query interface by giving the SQL-building relation
// methods their executable counterparts through the injected [Adapter]: to_a /
// pluck / ids / exists? / count / find / find_by / first / last and the
// find_each batch iterator. The SQL each one runs is exactly what the matching
// query.go builder renders (byte-for-byte with ActiveRecord); these wrappers add
// the execution + materialization the README used to leave to the host.

// ToArray runs the relation's SELECT and returns the materialized records
// (ActiveRecord's to_a / load).
func (r *Relation) ToArray(a Adapter) ([]*Record, error) { return LoadAll(a, r) }

// Count runs COUNT(*) for the relation and returns the scalar (relation.count).
func (r *Relation) Count(a Adapter) (int64, error) { return Count(a, r) }

// Exists runs the existence probe and reports whether any row matches
// (relation.exists?).
func (r *Relation) Exists(a Adapter) (bool, error) { return Exists(a, r) }

// Pluck runs the pluck SELECT and returns, per row, the requested column values
// in order (relation.pluck(:a, :b)). Values are read from the result rows by
// their bare column names, as adapters key them.
func (r *Relation) Pluck(a Adapter, cols ...any) ([][]any, error) {
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		if n, ok := symbolName(c); ok {
			names = append(names, n)
		}
	}
	rows, err := a.Execute(r.PluckSQL(cols...))
	if err != nil {
		return nil, err
	}
	out := make([][]any, 0, len(rows))
	for _, row := range rows {
		vals := make([]any, len(names))
		for i, n := range names {
			vals[i] = row[n]
		}
		out = append(out, vals)
	}
	return out, nil
}

// Ids runs SELECT of the primary key and returns the id values (relation.ids).
func (r *Relation) Ids(a Adapter) ([]any, error) {
	rows, err := a.Execute(r.PluckSQL(r.model.PrimaryKey))
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row[r.model.PrimaryKey])
	}
	return out, nil
}

// FirstRecord runs the relation scoped to its first row and returns it, or nil
// when empty (relation.first).
func (r *Relation) FirstRecord(a Adapter) (*Record, error) { return firstOf(a, r.First()) }

// LastRecord runs the relation scoped to its last row and returns it, or nil
// (relation.last).
func (r *Relation) LastRecord(a Adapter) (*Record, error) { return firstOf(a, r.Last()) }

// TakeRecord runs the relation with LIMIT 1 and returns a row without imposing an
// order, or nil (relation.take).
func (r *Relation) TakeRecord(a Adapter) (*Record, error) { return firstOf(a, r.Take()) }

// firstOf runs a single-row relation and returns the one record or nil.
func firstOf(a Adapter, r *Relation) (*Record, error) {
	recs, err := LoadAll(a, r)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return recs[0], nil
}

// FindRecord looks up a row by primary key through the cached prepared statement
// and returns the record, or nil when not found (Model.find, minus the raise).
func (m *Model) FindRecord(a Adapter, id any) (*Record, error) {
	return m.FindStatement().ExecuteOne(a, id)
}

// FindByRecord returns the first record matching the hash conditions, or nil
// (Model.find_by). The conditions are applied in sorted key order for a
// deterministic single query.
func (m *Model) FindByRecord(a Adapter, cond map[string]any) (*Record, error) {
	return firstOf(a, m.All().FindBy(cond))
}

// FindEach loads the relation in primary-key-ordered batches of batchSize and
// calls fn for each record, stopping early if fn returns an error
// (relation.find_each). A non-positive batchSize defaults to 1000, matching
// ActiveRecord.
func (r *Relation) FindEach(a Adapter, batchSize int, fn func(*Record) error) error {
	if batchSize <= 0 {
		batchSize = 1000
	}
	offset := 0
	for {
		batch, err := LoadAll(a, r.Order(r.model.PrimaryKey).Limit(batchSize).Offset(offset))
		if err != nil {
			return err
		}
		for _, rec := range batch {
			if err := fn(rec); err != nil {
				return err
			}
		}
		if len(batch) < batchSize {
			return nil
		}
		offset += batchSize
	}
}
