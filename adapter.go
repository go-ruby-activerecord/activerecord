// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// Adapter is the host seam through which this deterministic core reaches a real
// database. A host wires an implementation backed by go-ruby-sqlite3 or
// go-ruby-pg (or any driver) and this package hands it the SQL strings it
// renders; the actual execution, connection pooling and transactions live in the
// host. Keeping execution behind this interface is what makes the SQL-generation
// core 100% Ruby- and CGO-free and byte-for-byte testable against the
// ActiveRecord oracle.
//
// Row is one result row as an ordered column=>value map; the host materializes
// these into [Record]s via [Model.Load].
type Adapter interface {
	// Execute runs a statement that returns rows (a SELECT / ExistsSQL probe)
	// and yields them in order.
	Execute(sql string) ([]Row, error)
	// ExecuteDML runs an INSERT/UPDATE/DELETE and returns the affected row
	// count (and the last insert id where the driver provides it).
	ExecuteDML(sql string) (affected int64, lastInsertID int64, err error)
	// AdapterName reports the driver's ActiveRecord adapter name, used to pick
	// the [Dialect] ("sqlite3", "postgresql", "mysql2").
	AdapterName() string
}

// Row is one result row: column name => value.
type Row = map[string]any

// DialectFor returns the Dialect matching an adapter's AdapterName.
func DialectFor(a Adapter) Dialect { return DialectByName(a.AdapterName()) }

// Exists runs the relation's existence probe through the adapter and reports
// whether a matching row exists (the execution half of exists?). It is the
// canonical wiring a uniqueness validator's callback uses.
func Exists(a Adapter, r *Relation) (bool, error) {
	rows, err := a.Execute(r.ExistsSQL())
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// Count runs the relation's COUNT(*) through the adapter and returns the scalar.
// The single result row's single column is coerced to int64.
func Count(a Adapter, r *Relation) (int64, error) {
	rows, err := a.Execute(r.CountSQL())
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	for _, v := range rows[0] {
		return toInt64(v), nil
	}
	return 0, nil
}

// LoadAll runs the relation's SELECT through the adapter and materializes each
// row into a persisted [Record].
func LoadAll(a Adapter, r *Relation) ([]*Record, error) {
	rows, err := a.Execute(r.ToSQL())
	if err != nil {
		return nil, err
	}
	out := make([]*Record, 0, len(rows))
	for _, row := range rows {
		out = append(out, r.model.Load(row))
	}
	return out, nil
}

// toInt64 coerces a scalar aggregate result to int64.
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	default:
		if f, _, ok := toNumber(v); ok {
			return int64(f)
		}
		return 0
	}
}
