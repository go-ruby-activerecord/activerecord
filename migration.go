// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file rounds out the schema/migration subsystem: the DDL builders schema.go
// lacked (drop table, drop/rename/change column, remove index, add timestamps)
// and a [Migrator] that actually runs migrations through the injected [Adapter],
// tracking applied versions in the schema_migrations table exactly like
// ActiveRecord::Migrator.
//
// The column ALTER builders emit the standard ANSI form (ALTER TABLE … DROP/
// RENAME/ALTER COLUMN) that ActiveRecord's postgresql and mysql2 adapters
// produce; the sqlite3 adapter rebuilds the table for those operations, so the
// oracle-compared builders are the ones sqlite emits verbatim (create table, drop
// table, add column, add/remove index, add foreign key).

import (
	"strings"
)

// DropTableSQL renders DROP TABLE (drop_table).
func DropTableSQL(dialect Dialect, table string) string {
	return "DROP TABLE " + dialect.quoteTableName(table)
}

// RemoveColumnSQL renders ALTER TABLE … DROP COLUMN (remove_column), the ANSI
// form postgres/mysql emit.
func RemoveColumnSQL(dialect Dialect, table, column string) string {
	return "ALTER TABLE " + dialect.quoteTableName(table) +
		" DROP COLUMN " + dialect.quoteColumnName(column)
}

// RenameColumnSQL renders ALTER TABLE … RENAME COLUMN … TO … (rename_column).
func RenameColumnSQL(dialect Dialect, table, from, to string) string {
	return "ALTER TABLE " + dialect.quoteTableName(table) +
		" RENAME COLUMN " + dialect.quoteColumnName(from) + " TO " + dialect.quoteColumnName(to)
}

// ChangeColumnNullSQL renders the ALTER that toggles a column's NOT NULL
// constraint (change_column_null). Postgres uses SET/DROP NOT NULL; the other
// dialects use the MODIFY form.
func ChangeColumnNullSQL(dialect Dialect, table, column, typ string, null bool) string {
	q := dialect.quoteTableName(table) + " ALTER COLUMN " + dialect.quoteColumnName(column)
	if dialect == Postgres {
		if null {
			return "ALTER TABLE " + q + " DROP NOT NULL"
		}
		return "ALTER TABLE " + q + " SET NOT NULL"
	}
	col := dialect.quoteColumnName(column) + " " + dialect.typeToSQL(typ)
	if !null {
		col += " NOT NULL"
	}
	return "ALTER TABLE " + dialect.quoteTableName(table) + " MODIFY " + col
}

// RemoveIndexSQL renders DROP INDEX (remove_index). The index name defaults to
// ActiveRecord's index_<table>_on_<cols> convention when not given.
func RemoveIndexSQL(dialect Dialect, table string, cols []string, name string) string {
	if name == "" {
		name = "index_" + table + "_on_" + strings.Join(cols, "_and_")
	}
	return "DROP INDEX " + dialect.quoteColumnName(name)
}

// AddTimestampsSQL renders the two ALTER TABLE … ADD statements add_timestamps
// issues (created_at, updated_at as NOT NULL datetimes).
func AddTimestampsSQL(dialect Dialect, table string) []string {
	return []string{
		AddColumnSQL(dialect, table, "created_at", "datetime", NotNull()),
		AddColumnSQL(dialect, table, "updated_at", "datetime", NotNull()),
	}
}

// SchemaMigrationsTableSQL renders the CREATE TABLE for ActiveRecord's
// schema_migrations bookkeeping table.
func SchemaMigrationsTableSQL(dialect Dialect) string {
	return "CREATE TABLE " + dialect.quoteTableName("schema_migrations") +
		" (" + dialect.quoteColumnName("version") + " " + dialect.typeToSQL("string") +
		" NOT NULL PRIMARY KEY)"
}

// Migrator runs schema migrations through an [Adapter], recording applied
// versions in schema_migrations so migrations are idempotent, mirroring
// ActiveRecord::Migrator.
type Migrator struct {
	a       Adapter
	d       Dialect
	ensured bool
}

// NewMigrator returns a Migrator bound to the adapter's connection and dialect.
func NewMigrator(a Adapter) *Migrator { return &Migrator{a: a, d: DialectFor(a)} }

// Dialect returns the migrator's dialect.
func (mg *Migrator) Dialect() Dialect { return mg.d }

// Execute runs one DDL/DML statement through the adapter.
func (mg *Migrator) Execute(sql string) error {
	_, _, err := mg.a.ExecuteDML(sql)
	return err
}

// EnsureSchemaMigrations creates the schema_migrations table once per migrator.
func (mg *Migrator) EnsureSchemaMigrations() error {
	if mg.ensured {
		return nil
	}
	if err := mg.Execute("CREATE TABLE IF NOT EXISTS " + mg.d.quoteTableName("schema_migrations") +
		" (" + mg.d.quoteColumnName("version") + " " + mg.d.typeToSQL("string") +
		" NOT NULL PRIMARY KEY)"); err != nil {
		return err
	}
	mg.ensured = true
	return nil
}

// AppliedVersions returns the migration versions already recorded, in the order
// the adapter returns them.
func (mg *Migrator) AppliedVersions() ([]string, error) {
	rows, err := mg.a.Execute("SELECT " + mg.d.quoteColumnName("version") +
		" FROM " + mg.d.quoteTableName("schema_migrations"))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if v, ok := row["version"]; ok {
			out = append(out, toStr(v))
		}
	}
	return out, nil
}

// applied reports whether version is already recorded.
func (mg *Migrator) applied(version string) (bool, error) {
	vs, err := mg.AppliedVersions()
	if err != nil {
		return false, err
	}
	for _, v := range vs {
		if v == version {
			return true, nil
		}
	}
	return false, nil
}

// Migrate runs the up migration for version inside a transaction and records the
// version, unless it is already applied. It returns whether the migration ran.
func (mg *Migrator) Migrate(version string, up func(*Migrator) error) (bool, error) {
	if err := mg.EnsureSchemaMigrations(); err != nil {
		return false, err
	}
	done, err := mg.applied(version)
	if err != nil || done {
		return false, err
	}
	txErr := Transaction(mg.a, func() error {
		if err := up(mg); err != nil {
			return err
		}
		return mg.Execute("INSERT INTO " + mg.d.quoteTableName("schema_migrations") +
			" (" + mg.d.quoteColumnName("version") + ") VALUES (" + mg.d.quoteString(version) + ")")
	})
	if txErr != nil {
		return false, txErr
	}
	return true, nil
}

// Rollback runs the down migration for version inside a transaction and removes
// the version record, when the version is currently applied. It returns whether
// the rollback ran.
func (mg *Migrator) Rollback(version string, down func(*Migrator) error) (bool, error) {
	if err := mg.EnsureSchemaMigrations(); err != nil {
		return false, err
	}
	done, err := mg.applied(version)
	if err != nil || !done {
		return false, err
	}
	txErr := Transaction(mg.a, func() error {
		if err := down(mg); err != nil {
			return err
		}
		return mg.Execute("DELETE FROM " + mg.d.quoteTableName("schema_migrations") +
			" WHERE " + mg.d.quoteColumnName("version") + " = " + mg.d.quoteString(version))
	})
	if txErr != nil {
		return false, txErr
	}
	return true, nil
}
