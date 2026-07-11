// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func TestFindStatementSQL(t *testing.T) {
	m := personModel(SQLite)
	eq(t, "find", m.FindStatement().SQL,
		`SELECT "people".* FROM "people" WHERE "people"."id" = ? LIMIT ?`)
	eq(t, "find_by", m.FindByStatement("name").SQL,
		`SELECT "people".* FROM "people" WHERE "people"."name" = ? LIMIT ?`)
	eq(t, "find_by2", m.FindByStatement("name", "age").SQL,
		`SELECT "people".* FROM "people" WHERE "people"."name" = ? AND "people"."age" = ? LIMIT ?`)

	// Postgres uses positional $n markers.
	pm := personModel(Postgres)
	eq(t, "find-pg", pm.FindStatement().SQL,
		`SELECT "people".* FROM "people" WHERE "people"."id" = $1 LIMIT $2`)
	eq(t, "find_by-pg", pm.FindByStatement("name", "age").SQL,
		`SELECT "people".* FROM "people" WHERE "people"."name" = $1 AND "people"."age" = $2 LIMIT $3`)
}

func TestStatementCacheReuse(t *testing.T) {
	m := personModel(SQLite)
	s1 := m.FindStatement()
	s2 := m.FindStatement()
	if s1 != s2 {
		t.Error("statement not cached")
	}
	c := NewStatementCache()
	calls := 0
	build := func() *PreparedStatement { calls++; return &PreparedStatement{model: m, SQL: "x"} }
	c.Fetch("k", build)
	c.Fetch("k", build)
	if calls != 1 {
		t.Errorf("build called %d times", calls)
	}
}

func TestPreparedExecuteInline(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(5), "name": "bob"}}}}
	rec, err := m.FindStatement().ExecuteOne(a, int64(5))
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil {
		t.Fatal("nil record")
	}
	// The plain adapter received the inlined SQL (binds baked in, LIMIT 1).
	eq(t, "inline", a.log[0], `SELECT "people".* FROM "people" WHERE "people"."id" = 5 LIMIT 1`)
	if v, _ := rec.Get("name"); v != "bob" {
		t.Errorf("rec = %v", v)
	}

	// Postgres inlines $n markers.
	pm := personModel(Postgres)
	pa := &recAdapter{name: "postgresql", execRows: [][]Row{{{"id": int64(9)}}}}
	if _, err := pm.FindByStatement("name", "age").Execute(pa, "bob", 30); err != nil {
		t.Fatal(err)
	}
	eq(t, "inline-pg", pa.log[0],
		`SELECT "people".* FROM "people" WHERE "people"."name" = 'bob' AND "people"."age" = 30 LIMIT 1`)
}

func TestPreparedExecutePreparedAdapter(t *testing.T) {
	m := personModel(SQLite)
	a := &prepAdapter{recAdapter: recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(5)}}}}}
	recs, err := m.FindStatement().Execute(a, int64(5))
	if err != nil || len(recs) != 1 {
		t.Fatalf("prepared execute = %v %v", len(recs), err)
	}
	// The template SQL is untouched and the binds are supplied separately: [id, 1].
	eq(t, "template", a.lastPreparedSQL, `SELECT "people".* FROM "people" WHERE "people"."id" = ? LIMIT ?`)
	if len(a.lastBinds) != 2 || a.lastBinds[0] != int64(5) || a.lastBinds[1] != 1 {
		t.Errorf("binds = %#v", a.lastBinds)
	}
}

func TestPreparedExecuteEmptyAndError(t *testing.T) {
	m := personModel(SQLite)
	// No rows → ExecuteOne returns nil.
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}}
	if rec, err := m.FindStatement().ExecuteOne(a, int64(1)); err != nil || rec != nil {
		t.Fatalf("empty = %v %v", rec, err)
	}
	// Fewer supplied binds than non-fixed slots → nil-filled, still runs.
	a2 := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}}
	if _, err := m.FindStatement().Execute(a2); err != nil {
		t.Fatal(err)
	}
	// Execute error propagates through ExecuteOne.
	a3 := &recAdapter{name: "sqlite3", failOn: "SELECT"}
	if _, err := m.FindStatement().ExecuteOne(a3, int64(1)); err == nil {
		t.Error("expected execute error")
	}
}
