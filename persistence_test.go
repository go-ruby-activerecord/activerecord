// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"errors"
	"testing"
	"time"
)

// personModel is a small persistence fixture without timestamp columns, so its
// INSERT/UPDATE/DELETE SQL is deterministic (no clock literal).
func personModel(d Dialect) *Model {
	m := NewModel("Person", "people",
		Column{"id", "integer"}, Column{"name", "string"}, Column{"age", "integer"})
	m.Dialect = d
	return m
}

func TestInsertUpdateDeleteSQL(t *testing.T) {
	m := personModel(SQLite)
	rec := m.Build(map[string]any{"name": "bob", "age": 30})
	eq(t, "insert", rec.insertSQL(),
		`INSERT INTO "people" ("name", "age") VALUES ('bob', 30) RETURNING "id"`)

	// Persisted + change → UPDATE lists only changed columns in column order.
	loaded := m.Load(map[string]any{"id": int64(1), "name": "bob", "age": 30})
	loaded.Set("age", 31)
	eq(t, "update", loaded.updateSQL(),
		`UPDATE "people" SET "age" = 31 WHERE "people"."id" = 1`)
	eq(t, "delete", loaded.deleteSQL(), `DELETE FROM "people" WHERE "people"."id" = 1`)

	// MySQL omits RETURNING (uses last_insert_id).
	mm := personModel(MySQL)
	rec2 := mm.Build(map[string]any{"name": "x"})
	eq(t, "insert-mysql", rec2.insertSQL(), "INSERT INTO `people` (`name`) VALUES ('x')")
}

func TestSaveCreateSQLiteReturning(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(7)}}}}
	resetTxState(a)
	rec := m.Build(map[string]any{"name": "bob", "age": 30})
	ok, err := Save(a, rec)
	if !ok || err != nil {
		t.Fatalf("save = %v %v", ok, err)
	}
	want := []string{
		"BEGIN immediate TRANSACTION",
		`INSERT INTO "people" ("name", "age") VALUES ('bob', 30) RETURNING "id"`,
		"COMMIT TRANSACTION",
	}
	if len(a.log) != 3 || a.log[0] != want[0] || a.log[1] != want[1] || a.log[2] != want[2] {
		t.Fatalf("log = %#v", a.log)
	}
	if v, _ := rec.Get("id"); v != int64(7) {
		t.Errorf("pk not assigned: %v", v)
	}
	if !rec.IsPersisted() || rec.Changed() {
		t.Error("record should be persisted + clean")
	}
}

func TestSaveCreateMySQLLastID(t *testing.T) {
	m := personModel(MySQL)
	a := &recAdapter{name: "mysql2", lastID: 42}
	resetTxState(a)
	rec := m.Build(map[string]any{"name": "z"})
	ok, err := Save(a, rec)
	if !ok || err != nil {
		t.Fatalf("save = %v %v", ok, err)
	}
	if v, _ := rec.Get("id"); v != int64(42) {
		t.Errorf("pk from lastID = %v", v)
	}
	// BEGIN/COMMIT are the bare mysql forms.
	if a.log[0] != "BEGIN" || a.log[2] != "COMMIT" {
		t.Errorf("mysql tx sql = %#v", a.log)
	}
	// An explicit primary key is not overwritten by lastID.
	a2 := &recAdapter{name: "mysql2", lastID: 99}
	resetTxState(a2)
	rec2 := m.Build(map[string]any{"id": int64(5), "name": "keep"})
	if _, err := Save(a2, rec2); err != nil {
		t.Fatal(err)
	}
	if v, _ := rec2.Get("id"); v != int64(5) {
		t.Errorf("explicit pk overwritten: %v", v)
	}
}

