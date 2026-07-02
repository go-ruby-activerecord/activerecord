// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// AssocKind is the macro that declared an association.
type AssocKind int

const (
	// BelongsTo is belongs_to: this model holds the foreign key.
	BelongsTo AssocKind = iota
	// HasMany is has_many.
	HasMany
	// HasOne is has_one.
	HasOne
	// HABTM is has_and_belongs_to_many (a join table, no model).
	HABTM
)

// Association describes one declared association on a model.
type Association struct {
	Kind AssocKind
	// Name is the association name ("posts", "company").
	Name string
	// ClassName is the target model's Ruby class name ("Post"). For :through it
	// is the final target.
	ClassName string
	// ForeignKey overrides the default foreign-key column.
	ForeignKey string
	// Through, when set, names the intermediate association (has_many :through /
	// has_one :through).
	Through string
	// JoinTable overrides the HABTM join-table name.
	JoinTable string
	// AssociationForeignKey overrides the HABTM other-side key.
	AssociationForeignKey string
}

// BelongsTo declares a belongs_to association. className defaults from name
// (host handles classification when it differs) — here name is also used as the
// class-name stem via [singularizeClass].
func (m *Model) BelongsTo(name, className string, opts ...AssocOpt) *Model {
	return m.assoc(BelongsTo, name, className, opts)
}

// HasMany declares a has_many association.
func (m *Model) HasMany(name, className string, opts ...AssocOpt) *Model {
	return m.assoc(HasMany, name, className, opts)
}

// HasOne declares a has_one association.
func (m *Model) HasOne(name, className string, opts ...AssocOpt) *Model {
	return m.assoc(HasOne, name, className, opts)
}

// HABTM declares a has_and_belongs_to_many association.
func (m *Model) HABTM(name, className string, opts ...AssocOpt) *Model {
	return m.assoc(HABTM, name, className, opts)
}

func (m *Model) assoc(kind AssocKind, name, className string, opts []AssocOpt) *Model {
	a := &Association{Kind: kind, Name: name, ClassName: className}
	for _, o := range opts {
		o(a)
	}
	m.associations[name] = a
	return m
}

// AssocOpt configures an association.
type AssocOpt func(*Association)

// ForeignKey sets a non-default foreign key.
func ForeignKey(fk string) AssocOpt { return func(a *Association) { a.ForeignKey = fk } }

// Through sets the intermediate association name for has_many/has_one :through.
func Through(name string) AssocOpt { return func(a *Association) { a.Through = name } }

// JoinTable overrides the HABTM join-table name.
func JoinTable(name string) AssocOpt { return func(a *Association) { a.JoinTable = name } }

// AssociationForeignKey overrides the HABTM other-side key column.
func AssociationForeignKey(fk string) AssocOpt {
	return func(a *Association) { a.AssociationForeignKey = fk }
}

// Association returns the named association, or nil.
func (m *Model) Association(name string) *Association { return m.associations[name] }

// foreignKey returns the association's foreign-key column, defaulting to
// "<singular>_id" of the belongs_to owner (ActiveRecord's convention).
func (a *Association) foreignKey(ownerSingular string) string {
	if a.ForeignKey != "" {
		return a.ForeignKey
	}
	return ownerSingular + "_id"
}

// joinClauses renders the JOIN clause(s) for joining name from this model,
// resolving belongs_to/has_many/has_one/HABTM and :through.
func (m *Model) joinClauses(kind, name string) []string {
	a := m.associations[name]
	if a == nil {
		return nil
	}
	switch a.Kind {
	case BelongsTo:
		return m.belongsToJoin(kind, a)
	case HasMany, HasOne:
		if a.Through != "" {
			return m.throughJoin(kind, a)
		}
		return m.hasManyJoin(kind, a)
	case HABTM:
		return m.habtmJoin(kind, a)
	}
	return nil
}

