// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file implements ActiveRecord's transaction execution — the half the
// README used to defer to the host. It drives real BEGIN/COMMIT/ROLLBACK and
// nested SAVEPOINT/RELEASE/ROLLBACK-TO statements through the injected [Adapter],
// matching ActiveRecord's connection-level transaction manager:
//
//   - The outermost Transaction opens a real DB transaction (a dialect-specific
//     BEGIN); a nested Transaction opens a SAVEPOINT named active_record_<n>
//     (n = nesting depth), exactly as ActiveRecord's savepoint transactions.
//   - Returning [ErrRollback] from the block rolls the (sub)transaction back and
//     is swallowed (Transaction returns nil), mirroring `raise
//     ActiveRecord::Rollback`. Any other error rolls back and propagates.
//   - A panic rolls back and re-panics, so a host's Ruby exception unwinding is
//     transaction-safe.
//
// Transaction state is tracked per connection (per Adapter value) so persistence
// (see persistence.go) can transparently open a savepoint when a Save runs
// inside a user Transaction, just as ActiveRecord nests record saves.

import (
	"errors"
	"strconv"
	"sync"
)

// ErrRollback is the pure-Go equivalent of `raise ActiveRecord::Rollback`:
// returning it from a [Transaction] block rolls that transaction back but is not
// re-raised to the caller (Transaction returns nil).
var ErrRollback = errors.New("activerecord: transaction rollback")

// txState tracks a connection's open-transaction nesting depth.
type txState struct {
	depth int
}

// txStates maps an Adapter (a connection) to its live transaction nesting. A
// sync.Map keeps concurrent connections independent without a global lock on the
// hot path.
var txStates sync.Map // Adapter -> *txState

func stateFor(a Adapter) *txState {
	if v, ok := txStates.Load(a); ok {
		return v.(*txState)
	}
	s := &txState{}
	actual, _ := txStates.LoadOrStore(a, s)
	return actual.(*txState)
}

// TransactionDepth reports how many transactions/savepoints are currently open
// on the connection (0 when none). It is exposed for hosts and tests that assert
// nesting.
func TransactionDepth(a Adapter) int { return stateFor(a).depth }

// Transaction runs fn inside a database transaction on the connection, opening a
// real transaction at the top level and a SAVEPOINT when already nested. It
// commits (or releases the savepoint) when fn returns nil, and rolls back when
// fn returns an error or panics. [ErrRollback] rolls back and is swallowed; any
// other error rolls back and propagates.
func Transaction(a Adapter, fn func() error) (err error) {
	st := stateFor(a)
	st.depth++
	depth := st.depth
	savepoint := depth > 1
	spName := "active_record_" + strconv.Itoa(depth-1)

	if beginErr := execTx(a, beginSQL(a, savepoint, spName)); beginErr != nil {
		st.depth--
		return beginErr
	}

	committed := false
	defer func() {
		st.depth--
		if r := recover(); r != nil {
			// Roll back and re-panic so host exception semantics are preserved.
			_ = execTx(a, rollbackSQL(a, savepoint, spName))
			panic(r)
		}
		if !committed {
			// A rollback path already ran (or a begin/commit error occurred).
			return
		}
	}()

	if ferr := fn(); ferr != nil {
		if rbErr := execTx(a, rollbackSQL(a, savepoint, spName)); rbErr != nil {
			return rbErr
		}
		if errors.Is(ferr, ErrRollback) {
			return nil
		}
		return ferr
	}

	if cErr := execTx(a, commitSQL(a, savepoint, spName)); cErr != nil {
		if rbErr := execTx(a, rollbackSQL(a, savepoint, spName)); rbErr != nil {
			return rbErr
		}
		return cErr
	}
	committed = true
	return nil
}

// execTx runs a transaction-control statement through the adapter's DML path.
func execTx(a Adapter, sql string) error {
	_, _, err := a.ExecuteDML(sql)
	return err
}

// beginSQL renders the statement that opens a (sub)transaction: a SAVEPOINT when
// nested, else the dialect's BEGIN.
func beginSQL(a Adapter, savepoint bool, name string) string {
	if savepoint {
		return "SAVEPOINT " + name
	}
	return DialectFor(a).beginTransactionSQL()
}

// commitSQL renders the statement that finishes a (sub)transaction: RELEASE
// SAVEPOINT when nested, else the dialect's COMMIT.
func commitSQL(a Adapter, savepoint bool, name string) string {
	if savepoint {
		return "RELEASE SAVEPOINT " + name
	}
	return DialectFor(a).commitTransactionSQL()
}

// rollbackSQL renders the statement that aborts a (sub)transaction: ROLLBACK TO
// SAVEPOINT when nested, else the dialect's ROLLBACK.
func rollbackSQL(a Adapter, savepoint bool, name string) string {
	if savepoint {
		return "ROLLBACK TO SAVEPOINT " + name
	}
	return DialectFor(a).rollbackTransactionSQL()
}

// beginTransactionSQL is the dialect's BEGIN. ActiveRecord's sqlite3 adapter
// emits "BEGIN immediate TRANSACTION"; postgresql/mysql emit a bare "BEGIN".
func (d Dialect) beginTransactionSQL() string {
	if d == SQLite {
		return "BEGIN immediate TRANSACTION"
	}
	return "BEGIN"
}

// commitTransactionSQL is the dialect's COMMIT.
func (d Dialect) commitTransactionSQL() string {
	if d == SQLite {
		return "COMMIT TRANSACTION"
	}
	return "COMMIT"
}

// rollbackTransactionSQL is the dialect's ROLLBACK.
func (d Dialect) rollbackTransactionSQL() string {
	if d == SQLite {
		return "ROLLBACK TRANSACTION"
	}
	return "ROLLBACK"
}