func TestSaveUpdate(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	rec := m.Load(map[string]any{"id": int64(1), "name": "bob", "age": 30})
	rec.Set("age", 31)
	ok, err := Save(a, rec)
	if !ok || err != nil {
		t.Fatalf("save = %v %v", ok, err)
	}
	if a.nonTx()[0] != `UPDATE "people" SET "age" = 31 WHERE "people"."id" = 1` {
		t.Errorf("update sql = %#v", a.nonTx())
	}
	// A save of an unchanged persisted record issues no UPDATE (partial writes).
	a.clear()
	if ok, _ := Save(a, rec); !ok {
		t.Error("no-op save should succeed")
	}
	if len(a.nonTx()) != 0 {
		t.Errorf("unchanged save issued SQL: %#v", a.nonTx())
	}
}

func TestSaveValidationFailure(t *testing.T) {
	m := personModel(SQLite)
	m.ValidatesPresence("name")
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	rec := m.Build(map[string]any{"age": 1})
	ok, err := Save(a, rec)
	if ok || err != nil {
		t.Fatalf("invalid save = %v %v", ok, err)
	}
	if len(a.log) != 0 {
		t.Errorf("invalid save touched db: %#v", a.log)
	}
	if rec.Errors().Empty() {
		t.Error("errors should be populated")
	}
	// validate:false bypasses validation and inserts.
	a.clear()
	a.execRows = [][]Row{{{"id": int64(3)}}}
	if ok, err := Save(a, rec, WithoutValidation()); !ok || err != nil {
		t.Fatalf("save without validation = %v %v", ok, err)
	}
}

func TestCreateAndUpdateHelpers(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1)}}}}
	resetTxState(a)
	rec, err := m.Create(a, map[string]any{"name": "bob", "age": 20})
	if err != nil || !rec.IsPersisted() {
		t.Fatalf("create = %v %v", rec.IsPersisted(), err)
	}
	a.clear()
	ok, err := rec.Update(a, map[string]any{"age": 21})
	if !ok || err != nil {
		t.Fatalf("update = %v %v", ok, err)
	}
	if a.nonTx()[0] != `UPDATE "people" SET "age" = 21 WHERE "people"."id" = 1` {
		t.Errorf("update = %#v", a.nonTx())
	}
}

func TestDestroyAndDelete(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	rec := m.Load(map[string]any{"id": int64(1), "name": "bob"})
	ok, err := Destroy(a, rec)
	if !ok || err != nil {
		t.Fatalf("destroy = %v %v", ok, err)
	}
	if !rec.IsDestroyed() || rec.IsPersisted() {
		t.Error("record should be destroyed")
	}
	if a.log[0] != "BEGIN immediate TRANSACTION" ||
		a.nonTx()[0] != `DELETE FROM "people" WHERE "people"."id" = 1` {
		t.Errorf("destroy log = %#v", a.log)
	}

	// Delete: single statement, no transaction.
	a.clear()
	rec2 := m.Load(map[string]any{"id": int64(2), "name": "x"})
	if err := Delete(a, rec2); err != nil {
		t.Fatal(err)
	}
	if len(a.log) != 1 || a.log[0] != `DELETE FROM "people" WHERE "people"."id" = 2` {
		t.Errorf("delete log = %#v", a.log)
	}
	if !rec2.IsDestroyed() {
		t.Error("deleted record marked destroyed")
	}
}

