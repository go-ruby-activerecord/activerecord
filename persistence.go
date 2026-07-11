// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file implements ActiveRecord's persistence execution — Save / Create /
// Update / Destroy / Delete — the "statement execution" the README used to defer
// to the host. It renders the INSERT / UPDATE / DELETE (byte-faithful to what
// ActiveRecord's sqlite3/postgresql/mysql2 adapters emit, including column order
// and the RETURNING clause), runs the full validation + callback lifecycle
// around the write inside a transaction, drives the SQL through the injected
// [Adapter], and writes the generated primary key and dirty state back onto the
// [Record].
//
// The write is wrapped in [Transaction]; because Transaction tracks nesting per
// connection, a Save inside a user Transaction automatically runs on a SAVEPOINT,
// exactly like nested record saves in ActiveRecord.

import (
	"sort"
	"strings"
	"time"
)

// nowFunc supplies the timestamp for created_at/updated_at. It is a package
// variable so tests can pin a deterministic clock; hosts leave it as time.Now.
var nowFunc = time.Now

// saveConfig holds the resolved options for a Save.
type saveConfig struct {
	validate bool
}

// SaveOption configures Save/Create.
type SaveOption func(*saveConfig)

// WithoutValidation skips validations for this write (save(validate: false)).
func WithoutValidation() SaveOption { return func(c *saveConfig) { c.validate = false } }

// Save inserts a new record or updates a changed one through the adapter, running
// the validation and callback lifecycle in a transaction. It returns whether the
// record was saved (false on a validation failure or a halted before_* callback,
// with no error) and any database or callback error.
//
// The lifecycle mirrors ActiveRecord's save:
//
//	before_validation → validate → after_validation   (outside the transaction)
//	BEGIN
//	  before_save → before_(create|update) → INSERT/UPDATE →
//	  after_(create|update) → after_save
//	COMMIT
//	after_commit                                       (or after_rollback)
func Save(a Adapter, rec *Record, opts ...SaveOption) (bool, error) {
	cfg := saveConfig{validate: true}
	for _, o := range opts {
		o(&cfg)
	}
	m := rec.model

	if cfg.validate {
		if err := m.runBefore(beforeValidationCB, rec); err != nil {
			return halt(err)
		}
		errs := rec.Validate()
		if err := m.runAfter(afterValidationCB, rec); err != nil {
			return false, err
		}
		if !errs.Empty() {
			return false, nil
		}
	}

	create := !rec.persisted
	m.touchTimestamps(rec, create)

	txErr := Transaction(a, func() error {
		if err := m.runBefore(beforeSaveCB, rec); err != nil {
			return err
		}
		if create {
			if err := m.runCreate(a, rec); err != nil {
				return err
			}
		} else {
			if err := m.runUpdate(a, rec); err != nil {
				return err
			}
		}
		return m.runAfter(afterSaveCB, rec)
	})

	if txErr != nil {
		_ = m.runAfter(afterRollbackCB, rec)
		if txErr == ErrAbort {
			return false, nil
		}
		return false, txErr
	}
	rec.persisted = true
	rec.SaveClean()
	if err := m.runAfter(afterCommitCB, rec); err != nil {
		return true, err
	}
	return true, nil
}

// runCreate fires the create callbacks around the INSERT and writes the returned
// primary key back onto the record.
func (m *Model) runCreate(a Adapter, rec *Record) error {
	if err := m.runBefore(beforeCreateCB, rec); err != nil {
		return err
	}
	if err := m.executeInsert(a, rec); err != nil {
		return err
	}
	return m.runAfter(afterCreateCB, rec)
}

// runUpdate fires the update callbacks around the UPDATE. When nothing changed
// no SQL is issued (ActiveRecord's partial-writes behaviour) but the callbacks
// still run.
func (m *Model) runUpdate(a Adapter, rec *Record) error {
	if err := m.runBefore(beforeUpdateCB, rec); err != nil {
		return err
	}
	if len(rec.changedAttrs()) > 0 {
		if _, _, err := a.ExecuteDML(rec.updateSQL()); err != nil {
			return err
		}
	}
	return m.runAfter(afterUpdateCB, rec)
}

// executeInsert runs the INSERT and assigns the primary key. On dialects with
// RETURNING (sqlite/postgres) the key comes back as a row; otherwise the
// adapter's last-insert-id is used.
func (m *Model) executeInsert(a Adapter, rec *Record) error {
	sql := rec.insertSQL()
	if m.Dialect.supportsReturning() {
		rows, err := a.Execute(sql)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			if v, ok := rows[0][m.PrimaryKey]; ok {
				rec.Set(m.PrimaryKey, v)
			}
		}
		return nil
	}
	_, lastID, err := a.ExecuteDML(sql)
	if err != nil {
		return err
	}
	if _, ok := rec.attrs[m.PrimaryKey]; !ok || rec.attrs[m.PrimaryKey] == nil {
		rec.Set(m.PrimaryKey, lastID)
	}
	return nil
}

