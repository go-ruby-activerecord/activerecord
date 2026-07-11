// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"errors"
	"reflect"
	"testing"
)

func TestCallbackLifecycleOrder(t *testing.T) {
	var seq []string
	rec2 := func(s string) Callback { return func(*Record) error { seq = append(seq, s); return nil } }
	m := personModel(SQLite)
	m.BeforeValidation(rec2("bv")).AfterValidation(rec2("av")).
		BeforeSave(rec2("bs1")).BeforeSave(rec2("bs2")).
		BeforeCreate(rec2("bc")).AfterCreate(rec2("ac")).
		AfterSave(rec2("as1")).AfterSave(rec2("as2")).
		AfterCommit(rec2("commit"))
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1)}}}}
	resetTxState(a)
	if ok, err := Save(a, m.Build(map[string]any{"name": "x"})); !ok || err != nil {
		t.Fatalf("save = %v %v", ok, err)
	}
	// before_save fires in registration order; after_save in reverse (LIFO).
	want := []string{"bv", "av", "bs1", "bs2", "bc", "ac", "as2", "as1", "commit"}
	if !reflect.DeepEqual(seq, want) {
		t.Errorf("order:\n got %v\nwant %v", seq, want)
	}
}

func TestCallbackUpdateOrder(t *testing.T) {
	var seq []string
	cb := func(s string) Callback { return func(*Record) error { seq = append(seq, s); return nil } }
	m := personModel(SQLite)
	m.BeforeSave(cb("bs")).BeforeUpdate(cb("bu")).AfterUpdate(cb("au")).AfterSave(cb("as"))
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	rec := m.Load(map[string]any{"id": int64(1), "name": "a"})
	rec.Set("name", "b")
	if _, err := Save(a, rec); err != nil {
		t.Fatal(err)
	}
	want := []string{"bs", "bu", "au", "as"}
	if !reflect.DeepEqual(seq, want) {
		t.Errorf("update order = %v", seq)
	}
}

func TestCallbackHaltBeforeSave(t *testing.T) {
	m := personModel(SQLite)
	fired := false
	m.BeforeSave(func(*Record) error { return ErrAbort }).
		AfterSave(func(*Record) error { fired = true; return nil }).
		AfterRollback(func(*Record) error { fired = fired || true; return nil })
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	ok, err := Save(a, m.Build(map[string]any{"name": "x"}))
	if ok || err != nil {
		t.Fatalf("halt should be soft failure, got %v %v", ok, err)
	}
	if len(a.nonTx()) != 0 {
		t.Errorf("halt issued DML: %#v", a.nonTx())
	}
	if a.log[len(a.log)-1] != "ROLLBACK TRANSACTION" {
		t.Errorf("halt should roll back: %#v", a.log)
	}
}

func TestCallbackHaltBeforeValidation(t *testing.T) {
	m := personModel(SQLite)
	m.BeforeValidation(func(*Record) error { return ErrAbort })
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	ok, err := Save(a, m.Build(map[string]any{"name": "x"}))
	if ok || err != nil || len(a.log) != 0 {
		t.Fatalf("halted validation = %v %v log=%v", ok, err, a.log)
	}
}

func TestCallbackErrorPropagation(t *testing.T) {
	sentinel := errors.New("boom")
	cases := []struct {
		name string
		wire func(*Model)
	}{
		{"before_validation", func(m *Model) { m.BeforeValidation(func(*Record) error { return sentinel }) }},
		{"after_validation", func(m *Model) { m.AfterValidation(func(*Record) error { return sentinel }) }},
		{"before_create", func(m *Model) { m.BeforeCreate(func(*Record) error { return sentinel }) }},
		{"after_create", func(m *Model) { m.AfterCreate(func(*Record) error { return sentinel }) }},
	}
	for _, tc := range cases {
		m := personModel(SQLite)
		tc.wire(m)
		a := &recAdapter{name: "sqlite3", execRows: [][]Row{{{"id": int64(1)}}}}
		resetTxState(a)
		ok, err := Save(a, m.Build(map[string]any{"name": "x"}))
		if ok || !errors.Is(err, sentinel) {
			t.Errorf("%s: got %v %v", tc.name, ok, err)
		}
	}
}

func TestCallbackDestroyHaltAndError(t *testing.T) {
	// Halt in before_destroy is a soft failure.
	m := personModel(SQLite)
	m.BeforeDestroy(func(*Record) error { return ErrAbort })
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	rec := m.Load(map[string]any{"id": int64(1), "name": "x"})
	if ok, err := Destroy(a, rec); ok || err != nil || rec.IsDestroyed() {
		t.Fatalf("halted destroy = %v %v destroyed=%v", ok, err, rec.IsDestroyed())
	}

	// after_destroy error propagates.
	sentinel := errors.New("boom")
	m2 := personModel(SQLite)
	m2.AfterDestroy(func(*Record) error { return sentinel })
	a2 := &recAdapter{name: "sqlite3"}
	resetTxState(a2)
	rec2 := m2.Load(map[string]any{"id": int64(1), "name": "x"})
	if ok, err := Destroy(a2, rec2); ok || !errors.Is(err, sentinel) {
		t.Fatalf("after_destroy error = %v %v", ok, err)
	}

	// after_commit error on destroy returns (true, err).
	m3 := personModel(SQLite)
	m3.AfterCommit(func(*Record) error { return sentinel })
	a3 := &recAdapter{name: "sqlite3"}
	resetTxState(a3)
	rec3 := m3.Load(map[string]any{"id": int64(1), "name": "x"})
	if ok, err := Destroy(a3, rec3); !ok || !errors.Is(err, sentinel) {
		t.Fatalf("destroy after_commit error = %v %v", ok, err)
	}
}

// A model with no callbacks registered exercises the nil-store fast path.
func TestCallbackNilStore(t *testing.T) {
	m := personModel(SQLite)
	if err := m.runBefore(beforeSaveCB, m.Build(nil)); err != nil {
		t.Error(err)
	}
	if err := m.runAfter(afterSaveCB, m.Build(nil)); err != nil {
		t.Error(err)
	}
}
