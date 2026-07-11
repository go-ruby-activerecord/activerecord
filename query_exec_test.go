// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"errors"
	"testing"
)

func TestPluckAndIds(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{
		{"name": "a", "age": int64(1)}, {"name": "b", "age": int64(2)},
	}}}
	rows, err := m.All().Pluck(a, "name", "age", 99) // non-name arg ignored
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "pluck-sql", a.log[0], `SELECT "people"."name", "people"."age" FROM "people"`)
	if len(rows) != 2 || rows[0][0] != "a" || rows[1][1] != int64(2) {
		t.Errorf("pluck = %#v", rows)
	}
	a.clear()
	a.execRows = [][]Row{{{"id": int64(5)}, {"id": int64(6)}}}
	ids, err := m.All().Ids(a)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "ids-sql", a.log[0], `SELECT "people"."id" FROM "people"`)
	if len(ids) != 2 || ids[0] != int64(5) {
		t.Errorf("ids = %#v", ids)
	}
}

func TestExistsCountToArray(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"one": int64(1)}}}}
	if ok, err := m.All().Exists(a); err != nil || !ok {
		t.Errorf("exists = %v %v", ok, err)
	}
	a.clear()
	a.execRows = [][]Row{{{"count": int64(7)}}}
	if n, err := m.All().Count(a); err != nil || n != 7 {
		t.Errorf("count = %v %v", n, err)
	}
	a.clear()
	a.execRows = [][]Row{{{"id": int64(1), "name": "a"}}}
	recs, err := m.All().ToArray(a)
	if err != nil || len(recs) != 1 {
		t.Errorf("to_a = %v %v", len(recs), err)
	}
}

func TestFirstLastTake(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1), "name": "a"}}}}
	rec, err := m.All().FirstRecord(a)
	if err != nil || rec == nil {
		t.Fatalf("first = %v %v", rec, err)
	}
	eq(t, "first-sql", a.log[0],
		`SELECT "people".* FROM "people" ORDER BY "people"."id" ASC LIMIT 1`)
	a.clear()
	a.execRows = [][]Row{{{"id": int64(9), "name": "z"}}}
	if _, err := m.All().LastRecord(a); err != nil {
		t.Fatal(err)
	}
	eq(t, "last-sql", a.log[0],
		`SELECT "people".* FROM "people" ORDER BY "people"."id" DESC LIMIT 1`)
	a.clear()
	a.execRows = [][]Row{{{"id": int64(3)}}}
	if _, err := m.All().TakeRecord(a); err != nil {
		t.Fatal(err)
	}
	eq(t, "take-sql", a.log[0], `SELECT "people".* FROM "people" LIMIT 1`)
	// Empty result → nil record.
	a.clear()
	a.execRows = [][]Row{{}}
	if rec, err := m.All().FirstRecord(a); err != nil || rec != nil {
		t.Errorf("empty first = %v %v", rec, err)
	}
}

func TestFindRecordAndFindBy(t *testing.T) {
	m := personModel(SQLite)
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(5), "name": "bob"}}}}
	rec, err := m.FindRecord(a, int64(5))
	if err != nil || rec == nil {
		t.Fatalf("find = %v %v", rec, err)
	}
	eq(t, "find-sql", a.log[0], `SELECT "people".* FROM "people" WHERE "people"."id" = 5 LIMIT 1`)
	a.clear()
	a.execRows = [][]Row{{{"id": int64(1), "name": "bob"}}}
	if _, err := m.FindByRecord(a, map[string]any{"name": "bob"}); err != nil {
		t.Fatal(err)
	}
	eq(t, "find-by-sql", a.log[0],
		`SELECT "people".* FROM "people" WHERE "people"."name" = 'bob' LIMIT 1`)
}

func TestFindEach(t *testing.T) {
	m := personModel(SQLite)
	// Two full batches of 2, then a short batch of 1 → iteration stops.
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{
		{{"id": int64(1)}, {"id": int64(2)}},
		{{"id": int64(3)}, {"id": int64(4)}},
		{{"id": int64(5)}},
	}}
	var seen []any
	if err := m.All().FindEach(a, 2, func(r *Record) error {
		v, _ := r.Get("id")
		seen = append(seen, v)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 5 {
		t.Errorf("find_each saw %d", len(seen))
	}
	eq(t, "batch-sql", a.log[0],
		`SELECT "people".* FROM "people" ORDER BY "people"."id" ASC LIMIT 2 OFFSET 0`)
	// fn error stops iteration.
	a2 := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1)}, {"id": int64(2)}}}}
	boom := errors.New("stop")
	if err := m.All().FindEach(a2, 2, func(*Record) error { return boom }); !errors.Is(err, boom) {
		t.Errorf("find_each fn error = %v", err)
	}
	// Default batch size when non-positive.
	a3 := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}}
	if err := m.All().FindEach(a3, 0, func(*Record) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if a3.log[0] != `SELECT "people".* FROM "people" ORDER BY "people"."id" ASC LIMIT 1000 OFFSET 0` {
		t.Errorf("default batch sql = %s", a3.log[0])
	}
}

func TestQueryExecErrorPaths(t *testing.T) {
	m := personModel(SQLite)
	fail := func() *recAdapter { return &recAdapter{name: "sqlite3", failOn: "SELECT"} }
	if _, err := m.All().Pluck(fail(), "name"); err == nil {
		t.Error("pluck error")
	}
	if _, err := m.All().Ids(fail()); err == nil {
		t.Error("ids error")
	}
	if _, err := m.All().ToArray(fail()); err == nil {
		t.Error("to_a error")
	}
	if _, err := m.All().FirstRecord(fail()); err == nil {
		t.Error("first error")
	}
	if _, err := m.FindByRecord(fail(), map[string]any{"name": "x"}); err == nil {
		t.Error("find_by error")
	}
	if err := m.All().FindEach(fail(), 2, func(*Record) error { return nil }); err == nil {
		t.Error("find_each error")
	}
}
