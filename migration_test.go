// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"strings"
	"testing"
)

func TestMigrationDDLBuilders(t *testing.T) {
	eq(t, "drop-table", DropTableSQL(SQLite, "users"), `DROP TABLE "users"`)
	eq(t, "remove-col", RemoveColumnSQL(SQLite, "users", "age"),
		`ALTER TABLE "users" DROP COLUMN "age"`)
	eq(t, "rename-col", RenameColumnSQL(Postgres, "users", "name", "full_name"),
		`ALTER TABLE "users" RENAME COLUMN "name" TO "full_name"`)
	eq(t, "null-pg-set", ChangeColumnNullSQL(Postgres, "users", "name", "string", false),
		`ALTER TABLE "users" ALTER COLUMN "name" SET NOT NULL`)
	eq(t, "null-pg-drop", ChangeColumnNullSQL(Postgres, "users", "name", "string", true),
		`ALTER TABLE "users" ALTER COLUMN "name" DROP NOT NULL`)
	eq(t, "null-mysql", ChangeColumnNullSQL(MySQL, "users", "age", "integer", false),
		"ALTER TABLE `users` MODIFY `age` int NOT NULL")
	eq(t, "null-mysql-nullable", ChangeColumnNullSQL(MySQL, "users", "age", "integer", true),
		"ALTER TABLE `users` MODIFY `age` int")
	eq(t, "remove-index-default", RemoveIndexSQL(SQLite, "widgets", []string{"name"}, ""),
		`DROP INDEX "index_widgets_on_name"`)
	eq(t, "remove-index-named", RemoveIndexSQL(SQLite, "widgets", nil, "custom_idx"),
		`DROP INDEX "custom_idx"`)
	ts := AddTimestampsSQL(SQLite, "widgets")
	eq(t, "ts-created", ts[0], `ALTER TABLE "widgets" ADD "created_at" datetime(6) NOT NULL`)
	eq(t, "ts-updated", ts[1], `ALTER TABLE "widgets" ADD "updated_at" datetime(6) NOT NULL`)
	eq(t, "schema-migrations", SchemaMigrationsTableSQL(SQLite),
		`CREATE TABLE "schema_migrations" ("version" varchar NOT NULL PRIMARY KEY)`)
	eq(t, "schema-migrations-pg", SchemaMigrationsTableSQL(Postgres),
		`CREATE TABLE "schema_migrations" ("version" character varying NOT NULL PRIMARY KEY)`)
}

func TestMigratorRun(t *testing.T) {
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}} // no versions applied yet
	resetTxState(a)
	mg := NewMigrator(a)
	if mg.Dialect() != SQLite {
		t.Error("dialect")
	}
	ran, err := mg.Migrate("20260711", func(m *Migrator) error {
		return m.Execute(DropTableSQL(m.Dialect(), "old"))
	})
	if err != nil || !ran {
		t.Fatalf("migrate = %v %v", ran, err)
	}
	joined := strings.Join(a.log, "\n")
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS \"schema_migrations\"",
		"BEGIN immediate TRANSACTION",
		`DROP TABLE "old"`,
		`INSERT INTO "schema_migrations" ("version") VALUES ('20260711')`,
		"COMMIT TRANSACTION",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("migrate log missing %q in:\n%s", want, joined)
		}
	}
	// EnsureSchemaMigrations only creates the table once.
	before := len(a.log)
	if err := mg.EnsureSchemaMigrations(); err != nil {
		t.Fatal(err)
	}
	if len(a.log) != before {
		t.Error("EnsureSchemaMigrations ran twice")
	}
}

func TestMigratorIdempotent(t *testing.T) {
	// A version already recorded is skipped.
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"version": "1"}}}}
	resetTxState(a)
	mg := NewMigrator(a)
	ran, err := mg.Migrate("1", func(*Migrator) error { t.Fatal("up should not run"); return nil })
	if err != nil || ran {
		t.Fatalf("idempotent migrate = %v %v", ran, err)
	}
}

func TestMigratorRollback(t *testing.T) {
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"version": "1"}}}}
	resetTxState(a)
	mg := NewMigrator(a)
	ran, err := mg.Rollback("1", func(m *Migrator) error {
		return m.Execute(DropTableSQL(m.Dialect(), "t"))
	})
	if err != nil || !ran {
		t.Fatalf("rollback = %v %v", ran, err)
	}
	if !strings.Contains(strings.Join(a.log, "\n"),
		`DELETE FROM "schema_migrations" WHERE "version" = '1'`) {
		t.Errorf("rollback did not delete version: %#v", a.log)
	}
	// Rolling back an unapplied version is a no-op.
	a2 := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}}
	resetTxState(a2)
	if ran, _ := NewMigrator(a2).Rollback("9", func(*Migrator) error { return nil }); ran {
		t.Error("unapplied rollback should be no-op")
	}
}

func TestMigratorAppliedVersions(t *testing.T) {
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{
		{"version": "1"}, {"version": "2"}, {"other": "x"}, // missing-version row skipped
	}}}
	mg := NewMigrator(a)
	vs, err := mg.AppliedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 || vs[0] != "1" || vs[1] != "2" {
		t.Errorf("applied = %v", vs)
	}
}

func TestMigratorErrorPaths(t *testing.T) {
	// EnsureSchemaMigrations create error.
	a := &recAdapter{name: "sqlite3", failOn: "CREATE TABLE"}
	resetTxState(a)
	if _, err := NewMigrator(a).Migrate("1", func(*Migrator) error { return nil }); err == nil {
		t.Error("ensure error expected")
	}
	// AppliedVersions SELECT error.
	a2 := &recAdapter{name: "sqlite3", failOn: "SELECT"}
	resetTxState(a2)
	if _, err := NewMigrator(a2).Migrate("1", func(*Migrator) error { return nil }); err == nil {
		t.Error("applied select error expected")
	}
	// up() error rolls back and surfaces.
	a3 := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}, failOn: "DROP"}
	resetTxState(a3)
	if _, err := NewMigrator(a3).Migrate("1", func(m *Migrator) error {
		return m.Execute("DROP TABLE x")
	}); err == nil {
		t.Error("up error expected")
	}
	// Rollback ensure error and down error.
	a4 := &recAdapter{name: "sqlite3", failOn: "CREATE TABLE"}
	resetTxState(a4)
	if _, err := NewMigrator(a4).Rollback("1", func(*Migrator) error { return nil }); err == nil {
		t.Error("rollback ensure error expected")
	}
	a5 := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"version": "1"}}}, failOn: "DROP"}
	resetTxState(a5)
	if _, err := NewMigrator(a5).Rollback("1", func(m *Migrator) error {
		return m.Execute("DROP TABLE x")
	}); err == nil {
		t.Error("down error expected")
	}
}
