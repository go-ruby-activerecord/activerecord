// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"sort"
	"strings"
)

// Column is one column of a model's table: its name and its logical type (used
// for DDL emission and attribute type-casting). Type is an ActiveRecord type
// symbol name ("string", "integer", "boolean", "datetime", …).
type Column struct {
	Name string
	Type string
}

// Model describes a mapped ActiveRecord class: its table name, primary key,
// columns, associations and validations. A host builds one per Ruby model class
// and hands it to [Model.All] (or the Where/Order/… shortcuts) to obtain a
// [Relation], and to [Model.Build] to obtain a validating attribute record.
//
// TableName defaults to the pluralized, underscored class name in Rails; the
// host supplies the already-resolved table name (pluralization is a host
// concern), matching how a differential oracle names its tables.
type Model struct {
	// Name is the Ruby class name ("User"), used only for validation messages
	// and diagnostics.
	Name string
	// TableName is the resolved SQL table name ("users").
	TableName string
	// PrimaryKey is the primary-key column name ("id").
	PrimaryKey string
	// Dialect selects SQL rendering; defaults to SQLite (zero value).
	Dialect Dialect

	columns      []Column
	columnByName map[string]Column
	associations map[string]*Association
	validations  []validation
	scopes       map[string]func(*Relation) *Relation
	models       map[string]*Model // sibling models by class name, for joins
	callbacks    [][]Callback      // lifecycle hooks, indexed by callbackKind

	// defaultScope, when set, refines every relation started from the model
	// (ActiveRecord's default_scope); Unscoped bypasses it.
	defaultScope func(*Relation) *Relation

	// Single-table-inheritance wiring (see sti.go). typeColumn is the
	// discriminator column ("type" by default) when the model participates in
	// STI; typeName is this class's discriminator value; subtypes lists the
	// discriminator values of descendant classes (for the base-class filter).
	typeColumn    string
	typeName      string
	subtypes      []string
	base          *Model
	subtypeModels map[string]*Model

	// stmts caches this model's prepared statements (see statement_cache.go).
	stmts *StatementCache
}

// NewModel returns a Model for class name with the given table and columns. The
// primary key defaults to "id".
func NewModel(name, table string, columns ...Column) *Model {
	m := &Model{
		Name:         name,
		TableName:    table,
		PrimaryKey:   "id",
		columnByName: map[string]Column{},
		associations: map[string]*Association{},
		scopes:       map[string]func(*Relation) *Relation{},
		models:       map[string]*Model{},
	}
	for _, c := range columns {
		m.addColumn(c)
	}
	return m
}

func (m *Model) addColumn(c Column) {
	if _, ok := m.columnByName[c.Name]; ok {
		return
	}
	m.columns = append(m.columns, c)
	m.columnByName[c.Name] = c
}

// AddColumn appends a column (chainable), for hosts that describe the schema
// incrementally.
func (m *Model) AddColumn(name, typ string) *Model {
	m.addColumn(Column{Name: name, Type: typ})
	return m
}

// Columns returns the model's columns in declaration order. The slice must not
// be mutated.
func (m *Model) Columns() []Column { return m.columns }

// HasColumn reports whether the model has a column of the given name.
func (m *Model) HasColumn(name string) bool {
	_, ok := m.columnByName[name]
	return ok
}

// column returns the column named name, or a zero Column and false.
func (m *Model) column(name string) (Column, bool) {
	c, ok := m.columnByName[name]
	return c, ok
}

// Register links a sibling model so association joins can resolve the target
// table/keys. It is chainable and idempotent.
func (m *Model) Register(others ...*Model) *Model {
	for _, o := range others {
		if o != nil {
			m.models[o.Name] = o
		}
	}
	return m
}

// resolveModel finds a registered sibling (or self) by class name.
func (m *Model) resolveModel(name string) (*Model, bool) {
	if name == m.Name {
		return m, true
	}
	o, ok := m.models[name]
	return o, ok
}

// dialect returns the model's dialect (zero value is SQLite).
func (m *Model) dialect() Dialect { return m.Dialect }

// starSelect renders the default `"table".*` projection.
func (m *Model) starSelect() string {
	return m.Dialect.quoteTableName(m.TableName) + ".*"
}

// Scope registers a named scope: a function that refines a Relation. Calling
// [Relation.Scope] (or the generated shortcut a host wires) applies it.
func (m *Model) Scope(name string, body func(*Relation) *Relation) *Model {
	m.scopes[strings.TrimSpace(name)] = body
	return m
}

// ScopeNames returns the registered named-scope names in sorted order.
func (m *Model) ScopeNames() []string {
	out := make([]string, 0, len(m.scopes))
	for k := range m.scopes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultScope registers the model's default_scope: a refinement applied to
// every relation started with [Model.All] (and the Where/Order/… shortcuts).
// [Model.Unscoped] bypasses it. Registering twice replaces the previous one,
// matching a single default_scope declaration.
func (m *Model) DefaultScope(body func(*Relation) *Relation) *Model {
	m.defaultScope = body
	return m
}
