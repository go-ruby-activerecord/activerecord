// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func TestModelBasics(t *testing.T) {
	m := NewModel("User", "users", Column{"id", "integer"}, Column{"name", "string"})
	if m.PrimaryKey != "id" || m.TableName != "users" || m.Name != "User" {
		t.Error("defaults")
	}
	if !m.HasColumn("name") || m.HasColumn("nope") {
		t.Error("HasColumn")
	}
	if len(m.Columns()) != 2 {
		t.Error("Columns")
	}
	// AddColumn is idempotent.
	m.AddColumn("name", "string").AddColumn("age", "integer")
	if len(m.Columns()) != 3 {
		t.Errorf("after add = %d", len(m.Columns()))
	}
	// duplicate Column via NewModel is ignored.
	m2 := NewModel("X", "xs", Column{"a", "string"}, Column{"a", "string"})
	if len(m2.Columns()) != 1 {
		t.Errorf("dup column = %d", len(m2.Columns()))
	}
	c, ok := m.column("name")
	if !ok || c.Type != "string" {
		t.Error("column lookup")
	}
	if m.dialect() != SQLite {
		t.Error("default dialect")
	}
}

func TestModelShortcuts(t *testing.T) {
	m := NewModel("User", "users", Column{"id", "integer"}, Column{"name", "string"})
	eq(t, "where", m.Where(map[string]any{"name": "x"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" = 'x'`)
	eq(t, "order", m.Order("name").ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."name" ASC`)
	eq(t, "select", m.Select("name").ToSQL(), `SELECT "users"."name" FROM "users"`)
	eq(t, "all", m.All().ToSQL(), `SELECT "users".* FROM "users"`)
}

func TestSymbolAndName(t *testing.T) {
	if Symbol("x").String() != "x" {
		t.Error("Symbol.String")
	}
	if n, ok := symbolName("s"); !ok || n != "s" {
		t.Error("symbolName string")
	}
	if n, ok := symbolName(Symbol("y")); !ok || n != "y" {
		t.Error("symbolName symbol")
	}
	if _, ok := symbolName(42); ok {
		t.Error("symbolName bad")
	}
}
