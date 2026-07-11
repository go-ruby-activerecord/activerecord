// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

// This file drives the last remaining branches of the new execution subsystems
// to keep the deterministic suite at 100% statement coverage.
package activerecord

import (
	"errors"
	"testing"
	"time"
)

func TestRecordStateAccessors(t *testing.T) {
	m := personModel(SQLite)
	built := m.Build(map[string]any{"name": "x"})
	if !built.IsNewRecord() || built.IsPersisted() {
		t.Error("built record should be new")
	}
	loaded := m.Load(map[string]any{"id": int64(1), "name": "x"})
	if loaded.IsNewRecord() || !loaded.IsPersisted() {
		t.Error("loaded record should be persisted")
	}
	// Errors() on a record that has not been validated yields an empty set.
	if !built.Errors().Empty() {
		t.Error("fresh Errors should be empty")
	}
}

func TestInPredicateNegatedSingle(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "not-single", u.All().Not(map[string]any{"age": []any{1}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" != 1`)
}

func TestScopeNamesAndDefaultScope(t *testing.T) {
	m := personModel(SQLite)
	m.Scope("adults", func(r *Relation) *Relation { return r.Where("age >= 18") }).
		Scope("named", func(r *Relation) *Relation { return r })
	names := m.ScopeNames()
	if len(names) != 2 || names[0] != "adults" || names[1] != "named" {
		t.Errorf("scope names = %v", names)
	}
	// default_scope refines every relation started from the model.
	m.DefaultScope(func(r *Relation) *Relation { return r.Where(map[string]any{"active": true}).Order("name") })
	eq(t, "default-scope", m.All().ToSQL(),
		`SELECT "people".* FROM "people" WHERE "people"."active" = TRUE ORDER BY "people"."name" ASC`)
	// Unscoped bypasses it.
	eq(t, "unscoped", m.Unscoped().ToSQL(), `SELECT "people".* FROM "people"`)
}

func TestBeforeUpdateError(t *testing.T) {
	sentinel := errors.New("boom")
	m := personModel(SQLite)
	m.BeforeUpdate(func(*Record) error { return sentinel })
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	rec := m.Load(map[string]any{"id": int64(1), "name": "a"})
	rec.Set("name", "b")
	if ok, err := Save(a, rec); ok || !errors.Is(err, sentinel) {
		t.Fatalf("before_update error = %v %v", ok, err)
	}
}

func TestCreateError(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", failOn: "INSERT"}
	resetTxState(a)
	rec, err := m.Create(a, map[string]any{"name": "x"})
	if err == nil || rec.IsPersisted() {
		t.Errorf("create error = %v persisted=%v", err, rec.IsPersisted())
	}
}

func TestInsertSQLNilPrimaryKey(t *testing.T) {
	m := personModel(SQLite)
	// An explicit nil primary key is omitted from the INSERT column list.
	rec := m.Build(map[string]any{"id": nil, "name": "x"})
	eq(t, "nil-pk", rec.insertSQL(),
		`INSERT INTO "people" ("name") VALUES ('x') RETURNING "id"`)
}

func TestTouchTimestampsUnchangedUpdate(t *testing.T) {
	at := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	withClock(t, at)
	m := NewModel("Post", "posts",
		Column{"id", "integer"}, Column{"title", "string"}, Column{"updated_at", "datetime"})
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	rec := m.Load(map[string]any{"id": int64(1), "title": "t", "updated_at": at})
	// No change → updated_at is NOT touched and no UPDATE is issued.
	if _, err := Save(a, rec); err != nil {
		t.Fatal(err)
	}
	if len(a.nonTx()) != 0 {
		t.Errorf("unchanged timestamped save issued SQL: %#v", a.nonTx())
	}
}

func TestPreloadBelongsToEmptyAndDedup(t *testing.T) {
	user, post, _, _ := testModels(SQLite)
	_ = user
	// Parents without the foreign key → no query.
	a := &recAdapter{name: "sqlite3"}
	if err := Preload(a, []*Record{post.Load(map[string]any{"id": int64(1)})}, "user"); err != nil {
		t.Fatal(err)
	}
	if len(a.log) != 0 {
		t.Errorf("empty fk preload issued SQL: %#v", a.log)
	}
	// Duplicate foreign keys are de-duplicated in the IN set.
	a2 := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1), "name": "a"}}}}
	p1 := post.Load(map[string]any{"id": int64(10), "user_id": int64(1)})
	p2 := post.Load(map[string]any{"id": int64(11), "user_id": int64(1)})
	if err := Preload(a2, []*Record{p1, p2}, "user"); err != nil {
		t.Fatal(err)
	}
	eq(t, "dedup", a2.log[0], `SELECT "users".* FROM "users" WHERE "users"."id" = 1`)
}

func TestPreloadHABTMEmptyJoin(t *testing.T) {
	user, _, _, _ := testModels(SQLite)
	// Join table returns no rows → no target query, nothing attached.
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}}
	u := user.Load(map[string]any{"id": int64(1)})
	if err := Preload(a, []*Record{u}, "roles"); err != nil {
		t.Fatal(err)
	}
	if len(a.log) != 1 {
		t.Errorf("empty-join HABTM should stop after join query: %#v", a.log)
	}
	// Parents without a primary key → no query at all.
	a2 := &recAdapter{name: "sqlite3"}
	if err := Preload(a2, []*Record{user.Load(map[string]any{"name": "no-id"})}, "roles"); err != nil {
		t.Fatal(err)
	}
	if len(a2.log) != 0 {
		t.Errorf("pk-less HABTM issued SQL: %#v", a2.log)
	}
}

func TestPreloadThroughSourceFallback(t *testing.T) {
	// The through association name ("writings") is not defined on the intermediate;
	// the source falls back to the underscored target class name ("article").
	company := NewModel("Company", "companies", Column{"id", "integer"})
	user := NewModel("User", "users", Column{"id", "integer"}, Column{"company_id", "bigint"})
	article := NewModel("Article", "articles", Column{"id", "integer"}, Column{"user_id", "bigint"})
	company.Register(user, article)
	user.Register(company, article)
	company.HasMany("users", "User").HasMany("writings", "Article", Through("users"))
	user.HasMany("article", "Article") // source reflection by class-name stem
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{
		{{"id": int64(1), "company_id": int64(1)}},
		{{"id": int64(10), "user_id": int64(1)}},
	}}
	c := company.Load(map[string]any{"id": int64(1)})
	if err := Preload(a, []*Record{c}, "writings"); err != nil {
		t.Fatal(err)
	}
	if got := c.PreloadedAssociation("writings"); len(got) != 1 {
		t.Fatalf("through-fallback = %d", len(got))
	}
}
