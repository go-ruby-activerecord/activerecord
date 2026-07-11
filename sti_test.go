// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func stiModels() (base, admin, super *Model) {
	base = NewModel("User", "users", Column{"id", "integer"}, Column{"name", "string"})
	base.STI("type")
	admin = base.Subclass("Admin")
	super = admin.Subclass("SuperAdmin")
	return
}

func TestSTITypeCondition(t *testing.T) {
	base, admin, super := stiModels()
	// The base class queries every row (no type filter).
	eq(t, "base", base.All().ToSQL(), `SELECT "users".* FROM "users"`)
	// A leaf subclass filters with equality.
	eq(t, "leaf", super.All().ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."type" = 'SuperAdmin'`)
	// A subclass with descendants filters with IN over itself + descendants.
	eq(t, "branch", admin.All().ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."type" IN ('Admin', 'SuperAdmin')`)
	// The type condition composes with other clauses.
	eq(t, "chain", super.Where(map[string]any{"name": "x"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."type" = 'SuperAdmin' AND "users"."name" = 'x'`)
}

func TestSTIUnscoped(t *testing.T) {
	_, admin, _ := stiModels()
	eq(t, "unscoped", admin.Unscoped().ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "unscoped-where", admin.Unscoped().Where(map[string]any{"name": "x"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" = 'x'`)
}

func TestSTIInstantiation(t *testing.T) {
	base, admin, _ := stiModels()
	// A row with a known type instantiates the subclass.
	rec := base.LoadSTI(map[string]any{"id": int64(1), "type": "Admin"})
	if rec.model != admin {
		t.Errorf("expected Admin instance, got %s", rec.model.Name)
	}
	// A row without a type loads as the base class.
	if base.LoadSTI(map[string]any{"id": int64(1)}).model != base {
		t.Error("typeless row should load as base")
	}
	// A row with an unknown type falls back to the base class.
	if base.LoadSTI(map[string]any{"id": int64(1), "type": "Ghost"}).model != base {
		t.Error("unknown type should load as base")
	}
	// LoadSTI on a non-STI model is just Load.
	plain := personModel(SQLite)
	if plain.LoadSTI(map[string]any{"id": int64(1)}).model != plain {
		t.Error("non-STI LoadSTI")
	}
}

func TestSTISubclassSharesSchema(t *testing.T) {
	base := NewModel("User", "users", Column{"id", "integer"}, Column{"name", "string"})
	base.Dialect = Postgres
	base.HasMany("posts", "Post")
	// Subclass without a prior STI call auto-enables the "type" discriminator.
	admin := base.Subclass("Admin")
	if !admin.HasColumn("type") || !admin.HasColumn("name") {
		t.Error("subclass should share columns + type")
	}
	if admin.Dialect != Postgres {
		t.Error("subclass should share dialect")
	}
	if admin.Association("posts") == nil {
		t.Error("subclass should share associations")
	}
	if admin.resolveModelName() != "users" {
		t.Error("subclass shares table")
	}
	// The subclass can reach the base via the sibling registry.
	if m, ok := admin.resolveModel("User"); !ok || m != base {
		t.Error("subclass should resolve base")
	}
}

// resolveModelName is a tiny test accessor for the shared table name.
func (m *Model) resolveModelName() string { return m.TableName }

func TestSTIDefaultColumn(t *testing.T) {
	base := NewModel("Animal", "animals", Column{"id", "integer"})
	base.STI("") // defaults to "type"
	if base.typeColumn != "type" || !base.HasColumn("type") {
		t.Error("STI default column")
	}
}