// Create builds a record from attrs and saves it, returning the record (whose
// [Record.IsPersisted] reports success) and any error, matching Model.create.
func (m *Model) Create(a Adapter, attrs map[string]any, opts ...SaveOption) (*Record, error) {
	rec := m.Build(attrs)
	if _, err := Save(a, rec, opts...); err != nil {
		return rec, err
	}
	return rec, nil
}

// Update assigns attrs to a persisted record and saves it (record.update).
func (r *Record) Update(a Adapter, attrs map[string]any, opts ...SaveOption) (bool, error) {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r.Set(k, attrs[k])
	}
	return Save(a, r, opts...)
}

// Destroy deletes the record's row through the adapter inside a transaction,
// running the destroy callbacks (before_destroy → DELETE → after_destroy). It
// reports success and any error; a halted before_destroy returns (false, nil).
func Destroy(a Adapter, rec *Record) (bool, error) {
	m := rec.model
	txErr := Transaction(a, func() error {
		if err := m.runBefore(beforeDestroyCB, rec); err != nil {
			return err
		}
		if _, _, err := a.ExecuteDML(rec.deleteSQL()); err != nil {
			return err
		}
		return m.runAfter(afterDestroyCB, rec)
	})
	if txErr != nil {
		_ = m.runAfter(afterRollbackCB, rec)
		if txErr == ErrAbort {
			return false, nil
		}
		return false, txErr
	}
	rec.destroyed = true
	rec.persisted = false
	if err := m.runAfter(afterCommitCB, rec); err != nil {
		return true, err
	}
	return true, nil
}

// Delete removes the record's row with a single DELETE and no callbacks or
// transaction, matching ActiveRecord's record.delete.
func Delete(a Adapter, rec *Record) error {
	if _, _, err := a.ExecuteDML(rec.deleteSQL()); err != nil {
		return err
	}
	rec.destroyed = true
	rec.persisted = false
	return nil
}

// halt maps a before_* callback error to Save's (saved, err) result: ErrAbort is
// a soft failure (false, nil); any other error propagates.
func halt(err error) (bool, error) {
	if err == ErrAbort {
		return false, nil
	}
	return false, err
}

// touchTimestamps sets updated_at (and created_at on create) to the current time
// when the model carries those columns, matching ActiveRecord::Timestamp.
func (m *Model) touchTimestamps(rec *Record, create bool) {
	now := nowFunc()
	if create && m.HasColumn("created_at") {
		if _, ok := rec.attrs["created_at"]; !ok {
			rec.Set("created_at", now)
		}
	}
	if m.HasColumn("updated_at") {
		// On update, only touch when something else changed (partial writes).
		if !create && len(rec.changedAttrs()) == 0 {
			return
		}
		rec.Set("updated_at", now)
	}
}

// insertSQL renders the INSERT for a new record: the set columns in model
// declaration order (as ActiveRecord builds the column list), a RETURNING clause
// on dialects that support it.
func (r *Record) insertSQL() string {
	m := r.model
	var cols, vals []string
	for _, c := range m.columns {
		v, ok := r.attrs[c.Name]
		if !ok {
			continue
		}
		if c.Name == m.PrimaryKey && v == nil {
			continue
		}
		cols = append(cols, m.Dialect.quoteColumnName(c.Name))
		vals = append(vals, m.Dialect.quote(v))
	}
	sql := "INSERT INTO " + m.Dialect.quoteTableName(m.TableName) +
		" (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(vals, ", ") + ")"
	if m.Dialect.supportsReturning() {
		sql += " RETURNING " + m.Dialect.quoteColumnName(m.PrimaryKey)
	}
	return sql
}

// updateSQL renders the UPDATE for a changed record: SET the changed columns in
// model declaration order, WHERE the primary key.
func (r *Record) updateSQL() string {
	m := r.model
	changed := map[string]bool{}
	for _, k := range r.changedAttrs() {
		changed[k] = true
	}
	var sets []string
	for _, c := range m.columns {
		if !changed[c.Name] {
			continue
		}
		sets = append(sets, m.Dialect.quoteColumnName(c.Name)+" = "+m.Dialect.quote(r.attrs[c.Name]))
	}
	return "UPDATE " + m.Dialect.quoteTableName(m.TableName) + " SET " + strings.Join(sets, ", ") +
		" WHERE " + m.Dialect.qualify(m.TableName, m.PrimaryKey) + " = " + m.Dialect.quote(r.attrs[m.PrimaryKey])
}

// deleteSQL renders the DELETE for the record's row.
func (r *Record) deleteSQL() string {
	m := r.model
	return "DELETE FROM " + m.Dialect.quoteTableName(m.TableName) +
		" WHERE " + m.Dialect.qualify(m.TableName, m.PrimaryKey) + " = " + m.Dialect.quote(r.attrs[m.PrimaryKey])
}

// supportsReturning reports whether the dialect's INSERT uses a RETURNING clause
// for the generated key (sqlite3 and postgresql do; mysql2 uses last_insert_id).
func (d Dialect) supportsReturning() bool { return d == SQLite || d == Postgres }