func TestTimestamps(t *testing.T) {
	at := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	withClock(t, at)
	m := NewModel("Post", "posts",
		Column{"id", "integer"}, Column{"title", "string"},
		Column{"created_at", "datetime"}, Column{"updated_at", "datetime"})
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1)}}}}
	resetTxState(a)
	rec := m.Build(map[string]any{"title": "hi"})
	if _, err := Save(a, rec); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.Get("created_at"); !ok {
		t.Error("created_at unset")
	}
	eq(t, "insert-ts", a.nonTx()[0],
		`INSERT INTO "posts" ("title", "created_at", "updated_at") VALUES ('hi', '2026-07-11 12:00:00', '2026-07-11 12:00:00') RETURNING "id"`)

	// Update touches updated_at (only) when another column changed.
	a.clear()
	later := at.Add(time.Hour)
	nowFunc = func() time.Time { return later }
	rec.Set("title", "bye")
	if _, err := Save(a, rec); err != nil {
		t.Fatal(err)
	}
	eq(t, "update-ts", a.nonTx()[0],
		`UPDATE "posts" SET "title" = 'bye', "updated_at" = '2026-07-11 13:00:00' WHERE "posts"."id" = 1`)

	// A create that supplies created_at keeps it (no overwrite).
	a.clear()
	a.execRows = [][]Row{{{"id": int64(2)}}}
	custom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	rec3 := m.Build(map[string]any{"title": "t", "created_at": custom})
	if _, err := Save(a, rec3); err != nil {
		t.Fatal(err)
	}
	if v, _ := rec3.Get("created_at"); !v.(time.Time).Equal(custom) {
		t.Errorf("created_at overwritten: %v", v)
	}
}

func TestSaveErrorPaths(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", failOn: "INSERT"}
	resetTxState(a)
	rec := m.Build(map[string]any{"name": "bob"})
	ok, err := Save(a, rec)
	if ok || err == nil {
		t.Fatalf("expected insert error, got %v %v", ok, err)
	}
	// The transaction rolled back after the failed INSERT.
	if a.log[len(a.log)-1] != "ROLLBACK TRANSACTION" {
		t.Errorf("no rollback: %#v", a.log)
	}

	// DML error on update.
	a2 := &recAdapter{name: "sqlite3", failOn: "UPDATE"}
	resetTxState(a2)
	rec2 := m.Load(map[string]any{"id": int64(1), "name": "a"})
	rec2.Set("name", "b")
	if ok, err := Save(a2, rec2); ok || err == nil {
		t.Fatalf("expected update error, got %v %v", ok, err)
	}

	// Destroy DML error.
	a3 := &recAdapter{name: "sqlite3", failOn: "DELETE"}
	resetTxState(a3)
	rec3 := m.Load(map[string]any{"id": int64(1), "name": "a"})
	if ok, err := Destroy(a3, rec3); ok || err == nil {
		t.Fatalf("expected destroy error, got %v %v", ok, err)
	}

	// Delete DML error propagates and does not mark destroyed.
	a4 := &recAdapter{name: "sqlite3", failOn: "DELETE"}
	rec4 := m.Load(map[string]any{"id": int64(1), "name": "a"})
	if err := Delete(a4, rec4); err == nil || rec4.IsDestroyed() {
		t.Errorf("delete error handling: %v %v", err, rec4.IsDestroyed())
	}
}

func TestExecuteInsertReturningError(t *testing.T) {
	// Execute (RETURNING path) error is surfaced.
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", failOn: "INSERT", failExec: true}
	if err := m.executeInsert(a, m.Build(map[string]any{"name": "x"})); err == nil {
		t.Error("expected returning execute error")
	}
	// RETURNING path with no rows returned leaves pk unset (no panic).
	a2 := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}}
	rec := m.Build(map[string]any{"name": "x"})
	if err := m.executeInsert(a2, rec); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.Get("id"); ok {
		t.Error("pk should be unset when no row returned")
	}
	// MySQL lastID error path.
	mm := personModel(MySQL)
	a3 := &recAdapter{name: "mysql2", failOn: "INSERT", failDML: true}
	if err := mm.executeInsert(a3, mm.Build(map[string]any{"name": "x"})); err == nil {
		t.Error("expected mysql insert dml error")
	}
}

// A save whose after_commit callback errors returns (true, err).
func TestSaveAfterCommitError(t *testing.T) {
	m := personModel(SQLite)
	m.AfterCommit(func(*Record) error { return errors.New("commit hook") })
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1)}}}}
	resetTxState(a)
	ok, err := Save(a, m.Build(map[string]any{"name": "x"}))
	if !ok || err == nil {
		t.Fatalf("after_commit error = %v %v", ok, err)
	}
}
