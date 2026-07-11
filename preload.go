// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file implements ActiveRecord's eager-loading materialization — the
// multi-query fetch behind includes/preload the README used to defer. The join
// geometry already lived in associations.go; here the planner runs the extra
// query per association and wires the loaded targets onto the parent records,
// exactly as ActiveRecord::Associations::Preloader does, avoiding the N+1:
//
//	users = User.includes(:posts).to_a
//	  SELECT "users".* FROM "users"
//	  SELECT "posts".* FROM "posts" WHERE "posts"."user_id" IN (1, 2, …)
//
// belongs_to, has_one, has_many, has_and_belongs_to_many and :through are all
// handled, matching the exact queries ActiveRecord emits (a one-element key set
// collapses to "= v", as ActiveRecord renders it).

import "strings"

// Includes records association names to eager-load when the relation is
// materialized with [LoadIncludes] (ActiveRecord's includes/preload). It is
// chainable and copy-on-write like every relation refinement.
func (r *Relation) Includes(names ...any) *Relation {
	n := r.clone()
	for _, nm := range names {
		if s, ok := symbolName(nm); ok {
			n.includes = append(n.includes, s)
		}
	}
	return n
}

// IncludesNames returns the association names queued for eager loading.
func (r *Relation) IncludesNames() []string { return append([]string(nil), r.includes...) }

// LoadIncludes runs the relation's main SELECT and then eager-loads every
// association named by [Relation.Includes], attaching the targets to the loaded
// records. It returns the root records (with their associations preloaded).
func LoadIncludes(a Adapter, r *Relation) ([]*Record, error) {
	roots, err := LoadAll(a, r)
	if err != nil {
		return nil, err
	}
	for _, name := range r.includes {
		if err := Preload(a, roots, name); err != nil {
			return nil, err
		}
	}
	return roots, nil
}

// PreloadedAssociation returns the eager-loaded target records for the named
// association (empty when it was not preloaded), the pure-Go equivalent of
// reading an association whose target ActiveRecord has already cached.
func (r *Record) PreloadedAssociation(name string) []*Record {
	if r.loaded == nil {
		return nil
	}
	return r.loaded[name]
}

// setLoaded stores targets for an association on the record.
func (r *Record) setLoaded(name string, targets []*Record) {
	if r.loaded == nil {
		r.loaded = map[string][]*Record{}
	}
	r.loaded[name] = targets
}

// Preload eager-loads the named association for a set of parent records with a
// single additional query (per hop), attaching the results to each parent. It is
// a no-op when parents is empty or the association is unknown.
func Preload(a Adapter, parents []*Record, name string) error {
	if len(parents) == 0 {
		return nil
	}
	owner := parents[0].model
	assoc := owner.associations[name]
	if assoc == nil {
		return nil
	}
	switch assoc.Kind {
	case BelongsTo:
		return owner.preloadBelongsTo(a, parents, name, assoc)
	case HasMany, HasOne:
		if assoc.Through != "" {
			return owner.preloadThrough(a, parents, name, assoc)
		}
		return owner.preloadHasMany(a, parents, name, assoc)
	case HABTM:
		return owner.preloadHABTM(a, parents, name, assoc)
	}
	return nil
}

// preloadHasMany loads children where fk IN (parent pks) and groups them onto the
// parents by that foreign key (has_one keeps at most one).
func (m *Model) preloadHasMany(a Adapter, parents []*Record, name string, assoc *Association) error {
	tgt, ok := m.resolveModel(assoc.ClassName)
	if !ok {
		return nil
	}
	fk := assoc.foreignKey(underscoreClass(m.Name))
	ids := collectValues(parents, m.PrimaryKey)
	if len(ids) == 0 {
		return nil
	}
	children, err := LoadAll(a, tgt.Where(map[string]any{fk: ids}))
	if err != nil {
		return err
	}
	byKey := groupBy(children, fk)
	for _, p := range parents {
		p.setLoaded(name, byKey[assocKey(p.attrs[m.PrimaryKey])])
	}
	return nil
}

// preloadBelongsTo loads targets where pk IN (distinct fk values) and attaches
// the matching one to each parent.
func (m *Model) preloadBelongsTo(a Adapter, parents []*Record, name string, assoc *Association) error {
	tgt, ok := m.resolveModel(assoc.ClassName)
	if !ok {
		return nil
	}
	fk := assoc.foreignKey(underscoreClass(assoc.ClassName))
	ids := collectValues(parents, fk)
	if len(ids) == 0 {
		return nil
	}
	targets, err := LoadAll(a, tgt.Where(map[string]any{tgt.PrimaryKey: ids}))
	if err != nil {
		return err
	}
	byKey := groupBy(targets, tgt.PrimaryKey)
	for _, p := range parents {
		p.setLoaded(name, byKey[assocKey(p.attrs[fk])])
	}
	return nil
}

