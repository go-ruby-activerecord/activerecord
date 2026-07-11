// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// recAdapter is a richer in-memory Adapter for the execution tests: it records
// every SQL string it is handed (in order, across Execute and ExecuteDML) so a
// test can assert the full statement sequence a persistence/transaction/preload
// operation issues, and it replays queued row results for the SELECT half.
type recAdapter struct {
	name string
	log  []string

	execRows [][]Row // rows returned by successive Execute calls
	execIdx  int

	affected int64
	lastID   int64

	// failOn makes any statement containing this substring return an error,
	// exercising the error branches deterministically. failKind selects whether
	// the Execute or ExecuteDML path fails (default: both).
	failOn   string
	failAlso string // a second substring that also triggers a failure
	failExec bool
	failDML  bool
}

func (r *recAdapter) shouldFail(sql string) bool {
	if r.failOn != "" && strings.Contains(sql, r.failOn) {
		return true
	}
	return r.failAlso != "" && strings.Contains(sql, r.failAlso)
}

func (r *recAdapter) Execute(sql string) ([]Row, error) {
	r.log = append(r.log, sql)
	if r.shouldFail(sql) && (r.failExec || (!r.failExec && !r.failDML)) {
		return nil, errors.New("exec boom: " + sql)
	}
	var rows []Row
	if r.execIdx < len(r.execRows) {
		rows = r.execRows[r.execIdx]
		r.execIdx++
	}
	return rows, nil
}

func (r *recAdapter) ExecuteDML(sql string) (int64, int64, error) {
	r.log = append(r.log, sql)
	if r.shouldFail(sql) && (r.failDML || (!r.failExec && !r.failDML)) {
		return 0, 0, errors.New("dml boom: " + sql)
	}
	return r.affected, r.lastID, nil
}

func (r *recAdapter) AdapterName() string { return r.name }

// clear resets the recorded log and row cursor between phases of a test.
func (r *recAdapter) clear() { r.log = nil; r.execIdx = 0 }

// dml returns only the recorded statements that are not transaction control,
// which is what most sequence assertions care about.
func (r *recAdapter) nonTx() []string {
	var out []string
	for _, s := range r.log {
		if isTxControl(s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func isTxControl(s string) bool {
	for _, p := range []string{"BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// prepAdapter is a recAdapter that also implements PreparedAdapter, capturing the
// SQL template and the bind values separately (the true prepared-statement path).
type prepAdapter struct {
	recAdapter
	lastPreparedSQL string
	lastBinds       []any
}

func (p *prepAdapter) ExecutePrepared(sql string, binds []any) ([]Row, error) {
	p.lastPreparedSQL = sql
	p.lastBinds = binds
	p.log = append(p.log, sql)
	var rows []Row
	if p.execIdx < len(p.execRows) {
		rows = p.execRows[p.execIdx]
		p.execIdx++
	}
	return rows, nil
}

// withClock pins nowFunc to a fixed instant for a test and restores it after,
// making created_at/updated_at deterministic.
func withClock(t *testing.T, at time.Time) {
	t.Helper()
	prev := nowFunc
	nowFunc = func() time.Time { return at }
	t.Cleanup(func() { nowFunc = prev })
}

// resetTxState drops any transaction bookkeeping for an adapter so a test starts
// from a clean nesting depth (defends against a prior failed test leaking state).
func resetTxState(a Adapter) { txStates.Delete(a) }

// -- ActiveRecord execution oracle -------------------------------------------

// arExecPreamble establishes an in-memory sqlite connection with prepared
// statements OFF (so ActiveRecord inlines bind values, matching this package's
// string-adapter output) and installs a subscriber that records issued SQL.
const arExecPreamble = `
$stdout.sync = true
require "active_record"
ActiveRecord::Base.establish_connection(adapter: "sqlite3", database: ":memory:", prepared_statements: false)
ActiveRecord::Schema.verbose = false
$arlog = []
ActiveSupport::Notifications.subscribe("sql.active_record") do |*a|
  ev = ActiveSupport::Notifications::Event.new(*a)
  next if ev.payload[:name] == "SCHEMA"
  $arlog << ev.payload[:sql]
end
def arlog!; $arlog.clear; end
def arlog_dump; puts $arlog.join("\x1e"); end
`

// runAR runs a Ruby script (schema + models + body) under the exec preamble and
// returns the SQL statements it logged after the last arlog! call.
func runAR(t *testing.T, bin, script string) []string {
	t.Helper()
	return runRuby(t, bin, arExecPreamble+"\n"+script+"\narlog_dump\n")
}

// runRuby runs a self-contained Ruby script and returns its \x1e-separated output
// tokens (the convention the oracle scripts print).
func runRuby(t *testing.T, bin, script string) []string {
	t.Helper()
	out, err := exec.Command(bin, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\n%s", err, out)
	}
	line := strings.TrimRight(string(out), "\n")
	if line == "" {
		return nil
	}
	return strings.Split(line, "\x1e")
}