// belongsToJoin: INNER JOIN "targets" ON "targets"."id" = "self"."target_id"
func (m *Model) belongsToJoin(kind string, a *Association) []string {
	tgt, ok := m.resolveModel(a.ClassName)
	if !ok {
		return nil
	}
	fk := a.foreignKey(underscoreClass(a.ClassName))
	on := m.Dialect.qualify(tgt.TableName, tgt.PrimaryKey) + " = " +
		m.Dialect.qualify(m.TableName, fk)
	return []string{kind + " " + m.Dialect.quoteTableName(tgt.TableName) + " ON " + on}
}

// hasManyJoin: INNER JOIN "targets" ON "targets"."self_id" = "self"."id"
func (m *Model) hasManyJoin(kind string, a *Association) []string {
	tgt, ok := m.resolveModel(a.ClassName)
	if !ok {
		return nil
	}
	fk := a.foreignKey(underscoreClass(m.Name))
	on := m.Dialect.qualify(tgt.TableName, fk) + " = " +
		m.Dialect.qualify(m.TableName, m.PrimaryKey)
	return []string{kind + " " + m.Dialect.quoteTableName(tgt.TableName) + " ON " + on}
}

// throughJoin: JOIN the intermediate, then JOIN the final target through it,
// matching ActiveRecord's two-hop join ordering.
func (m *Model) throughJoin(kind string, a *Association) []string {
	mid := m.associations[a.Through]
	if mid == nil {
		return nil
	}
	midModel, ok := m.resolveModel(mid.ClassName)
	if !ok {
		return nil
	}
	tgt, ok := m.resolveModel(a.ClassName)
	if !ok {
		return nil
	}
	// First hop: self -> intermediate (as has_many).
	first := m.hasManyJoin(kind, mid)
	// Second hop: intermediate -> target. The target's foreign key on the
	// intermediate table (has_many) or the intermediate's fk to target
	// (belongs_to) — use the has_many convention (target has <mid_singular>_id)
	// unless the target is reached via belongs_to on the intermediate.
	var on string
	if fk := tgt.belongsToFKTo(midModel); fk != "" {
		on = m.Dialect.qualify(tgt.TableName, tgt.PrimaryKey) + " = " +
			m.Dialect.qualify(midModel.TableName, fk)
	} else {
		fk = underscoreClass(midModel.Name) + "_id"
		on = m.Dialect.qualify(tgt.TableName, fk) + " = " +
			m.Dialect.qualify(midModel.TableName, midModel.PrimaryKey)
	}
	second := kind + " " + m.Dialect.quoteTableName(tgt.TableName) + " ON " + on
	return append(first, second)
}

// belongsToFKTo returns the foreign-key column on m that points at target via a
// belongs_to association, or "".
func (m *Model) belongsToFKTo(target *Model) string {
	for _, a := range m.associations {
		if a.Kind == BelongsTo && a.ClassName == target.Name {
			return a.foreignKey(underscoreClass(a.ClassName))
		}
	}
	return ""
}

// habtmJoin: JOIN the join table then the target, per ActiveRecord's HABTM.
func (m *Model) habtmJoin(kind string, a *Association) []string {
	tgt, ok := m.resolveModel(a.ClassName)
	if !ok {
		return nil
	}
	jt := a.JoinTable
	if jt == "" {
		jt = habtmJoinTable(m.TableName, tgt.TableName)
	}
	ownFK := a.ForeignKey
	if ownFK == "" {
		ownFK = underscoreClass(m.Name) + "_id"
	}
	otherFK := a.AssociationForeignKey
	if otherFK == "" {
		otherFK = underscoreClass(tgt.Name) + "_id"
	}
	j1on := m.Dialect.qualify(jt, ownFK) + " = " + m.Dialect.qualify(m.TableName, m.PrimaryKey)
	j2on := m.Dialect.qualify(tgt.TableName, tgt.PrimaryKey) + " = " + m.Dialect.qualify(jt, otherFK)
	return []string{
		kind + " " + m.Dialect.quoteTableName(jt) + " ON " + j1on,
		kind + " " + m.Dialect.quoteTableName(tgt.TableName) + " ON " + j2on,
	}
}
