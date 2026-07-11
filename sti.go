// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file implements ActiveRecord's single-table inheritance (STI) — the "STI
// edge cases" the README used to defer. Subclasses share one table and a type
// discriminator column, and ActiveRecord adds a type condition to a subclass's
// queries while the base class queries every row:
//
//	class User  < ActiveRecord::Base; end        # SELECT "users".* FROM "users"
//	class Admin < User; end                       # …WHERE "users"."type" = 'Admin'
//
// A subclass with descendants filters with an IN over itself and every
// descendant's type, exactly as ActiveRecord's finder_needs_type_condition?
// path. Rows are instantiated as the class named by the type column
// ([Model.LoadSTI]).

// STI marks the model as a single-table-inheritance root discriminated by
// column (defaulting to "type"). The base class queries every row (no type
// filter); subclasses created with [Model.Subclass] add the filter. The column
// is added to the model if absent.
func (m *Model) STI(column string) *Model {
	if column == "" {
		column = "type"
	}
	m.typeColumn = column
	if !m.HasColumn(column) {
		m.AddColumn(column, "string")
	}
	if m.subtypeModels == nil {
		m.subtypeModels = map[string]*Model{}
	}
	return m
}

// Subclass returns a new model for an STI subclass named className: it shares the
// root's table, primary key, dialect, columns, associations and sibling registry,
// carries className as its type value, and is filtered by that value in queries.
// The subclass is registered so the root (and every ancestor) can instantiate and
// filter it.
func (base *Model) Subclass(className string) *Model {
	if base.typeColumn == "" {
		base.STI("type")
	}
	sub := NewModel(className, base.TableName)
	sub.PrimaryKey = base.PrimaryKey
	sub.Dialect = base.Dialect
	for _, c := range base.columns {
		sub.addColumn(c)
	}
	for k, v := range base.associations {
		sub.associations[k] = v
	}
	for k, v := range base.models {
		sub.models[k] = v
	}
	sub.models[base.Name] = base
	sub.typeColumn = base.typeColumn
	sub.typeName = className
	sub.base = base
	sub.subtypeModels = map[string]*Model{}
	base.registerSubtype(className, sub)
	return sub
}

// registerSubtype records a descendant's type value and model on this model and
// every ancestor, so a subclass's IN condition covers its whole subtree and any
// ancestor can instantiate the row.
func (m *Model) registerSubtype(className string, sub *Model) {
	m.subtypeModels[className] = sub
	m.subtypes = append(m.subtypes, className)
	if m.base != nil {
		m.base.registerSubtype(className, sub)
	}
}

// applyTypeCondition adds the STI type predicate to a fresh relation for a
// subclass (a plain equality, or an IN over the subclass and its descendants). A
// base class (no type value) and an unscoped relation get no condition.
func (m *Model) applyTypeCondition(r *Relation) *Relation {
	if r.unscoped || m.typeName == "" {
		return r
	}
	q := m.Dialect.qualify(m.TableName, m.typeColumn)
	values := append([]string{m.typeName}, m.subtypes...)
	n := r.clone()
	if len(values) == 1 {
		n.wheres = append(n.wheres, q+" = "+m.Dialect.quoteString(values[0]))
		return n
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = m.Dialect.quoteString(v)
	}
	n.wheres = append(n.wheres, q+" IN ("+joinComma(quoted)+")")
	return n
}

// LoadSTI materializes a row as the subclass named by its type column when the
// model is an STI root with that subclass registered; otherwise it loads the row
// as the receiver. It is the instantiation ActiveRecord does when reading rows
// from a base-class relation.
func (m *Model) LoadSTI(row Row) *Record {
	if m.typeColumn != "" && m.subtypeModels != nil {
		if tv, ok := row[m.typeColumn]; ok {
			if s, ok := symbolName(tv); ok {
				if sub := m.subtypeModels[s]; sub != nil {
					return sub.Load(row)
				}
			}
		}
	}
	return m.Load(row)
}

// joinComma joins fragments with ", " (a tiny local helper to avoid importing
// strings just for one call site).
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
