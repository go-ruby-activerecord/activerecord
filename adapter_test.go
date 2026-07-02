// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"errors"
	"testing"
)

// fakeAdapter is an in-memory Adapter that records the SQL it is handed and
// replays a fixed result, exercising the host-seam wiring deterministically.
type fakeAdapter struct {
	name     string
	rows     []Row
	execErr  error
	lastSQL  string
	dmlErr   error
	affected int64
	lastID   int64
}

func (f *fakeAdapter) Execute(sql string) ([]Row, error) {
	f.lastSQL = sql
	return f.rows, f.execErr
}
func (f *fakeAdapter) ExecuteDML(sql string) (int64, int64, error) {
	f.lastSQL = sql
	return f.affected, f.lastID, f.dmlErr
}
func (f *fakeAdapter) AdapterName() string { return f.name }

func TestAdapterSeam(t *testing.T) {
	u := NewModel("User", "users", Column{"id", "integer"}, Column{"name", "string"})
	fa := &fakeAdapter{name: "postgresql"}
	if DialectFor(fa) != Postgres {
		t.Error("DialectFor")
	}

	// Exists true / false / error.
	fa.rows = []Row{{"one": int64(1)}}
	if ok, err := Exists(fa, u.All()); err != nil || !ok {
		t.Errorf("exists true = %v %v", ok, err)
	}
	if fa.lastSQL != `SELECT 1 AS one FROM "users" LIMIT 1` {
		t.Errorf("exists sql = %s", fa.lastSQL)
	}
	fa.rows = nil
	if ok, _ := Exists(fa, u.All()); ok {
		t.Error("exists false")
	}
	fa.execErr = errors.New("boom")
	if _, err := Exists(fa, u.All()); err == nil {
		t.Error("exists error")
	}
	fa.execErr = nil

	// Count scalar / empty / error.
	fa.rows = []Row{{"count": int64(3)}}
	if n, err := Count(fa, u.All()); err != nil || n != 3 {
		t.Errorf("count = %v %v", n, err)
	}
	fa.rows = []Row{}
	if n, _ := Count(fa, u.All()); n != 0 {
		t.Error("count empty")
	}
	fa.rows = nil
	fa.execErr = errors.New("x")
	if _, err := Count(fa, u.All()); err == nil {
		t.Error("count error")
	}
	fa.execErr = nil
	// count with a float scalar (some drivers return float).
	fa.rows = []Row{{"c": 4.0}}
	if n, _ := Count(fa, u.All()); n != 4 {
		t.Error("count float")
	}
	// count with empty row map yields 0 via fallthrough.
	fa.rows = []Row{{}}
	if n, _ := Count(fa, u.All()); n != 0 {
		t.Error("count empty row")
	}

	// LoadAll materializes persisted records.
	fa.rows = []Row{{"id": int64(1), "name": "a"}, {"id": int64(2), "name": "b"}}
	recs, err := LoadAll(fa, u.All())
	if err != nil || len(recs) != 2 {
		t.Fatalf("loadall = %v %v", len(recs), err)
	}
	if v, _ := recs[0].Get("name"); v != "a" {
		t.Errorf("rec0 = %v", v)
	}
	if recs[0].Changed() {
		t.Error("loaded record clean")
	}
	fa.execErr = errors.New("y")
	if _, err := LoadAll(fa, u.All()); err == nil {
		t.Error("loadall error")
	}
}

func TestToInt64(t *testing.T) {
	if toInt64(int64(1)) != 1 || toInt64(int(2)) != 2 || toInt64(int32(3)) != 3 ||
		toInt64(4.0) != 4 || toInt64("5") != 5 || toInt64([]any{}) != 0 {
		t.Error("toInt64")
	}
}
