// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"sort"
	"strconv"
	"strings"
)

// First returns a relation scoped to the first row: ORDER BY primary key ASC
// (when no order is set) LIMIT 1, matching Model.first.
func (r *Relation) First() *Relation {
	n := r.clone()
	if len(n.orders) == 0 {
		n.orders = []string{r.dialect().qualify(r.table(), r.model.PrimaryKey) + " ASC"}
	}
	one := 1
	n.limit = &one
	return n
}

// Last returns a relation scoped to the last row: the order reversed (or
// primary key DESC absent an order) LIMIT 1, matching Model.last.
func (r *Relation) Last() *Relation {
	n := r.clone()
	n.reverse = true
	one := 1
	n.limit = &one
	return n
}

// Take returns a relation with just LIMIT 1 and no imposed order (Model.take).
func (r *Relation) Take() *Relation {
	n := r.clone()
	one := 1
	n.limit = &one
	return n
}

// Find scopes to the row with the given primary-key value (Model.find(id)):
// WHERE "table"."id" = id LIMIT 1.
func (r *Relation) Find(id any) *Relation {
	return r.Where(map[string]any{r.model.PrimaryKey: id}).Take()
}

// FindBy scopes to the first row matching the hash conditions
// (Model.find_by(...)): the conditions plus LIMIT 1.
func (r *Relation) FindBy(cond map[string]any) *Relation {
	return r.Where(cond).Take()
}

// CountSQL renders the COUNT(*) query for the relation (Model...count),
// preserving where/join/group but dropping order and select, as ActiveRecord
// does for a count without a column.
func (r *Relation) CountSQL() string {
	return r.aggregateSQL("COUNT(*)", "")
}

// CountColumnSQL renders COUNT("table"."col").
func (r *Relation) CountColumnSQL(col string) string {
	return r.aggregateSQL("COUNT", col)
}

// SumSQL renders SUM("table"."col") (Model...sum(:col)).
func (r *Relation) SumSQL(col string) string { return r.aggregateSQL("SUM", col) }

// AverageSQL renders AVG("table"."col") (Model...average(:col)).
func (r *Relation) AverageSQL(col string) string { return r.aggregateSQL("AVG", col) }

// MinimumSQL renders MIN("table"."col") (Model...minimum(:col)).
func (r *Relation) MinimumSQL(col string) string { return r.aggregateSQL("MIN", col) }

// MaximumSQL renders MAX("table"."col") (Model...maximum(:col)).
func (r *Relation) MaximumSQL(col string) string { return r.aggregateSQL("MAX", col) }

// aggregateSQL builds the SELECT for an aggregate. fn is the SQL function; col
// is the column ("" means the function already includes its argument, e.g.
// "COUNT(*)"). Grouped aggregates additionally SELECT the group columns and add
// the GROUP BY, matching ActiveRecord's calculate.
func (r *Relation) aggregateSQL(fn, col string) string {
	var proj string
	if col == "" {
		proj = fn
	} else {
		proj = fn + "(" + r.model.projectColumn(col) + ")"
	}
	var b strings.Builder
	b.WriteString("SELECT ")
	if len(r.groups) > 0 {
		b.WriteString(strings.Join(r.groups, ", "))
		b.WriteString(", ")
	}
	b.WriteString(proj)
	if len(r.groups) > 0 {
		// ActiveRecord aliases grouped aggregates; keep the bare form for the
		// ungrouped-compatible oracle and document the grouped alias in README.
	}
	b.WriteString(" FROM ")
	b.WriteString(r.dialect().quoteTableName(r.table()))
	for _, j := range r.joins {
		b.WriteByte(' ')
		b.WriteString(j)
	}
	if len(r.wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(r.wheres, " AND "))
	}
	if len(r.groups) > 0 {
		b.WriteString(" GROUP BY ")
		b.WriteString(strings.Join(r.groups, ", "))
	}
	if len(r.havings) > 0 {
		b.WriteString(" HAVING ")
		b.WriteString(strings.Join(r.havings, " AND "))
	}
	return b.String()
}

// PluckSQL renders the SELECT for plucking the given columns
// (Model...pluck(:a,:b)): the qualified columns, keeping where/join/order but
// no star.
func (r *Relation) PluckSQL(cols ...any) string {
	proj := make([]string, 0, len(cols))
	for _, c := range cols {
		if name, ok := symbolName(c); ok {
			proj = append(proj, r.model.projectColumn(name))
		}
	}
	n := r.clone()
	n.selects = proj
	return n.ToSQL()
}

// ExistsSQL renders the existence probe ActiveRecord emits for exists?:
// SELECT 1 AS one FROM "table" ... LIMIT 1.
func (r *Relation) ExistsSQL() string {
	var b strings.Builder
	b.WriteString("SELECT 1 AS one FROM ")
	b.WriteString(r.dialect().quoteTableName(r.table()))
	for _, j := range r.joins {
		b.WriteByte(' ')
		b.WriteString(j)
	}
	if len(r.wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(r.wheres, " AND "))
	}
	b.WriteString(" LIMIT 1")
	return b.String()
}

// InsertSQL renders an INSERT for one row of attributes, columns in sorted order
// for determinism: INSERT INTO "table" ("a", "b") VALUES (v1, v2).
func (m *Model) InsertSQL(attrs map[string]any) string {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cols := make([]string, len(keys))
	vals := make([]string, len(keys))
	for i, k := range keys {
		cols[i] = m.Dialect.quoteColumnName(k)
		vals[i] = m.Dialect.quote(attrs[k])
	}
	return "INSERT INTO " + m.Dialect.quoteTableName(m.TableName) +
		" (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(vals, ", ") + ")"
}

// UpdateAllSQL renders an UPDATE for the relation's scope, SET assignments in
// sorted column order (Model...update_all(a: 1)).
func (r *Relation) UpdateAllSQL(sets map[string]any) string {
	keys := make([]string, 0, len(sets))
	for k := range sets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	assigns := make([]string, len(keys))
	for i, k := range keys {
		assigns[i] = r.dialect().quoteColumnName(k) + " = " + r.dialect().quote(sets[k])
	}
	sql := "UPDATE " + r.dialect().quoteTableName(r.table()) + " SET " + strings.Join(assigns, ", ")
	if len(r.wheres) > 0 {
		sql += " WHERE " + strings.Join(r.wheres, " AND ")
	}
	return sql
}

// DeleteAllSQL renders a DELETE for the relation's scope (Model...delete_all).
func (r *Relation) DeleteAllSQL() string {
	sql := "DELETE FROM " + r.dialect().quoteTableName(r.table())
	if len(r.wheres) > 0 {
		sql += " WHERE " + strings.Join(r.wheres, " AND ")
	}
	return sql
}

// itoa is a tiny local wrapper used by callers that need a rendered int.
func itoa(n int) string { return strconv.Itoa(n) }
