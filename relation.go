// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"sort"
	"strconv"
	"strings"
)

// Relation is a lazy, immutable, chainable ActiveRecord::Relation. Every
// refining method (Where, Order, Limit, …) returns a new Relation, leaving the
// receiver unchanged, mirroring ActiveRecord's copy-on-refine semantics.
// [Relation.ToSQL] renders the accumulated relation to a SQL string that is
// byte-faithful to ActiveRecord's Relation#to_sql for the model's dialect.
type Relation struct {
	model *Model

	selects  []string // rendered projection fragments; empty => "table".*
	distinct bool

	wheres    []string // rendered predicate fragments, AND-joined
	havings   []string
	groups    []string // rendered group columns
	orders    []string // rendered "col" ASC fragments
	joins     []string // rendered JOIN clauses
	fromTable string   // overrides model.TableName when set

	limit    *int
	offset   *int
	lockOne  bool // internal: first/find_by add LIMIT 1
	reverse  bool // last: reverse default order
	distinctReset bool
}

// clone returns a shallow copy with independently-appendable slices.
func (r *Relation) clone() *Relation {
	n := *r
	n.selects = append([]string(nil), r.selects...)
	n.wheres = append([]string(nil), r.wheres...)
	n.havings = append([]string(nil), r.havings...)
	n.groups = append([]string(nil), r.groups...)
	n.orders = append([]string(nil), r.orders...)
	n.joins = append([]string(nil), r.joins...)
	return &n
}

// All returns an unrefined Relation over the model (ActiveRecord's Model.all).
func (m *Model) All() *Relation { return &Relation{model: m} }

// Where refines the relation with a condition. The shortcut on Model starts a
// fresh relation.
func (m *Model) Where(cond ...any) *Relation { return m.All().Where(cond...) }

// Order is the Model shortcut for All().Order(...).
func (m *Model) Order(cols ...any) *Relation { return m.All().Order(cols...) }

// Select is the Model shortcut for All().Select(...).
func (m *Model) Select(cols ...any) *Relation { return m.All().Select(cols...) }

// Joins is the Model shortcut for All().Joins(...).
func (m *Model) Joins(names ...any) *Relation { return m.All().Joins(names...) }

// table returns the effective FROM table name.
func (r *Relation) table() string {
	if r.fromTable != "" {
		return r.fromTable
	}
	return r.model.TableName
}

// dialect is the model's dialect.
func (r *Relation) dialect() Dialect { return r.model.Dialect }

// Where refines the relation. It accepts, matching ActiveRecord:
//
//   - a Hash-like map (map[string]any) — column => value, rendered as
//     equality / IN / BETWEEN / IS NULL and AND-joined and table-qualified.
//   - a single string — a raw SQL fragment, wrapped in parens.
//   - a string with "?" placeholders followed by bind values — each "?"
//     substituted by the quoted value.
//   - a string with ":name" placeholders followed by one map of binds.
//
// The receiver is unchanged; a new Relation is returned.
func (r *Relation) Where(cond ...any) *Relation {
	if len(cond) == 0 {
		return r
	}
	n := r.clone()
	n.wheres = append(n.wheres, r.model.buildConditions(cond, false)...)
	return n
}

// Not adds negated hash conditions (ActiveRecord's where.not). Only the
// hash form is supported (matching the common where.not(col: v) usage).
func (r *Relation) Not(cond ...any) *Relation {
	if len(cond) == 0 {
		return r
	}
	n := r.clone()
	n.wheres = append(n.wheres, r.model.buildConditions(cond, true)...)
	return n
}

// Or ORs the receiver's where-clause with another relation's, matching
// ActiveRecord's relation.or(other): the two predicate groups are parenthesized
// and joined by OR. Non-where clauses come from the receiver.
func (r *Relation) Or(other *Relation) *Relation {
	n := r.clone()
	left := strings.Join(r.wheres, " AND ")
	right := strings.Join(other.wheres, " AND ")
	switch {
	case left == "" && right == "":
		n.wheres = nil
	case left == "":
		n.wheres = []string{right}
	case right == "":
		n.wheres = []string{left}
	default:
		n.wheres = []string{"(" + left + " OR " + right + ")"}
	}
	return n
}

// Select sets the projection. Bare column names (string/Symbol) are
// table-qualified and quoted; a string that is not a plain identifier (contains
// a space, paren, dot or "*") is treated as a raw SQL expression and passed
// through, matching ActiveRecord.
func (r *Relation) Select(cols ...any) *Relation {
	n := r.clone()
	for _, c := range cols {
		name, ok := symbolName(c)
		if !ok {
			continue
		}
		n.selects = append(n.selects, r.model.projectColumn(name))
	}
	return n
}

// Distinct toggles SELECT DISTINCT. Distinct(false) clears it.
func (r *Relation) Distinct(on ...bool) *Relation {
	n := r.clone()
	n.distinct = len(on) == 0 || on[0]
	return n
}

// Order appends ordering terms. A bare name orders ASC; a map[string]any of
// name=>"desc"/"asc" (or Symbol values) sets direction; a raw string with a
// space is passed through.
func (r *Relation) Order(cols ...any) *Relation {
	n := r.clone()
	for _, c := range cols {
		n.orders = append(n.orders, r.model.orderTerms(c)...)
	}
	return n
}

// Group appends GROUP BY columns (table-qualified bare names, raw strings
// passed through).
func (r *Relation) Group(cols ...any) *Relation {
	n := r.clone()
	for _, c := range cols {
		if name, ok := symbolName(c); ok {
			n.groups = append(n.groups, r.model.projectColumn(name))
		}
	}
	return n
}

