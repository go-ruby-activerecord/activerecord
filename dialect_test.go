// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"math/big"
	"testing"
	"time"
)

func TestDialectNamesAndLookup(t *testing.T) {
	cases := []struct {
		d    Dialect
		name string
	}{{SQLite, "sqlite3"}, {Postgres, "postgresql"}, {MySQL, "mysql"}}
	for _, c := range cases {
		if c.d.String() != c.name {
			t.Errorf("String %d = %q want %q", c.d, c.d.String(), c.name)
		}
	}
	for name, want := range map[string]Dialect{
		"sqlite3": SQLite, "sqlite": SQLite, "postgresql": Postgres, "postgres": Postgres,
		"pg": Postgres, "mysql": MySQL, "mysql2": MySQL, "trilogy": MySQL, "weird": SQLite,
	} {
		if got := DialectByName(name); got != want {
			t.Errorf("DialectByName(%q) = %v want %v", name, got, want)
		}
	}
	if Dialect(99).String() != "sqlite3" {
		t.Errorf("unknown dialect name")
	}
}

func TestQuoting(t *testing.T) {
	if got := SQLite.quoteColumnName(`a"b`); got != `"a""b"` {
		t.Errorf("sqlite col = %q", got)
	}
	if got := MySQL.quoteColumnName("a`b"); got != "`a``b`" {
		t.Errorf("mysql col = %q", got)
	}
	if got := SQLite.quoteColumnName("*"); got != "*" {
		t.Errorf("star = %q", got)
	}
	if got := SQLite.quoteTableName("public.users"); got != `"public"."users"` {
		t.Errorf("schema.table = %q", got)
	}
	if got := MySQL.quoteTableName("db.t"); got != "`db`.`t`" {
		t.Errorf("mysql schema.table = %q", got)
	}
}

func TestLiteralization(t *testing.T) {
	bi, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	tm := time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		d    Dialect
		v    any
		want string
	}{
		{SQLite, nil, "NULL"},
		{SQLite, true, "TRUE"},
		{SQLite, false, "FALSE"},
		{MySQL, true, "1"},
		{MySQL, false, "0"},
		{SQLite, 42, "42"},
		{SQLite, int64(42), "42"},
		{SQLite, int32(42), "42"},
		{SQLite, bi, "123456789012345678901234567890"},
		{SQLite, 3.5, "3.5"},
		{SQLite, float32(1.5), "1.5"},
		{SQLite, "O'Brien", "'O''Brien'"},
		{MySQL, "O'Brien", `'O\'Brien'`},
		{MySQL, `a\b`, `'a\\b'`},
		{MySQL, "a\x00b", `'a\0b'`},
		{SQLite, Symbol("sym"), "'sym'"},
		{SQLite, tm, "'2026-07-02 12:30:00'"},
	} {
		if got := tc.d.quote(tc.v); got != tc.want {
			t.Errorf("%v.quote(%v) = %q want %q", tc.d, tc.v, got, tc.want)
		}
	}
	// default branch: an unhandled type falls back to quoted string.
	type weird struct{}
	if got := SQLite.quote(weird{}); got != "''" {
		t.Errorf("weird = %q", got)
	}
}

func TestSQLPerDialect(t *testing.T) {
	// Postgres identifiers match sqlite double-quoting; MySQL uses backticks.
	up, _, _, _ := testModels(Postgres)
	eq(t, "pg-where", up.Where(map[string]any{"active": true}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."active" = TRUE`)
	um, _, _, _ := testModels(MySQL)
	eq(t, "mysql-where", um.Where(map[string]any{"active": true}).ToSQL(),
		"SELECT `users`.* FROM `users` WHERE `users`.`active` = 1")
	eq(t, "mysql-join", um.Joins("posts").ToSQL(),
		"SELECT `users`.* FROM `users` INNER JOIN `posts` ON `posts`.`user_id` = `users`.`id`")
}

func TestStringify(t *testing.T) {
	if stringify(Symbol("x")) != "x" {
		t.Error("sym")
	}
	if stringify(true) != "true" || stringify(false) != "false" {
		t.Error("bool")
	}
	if stringify(nil) != "" {
		t.Error("nil")
	}
	if stringify("hi") != "hi" {
		t.Error("str")
	}
	if stringify(1.0) != "" {
		t.Error("default")
	}
}
