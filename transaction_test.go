// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"errors"
	"testing"
)

func TestTransactionCommit(t *testing.T) {
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	err := Transaction(a, func() error {
		if _, _, e := a.ExecuteDML("INSERT INTO x DEFAULT VALUES"); e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"BEGIN immediate TRANSACTION", "INSERT INTO x DEFAULT VALUES", "COMMIT TRANSACTION"}
	if len(a.log) != 3 || a.log[0] != want[0] || a.log[2] != want[2] {
		t.Errorf("commit log = %#v", a.log)
	}
	if TransactionDepth(a) != 0 {
		t.Errorf("depth leaked: %d", TransactionDepth(a))
	}
}

func TestTransactionRollbackSentinel(t *testing.T) {
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	if err := Transaction(a, func() error { return ErrRollback }); err != nil {
		t.Fatalf("ErrRollback should be swallowed: %v", err)
	}
	if a.log[len(a.log)-1] != "ROLLBACK TRANSACTION" {
		t.Errorf("no rollback: %#v", a.log)
	}
}

func TestTransactionErrorPropagates(t *testing.T) {
	a := &recAdapter{name: "postgresql"}
	resetTxState(a)
	sentinel := errors.New("boom")
	if err := Transaction(a, func() error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("error should propagate: %v", err)
	}
	// Postgres uses the bare BEGIN/ROLLBACK forms.
	if a.log[0] != "BEGIN" || a.log[len(a.log)-1] != "ROLLBACK" {
		t.Errorf("pg tx sql = %#v", a.log)
	}
}

func TestTransactionSavepointNesting(t *testing.T) {
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	err := Transaction(a, func() error {
		if d := TransactionDepth(a); d != 1 {
			t.Errorf("outer depth = %d", d)
		}
		return Transaction(a, func() error {
			if d := TransactionDepth(a); d != 2 {
				t.Errorf("inner depth = %d", d)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"BEGIN immediate TRANSACTION",
		"SAVEPOINT active_record_1",
		"RELEASE SAVEPOINT active_record_1",
		"COMMIT TRANSACTION",
	}
	for i, w := range want {
		if a.log[i] != w {
			t.Errorf("nesting[%d] = %q want %q", i, a.log[i], w)
		}
	}
}

func TestTransactionSavepointRollback(t *testing.T) {
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	err := Transaction(a, func() error {
		return Transaction(a, func() error { return ErrRollback })
	})
	if err != nil {
		t.Fatal(err)
	}
	// Inner rolls back to the savepoint; outer commits.
	want := []string{
		"BEGIN immediate TRANSACTION",
		"SAVEPOINT active_record_1",
		"ROLLBACK TO SAVEPOINT active_record_1",
		"COMMIT TRANSACTION",
	}
	for i, w := range want {
		if a.log[i] != w {
			t.Errorf("sp-rollback[%d] = %q want %q", i, a.log[i], w)
		}
	}
}

func TestTransactionPanicRollsBack(t *testing.T) {
	a := &recAdapter{name: "sqlite3"}
	resetTxState(a)
	defer func() {
		if r := recover(); r == nil {
			t.Error("panic should propagate")
		}
		if a.log[len(a.log)-1] != "ROLLBACK TRANSACTION" {
			t.Errorf("panic did not roll back: %#v", a.log)
		}
		if TransactionDepth(a) != 0 {
			t.Errorf("depth leaked after panic: %d", TransactionDepth(a))
		}
	}()
	_ = Transaction(a, func() error { panic("kaboom") })
}

func TestTransactionBeginError(t *testing.T) {
	a := &recAdapter{name: "sqlite3", failOn: "BEGIN"}
	resetTxState(a)
	if err := Transaction(a, func() error { return nil }); err == nil {
		t.Error("begin error should propagate")
	}
	if TransactionDepth(a) != 0 {
		t.Errorf("depth leaked after begin error: %d", TransactionDepth(a))
	}
}

func TestTransactionCommitError(t *testing.T) {
	// A failing COMMIT triggers a rollback and surfaces the commit error.
	a := &recAdapter{name: "sqlite3", failOn: "COMMIT"}
	resetTxState(a)
	if err := Transaction(a, func() error { return nil }); err == nil {
		t.Error("commit error should propagate")
	}
	if a.log[len(a.log)-1] != "ROLLBACK TRANSACTION" {
		t.Errorf("commit failure should roll back: %#v", a.log)
	}
}

func TestTransactionRollbackErrorOnFailure(t *testing.T) {
	// When both fn and the rollback fail, the rollback error is returned.
	a := &recAdapter{name: "sqlite3", failOn: "ROLLBACK"}
	resetTxState(a)
	err := Transaction(a, func() error { return errors.New("fn boom") })
	if err == nil {
		t.Error("expected rollback error")
	}
}

func TestTransactionCommitAndRollbackBothFail(t *testing.T) {
	// COMMIT fails, then the compensating ROLLBACK also fails: the rollback error
	// is what surfaces.
	a := &recAdapter{name: "postgresql", failOn: "COMMIT", failAlso: "ROLLBACK"}
	resetTxState(a)
	if err := Transaction(a, func() error { return nil }); err == nil {
		t.Error("expected rollback error after failed commit")
	}
}

func TestTransactionSQLPerDialect(t *testing.T) {
	if s := MySQL.beginTransactionSQL(); s != "BEGIN" {
		t.Errorf("mysql begin = %q", s)
	}
	if s := MySQL.commitTransactionSQL(); s != "COMMIT" {
		t.Errorf("mysql commit = %q", s)
	}
	if s := MySQL.rollbackTransactionSQL(); s != "ROLLBACK" {
		t.Errorf("mysql rollback = %q", s)
	}
	if s := SQLite.rollbackTransactionSQL(); s != "ROLLBACK TRANSACTION" {
		t.Errorf("sqlite rollback = %q", s)
	}
}