// preloadHABTM loads the join rows for the parents, then the targets, and groups
// targets onto each parent through the join table.
func (m *Model) preloadHABTM(a Adapter, parents []*Record, name string, assoc *Association) error {
	tgt, ok := m.resolveModel(assoc.ClassName)
	if !ok {
		return nil
	}
	jt := assoc.JoinTable
	if jt == "" {
		jt = habtmJoinTable(m.TableName, tgt.TableName)
	}
	ownFK := assoc.ForeignKey
	if ownFK == "" {
		ownFK = underscoreClass(m.Name) + "_id"
	}
	otherFK := assoc.AssociationForeignKey
	if otherFK == "" {
		otherFK = underscoreClass(tgt.Name) + "_id"
	}
	ids := collectValues(parents, m.PrimaryKey)
	if len(ids) == 0 {
		return nil
	}
	joinModel := NewModel(jt, jt)
	joinModel.Dialect = m.Dialect
	joinRows, err := a.Execute(joinModel.Where(map[string]any{ownFK: ids}).ToSQL())
	if err != nil {
		return err
	}
	// Map owner key -> ordered list of target keys, and gather distinct targets.
	var targetIDs []any
	seenTarget := map[string]bool{}
	ownerToTargets := map[string][]any{}
	for _, jr := range joinRows {
		ok := assocKey(jr[ownFK])
		tk := jr[otherFK]
		ownerToTargets[ok] = append(ownerToTargets[ok], tk)
		if k := assocKey(tk); !seenTarget[k] {
			seenTarget[k] = true
			targetIDs = append(targetIDs, tk)
		}
	}
	if len(targetIDs) == 0 {
		return nil
	}
	targets, err := LoadAll(a, tgt.Where(map[string]any{tgt.PrimaryKey: targetIDs}))
	if err != nil {
		return err
	}
	byKey := groupBy(targets, tgt.PrimaryKey)
	for _, p := range parents {
		var out []*Record
		for _, tk := range ownerToTargets[assocKey(p.attrs[m.PrimaryKey])] {
			out = append(out, byKey[assocKey(tk)]...)
		}
		p.setLoaded(name, out)
	}
	return nil
}

// preloadThrough loads the intermediate association, then the source association
// on the intermediates, and flattens the source targets onto each parent — the
// two-hop preload ActiveRecord runs for has_many/has_one :through.
func (m *Model) preloadThrough(a Adapter, parents []*Record, name string, assoc *Association) error {
	if err := Preload(a, parents, assoc.Through); err != nil {
		return err
	}
	// Gather all intermediate records across parents.
	var mids []*Record
	for _, p := range parents {
		mids = append(mids, p.loaded[assoc.Through]...)
	}
	midAssocDef := m.associations[assoc.Through]
	midModel, ok := m.resolveModel(midAssocDef.ClassName)
	if !ok || len(mids) == 0 {
		for _, p := range parents {
			p.setLoaded(name, nil)
		}
		return nil
	}
	// The source association is the through association's own name resolved on
	// the intermediate model (ActiveRecord's default source reflection).
	src := assoc.Name
	if midModel.associations[src] == nil {
		src = underscoreClass(assoc.ClassName)
	}
	if err := Preload(a, mids, src); err != nil {
		return err
	}
	for _, p := range parents {
		var out []*Record
		for _, mid := range p.loaded[assoc.Through] {
			out = append(out, mid.loaded[src]...)
		}
		p.setLoaded(name, out)
	}
	return nil
}

// collectValues returns the distinct non-nil values of a column across records,
// in first-seen order (the key set a preload query filters on).
func collectValues(recs []*Record, col string) []any {
	var out []any
	seen := map[string]bool{}
	for _, r := range recs {
		v, ok := r.attrs[col]
		if !ok || v == nil {
			continue
		}
		k := assocKey(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

// groupBy buckets records by the string key of a column value.
func groupBy(recs []*Record, col string) map[string][]*Record {
	out := map[string][]*Record{}
	for _, r := range recs {
		k := assocKey(r.attrs[col])
		out[k] = append(out[k], r)
	}
	return out
}

// assocKey renders a scalar association key (integer/string/…) to a stable map
// key so parents and children join on equal foreign-key values regardless of the
// concrete Go integer type a driver returned.
func assocKey(v any) string {
	if f, _, ok := toNumber(v); ok {
		return "n:" + num(f)
	}
	if s, ok := symbolName(v); ok {
		return "s:" + s
	}
	if v == nil {
		return "nil"
	}
	return "o:" + strings.TrimSpace(num2(v))
}