// Having adds a HAVING fragment (same forms as a string Where).
func (r *Relation) Having(cond ...any) *Relation {
	if len(cond) == 0 {
		return r
	}
	n := r.clone()
	n.havings = append(n.havings, r.model.buildConditions(cond, false)...)
	return n
}

// Limit sets LIMIT.
func (r *Relation) Limit(n int) *Relation {
	c := r.clone()
	c.limit = &n
	return c
}

// Offset sets OFFSET.
func (r *Relation) Offset(n int) *Relation {
	c := r.clone()
	c.offset = &n
	return c
}

// Joins adds INNER JOINs for the named associations (Symbol/string), rendering
// the join SQL ActiveRecord emits for belongs_to/has_many/has_one/HABTM/:through.
func (r *Relation) Joins(names ...any) *Relation {
	return r.addJoins("INNER JOIN", names)
}

// LeftJoins adds LEFT OUTER JOINs for the named associations.
func (r *Relation) LeftJoins(names ...any) *Relation {
	return r.addJoins("LEFT OUTER JOIN", names)
}

func (r *Relation) addJoins(kind string, names []any) *Relation {
	n := r.clone()
	for _, nm := range names {
		name, ok := symbolName(nm)
		if !ok {
			continue
		}
		n.joins = append(n.joins, r.model.joinClauses(kind, name)...)
	}
	return n
}

// Scope applies a registered named scope by name.
func (r *Relation) Scope(name string) *Relation {
	if body, ok := r.model.scopes[strings.TrimSpace(name)]; ok {
		return body(r)
	}
	return r
}

// Merge combines another relation's where/having/order/group/joins into the
// receiver (ActiveRecord's relation.merge), appending its clauses.
func (r *Relation) Merge(other *Relation) *Relation {
	n := r.clone()
	n.wheres = append(n.wheres, other.wheres...)
	n.havings = append(n.havings, other.havings...)
	n.orders = append(n.orders, other.orders...)
	n.groups = append(n.groups, other.groups...)
	n.joins = append(n.joins, other.joins...)
	if other.limit != nil {
		n.limit = other.limit
	}
	if other.offset != nil {
		n.offset = other.offset
	}
	if other.distinct {
		n.distinct = true
	}
	if len(other.selects) > 0 {
		n.selects = append(n.selects, other.selects...)
	}
	return n
}

// ToSQL renders the relation to a SELECT statement, byte-faithful to
// ActiveRecord's Relation#to_sql for the model's dialect.
func (r *Relation) ToSQL() string {
	var b strings.Builder
	b.WriteString("SELECT ")
	if r.distinct {
		b.WriteString("DISTINCT ")
	}
	if len(r.selects) == 0 {
		b.WriteString(r.model.starSelect())
	} else {
		b.WriteString(strings.Join(r.selects, ", "))
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
	orders := r.effectiveOrders()
	if len(orders) > 0 {
		b.WriteString(" ORDER BY ")
		b.WriteString(strings.Join(orders, ", "))
	}
	if r.limit != nil {
		b.WriteString(" LIMIT ")
		b.WriteString(strconv.Itoa(*r.limit))
	}
	if r.offset != nil {
		b.WriteString(" OFFSET ")
		b.WriteString(strconv.Itoa(*r.offset))
	}
	return b.String()
}

// effectiveOrders returns the ORDER BY terms, applying the last() reversal
// (ActiveRecord reverses the order and, absent any, orders by the primary key
// descending).
func (r *Relation) effectiveOrders() []string {
	if !r.reverse {
		return r.orders
	}
	src := r.orders
	if len(src) == 0 {
		return []string{r.dialect().qualify(r.table(), r.model.PrimaryKey) + " DESC"}
	}
	out := make([]string, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- {
		out = append(out, reverseOrderTerm(src[i]))
	}
	return out
}

// reverseOrderTerm flips ASC<->DESC on a rendered order term.
func reverseOrderTerm(term string) string {
	switch {
	case strings.HasSuffix(term, " ASC"):
		return term[:len(term)-4] + " DESC"
	case strings.HasSuffix(term, " DESC"):
		return term[:len(term)-5] + " ASC"
	default:
		return term + " DESC"
	}
}

// projectColumn renders a projection element: a bare identifier is
// table-qualified and quoted; anything else (space/paren/dot/star/comma) is
// passed through as a raw expression, matching ActiveRecord's select() rules.
func (m *Model) projectColumn(name string) string {
	if isBareIdentifier(name) {
		return m.Dialect.qualify(m.TableName, name)
	}
	return name
}

// orderTerms renders one order argument to a list of "col DIR" fragments.
func (m *Model) orderTerms(c any) []string {
	if hm, ok := c.(map[string]any); ok {
		keys := make([]string, 0, len(hm))
		for k := range hm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]string, 0, len(keys))
		for _, k := range keys {
			out = append(out, m.qualifyOrder(k)+" "+orderDir(hm[k]))
		}
		return out
	}
	name, ok := symbolName(c)
	if !ok {
		return nil
	}
	if !isBareIdentifier(name) {
		return []string{name}
	}
	return []string{m.qualifyOrder(name) + " ASC"}
}

// qualifyOrder qualifies a bare order column; raw expressions pass through.
func (m *Model) qualifyOrder(name string) string {
	if isBareIdentifier(name) {
		return m.Dialect.qualify(m.TableName, name)
	}
	return name
}

// orderDir normalizes a direction value to ASC/DESC.
func orderDir(v any) string {
	s, _ := symbolName(v)
	if strings.EqualFold(s, "desc") {
		return "DESC"
	}
	return "ASC"
}

// isBareIdentifier reports whether s is a plain SQL identifier (safe to qualify
// and quote) rather than a raw expression.
func isBareIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' ||
			(c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}
