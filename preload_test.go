// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func TestPreloadHasMany(t *testing.T) {
	user, _, _, _ := testModels(SQLite)
	u1 := user.Load(map[string]any{"id": int64(1), "name": "a"})
	u2 := user.Load(map[string]any{"id": int64(2), "name": "b"})
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{
		{"id": int64(10), "user_id": int64(1), "title": "p1"},
		{"id": int64(11), "user_id": int64(2), "title": "p2"},
	}}}
	if err := Preload(a, []*Record{u1, u2}, "posts"); err != nil {
		t.Fatal(err)
	}
	eq(t, "hm-sql", a.log[0], `SELECT "posts".* FROM "posts" WHERE "posts"."user_id" IN (1, 2)`)
	if got := u1.PreloadedAssociation("posts"); len(got) != 1 {
		t.Fatalf("u1 posts = %d", len(got))
	}
	if v, _ := u1.PreloadedAssociation("posts")[0].Get("title"); v != "p1" {
		t.Errorf("u1 post title = %v", v)
	}
	if len(u2.PreloadedAssociation("posts")) != 1 {
		t.Errorf("u2 posts = %d", len(u2.PreloadedAssociation("posts")))
	}
}

func TestPreloadBelongsTo(t *testing.T) {
	user, post, _, _ := testModels(SQLite)
	_ = user
	p1 := post.Load(map[string]any{"id": int64(10), "user_id": int64(1)})
	p2 := post.Load(map[string]any{"id": int64(11), "user_id": int64(2)})
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{{
		{"id": int64(1), "name": "a"}, {"id": int64(2), "name": "b"},
	}}}
	if err := Preload(a, []*Record{p1, p2}, "user"); err != nil {
		t.Fatal(err)
	}
	eq(t, "bt-sql", a.log[0], `SELECT "users".* FROM "users" WHERE "users"."id" IN (1, 2)`)
	if got := p1.PreloadedAssociation("user"); len(got) != 1 {
		t.Fatalf("p1 user = %d", len(got))
	}
}

func TestPreloadHABTM(t *testing.T) {
	user, _, _, _ := testModels(SQLite)
	u1 := user.Load(map[string]any{"id": int64(1)})
	u2 := user.Load(map[string]any{"id": int64(2)})
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{
		{{"user_id": int64(1), "role_id": int64(100)}, {"user_id": int64(2), "role_id": int64(100)}},
		{{"id": int64(100), "name": "admin"}},
	}}
	if err := Preload(a, []*Record{u1, u2}, "roles"); err != nil {
		t.Fatal(err)
	}
	eq(t, "habtm-join", a.log[0],
		`SELECT "roles_users".* FROM "roles_users" WHERE "roles_users"."user_id" IN (1, 2)`)
	eq(t, "habtm-target", a.log[1], `SELECT "roles".* FROM "roles" WHERE "roles"."id" = 100`)
	if len(u1.PreloadedAssociation("roles")) != 1 || len(u2.PreloadedAssociation("roles")) != 1 {
		t.Errorf("roles not attached: %d %d",
			len(u1.PreloadedAssociation("roles")), len(u2.PreloadedAssociation("roles")))
	}
}

func TestPreloadThrough(t *testing.T) {
	_, _, company, _ := testModels(SQLite)
	c1 := company.Load(map[string]any{"id": int64(1), "name": "acme"})
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{
		{{"id": int64(1), "company_id": int64(1)}, {"id": int64(2), "company_id": int64(1)}},
		{{"id": int64(10), "user_id": int64(1)}, {"id": int64(11), "user_id": int64(2)}},
	}}
	if err := Preload(a, []*Record{c1}, "posts"); err != nil {
		t.Fatal(err)
	}
	eq(t, "through-1", a.log[0], `SELECT "users".* FROM "users" WHERE "users"."company_id" = 1`)
	eq(t, "through-2", a.log[1], `SELECT "posts".* FROM "posts" WHERE "posts"."user_id" IN (1, 2)`)
	if got := c1.PreloadedAssociation("posts"); len(got) != 2 {
		t.Fatalf("through posts = %d", len(got))
	}
}

func TestLoadIncludes(t *testing.T) {
	user, _, _, _ := testModels(SQLite)
	rel := user.All().Includes("posts")
	if names := rel.IncludesNames(); len(names) != 1 || names[0] != "posts" {
		t.Errorf("includes = %v", names)
	}
	a := &recAdapter{name: "sqlite3", execRows: [][]Row{
		{{"id": int64(1), "name": "a"}},                        // root User.all
		{{"id": int64(10), "user_id": int64(1), "title": "p"}}, // preloaded posts
	}}
	roots, err := LoadIncludes(a, rel)
	if err != nil || len(roots) != 1 {
		t.Fatalf("load includes = %v %v", len(roots), err)
	}
	eq(t, "root", a.log[0], `SELECT "users".* FROM "users"`)
	eq(t, "child", a.log[1], `SELECT "posts".* FROM "posts" WHERE "posts"."user_id" = 1`)
	if len(roots[0].PreloadedAssociation("posts")) != 1 {
		t.Error("posts not preloaded on root")
	}
	// Includes(Symbol) and a non-name arg (ignored).
	if n := user.All().Includes(Symbol("posts"), 42).IncludesNames(); len(n) != 1 {
		t.Errorf("symbol includes = %v", n)
	}
}

func TestPreloadEdgeCases(t *testing.T) {
	user, _, _, _ := testModels(SQLite)
	a := &recAdapter{name: "sqlite3"}
	// Empty parents: no-op.
	if err := Preload(a, nil, "posts"); err != nil {
		t.Error(err)
	}
	// Unknown association: no-op.
	if err := Preload(a, []*Record{user.Load(map[string]any{"id": int64(1)})}, "nope"); err != nil {
		t.Error(err)
	}
	// Parents without the key column: no query issued.
	u := user.Load(map[string]any{"name": "no-id"})
	if err := Preload(a, []*Record{u}, "posts"); err != nil {
		t.Error(err)
	}
	if len(a.log) != 0 {
		t.Errorf("nil-id preload issued SQL: %#v", a.log)
	}
	// Association with an out-of-range kind: switch default returns nil.
	m := NewModel("A", "as", Column{"id", "integer"})
	m.associations["weird"] = &Association{Kind: AssocKind(99), Name: "weird", ClassName: "B"}
	if err := Preload(a, []*Record{m.Load(map[string]any{"id": int64(1)})}, "weird"); err != nil {
		t.Error(err)
	}
	// PreloadedAssociation on a record that never loaded anything.
	if user.Load(map[string]any{"id": int64(1)}).PreloadedAssociation("posts") != nil {
		t.Error("unloaded association should be nil")
	}
}

func TestPreloadUnresolvedTarget(t *testing.T) {
	// Associations pointing at unregistered classes hit the resolveModel !ok
	// branches in each preloader.
	a := &recAdapter{name: "sqlite3"}
	hm := NewModel("A", "as", Column{"id", "integer"})
	hm.HasMany("bs", "B")
	if err := Preload(a, []*Record{hm.Load(map[string]any{"id": int64(1)})}, "bs"); err != nil {
		t.Error(err)
	}
	bt := NewModel("A", "as", Column{"id", "integer"}, Column{"b_id", "bigint"})
	bt.BelongsTo("b", "B")
	if err := Preload(a, []*Record{bt.Load(map[string]any{"id": int64(1), "b_id": int64(2)})}, "b"); err != nil {
		t.Error(err)
	}
	ht := NewModel("A", "as", Column{"id", "integer"})
	ht.HABTM("bs", "B")
	if err := Preload(a, []*Record{ht.Load(map[string]any{"id": int64(1)})}, "bs"); err != nil {
		t.Error(err)
	}
	if len(a.log) != 0 {
		t.Errorf("unresolved target issued SQL: %#v", a.log)
	}
	// through whose intermediate resolves but has no mids.
	thr := NewModel("A", "as", Column{"id", "integer"})
	b := NewModel("B", "bs", Column{"id", "integer"}, Column{"a_id", "bigint"})
	thr.Register(b)
	thr.HasMany("bs", "B").HasMany("cs", "C", Through("bs"))
	arec := thr.Load(map[string]any{"id": int64(1)})
	a2 := &recAdapter{name: "sqlite3", execRows: [][]Row{{}}} // no intermediates
	if err := Preload(a2, []*Record{arec}, "cs"); err != nil {
		t.Error(err)
	}
	if arec.PreloadedAssociation("cs") != nil {
		t.Error("through with no mids should attach nothing")
	}
}

func TestPreloadErrorPaths(t *testing.T) {
	user, post, company, _ := testModels(SQLite)
	// has_many query error.
	if err := Preload(&recAdapter{name: "sqlite3", failOn: "posts"},
		[]*Record{user.Load(map[string]any{"id": int64(1)})}, "posts"); err == nil {
		t.Error("has_many error expected")
	}
	// belongs_to query error.
	if err := Preload(&recAdapter{name: "sqlite3", failOn: "users"},
		[]*Record{post.Load(map[string]any{"id": int64(1), "user_id": int64(1)})}, "user"); err == nil {
		t.Error("belongs_to error expected")
	}
	// HABTM join-query error.
	if err := Preload(&recAdapter{name: "sqlite3", failOn: "roles_users"},
		[]*Record{user.Load(map[string]any{"id": int64(1)})}, "roles"); err == nil {
		t.Error("habtm join error expected")
	}
	// HABTM target-query error (join rows returned, then targets fail).
	habtmErr := &recAdapter{name: "sqlite3", failOn: "FROM \"roles\" WHERE",
		execRows: [][]Row{{{"user_id": int64(1), "role_id": int64(9)}}}}
	if err := Preload(habtmErr, []*Record{user.Load(map[string]any{"id": int64(1)})}, "roles"); err == nil {
		t.Error("habtm target error expected")
	}
	// through first-hop error.
	if err := Preload(&recAdapter{name: "sqlite3", failOn: "users"},
		[]*Record{company.Load(map[string]any{"id": int64(1)})}, "posts"); err == nil {
		t.Error("through first hop error expected")
	}
	// through second-hop error.
	thrErr := &recAdapter{name: "sqlite3", failOn: "posts",
		execRows: [][]Row{{{"id": int64(1), "company_id": int64(1)}}}}
	if err := Preload(thrErr, []*Record{company.Load(map[string]any{"id": int64(1)})}, "posts"); err == nil {
		t.Error("through second hop error expected")
	}
	// LoadIncludes root error.
	if _, err := LoadIncludes(&recAdapter{name: "sqlite3", failOn: "users"},
		user.All().Includes("posts")); err == nil {
		t.Error("includes root error expected")
	}
	// LoadIncludes preload error (root ok, child fails).
	incErr := &recAdapter{name: "sqlite3", failOn: "posts",
		execRows: [][]Row{{{"id": int64(1)}}}}
	if _, err := LoadIncludes(incErr, user.All().Includes("posts")); err == nil {
		t.Error("includes preload error expected")
	}
}

func TestAssocKey(t *testing.T) {
	if assocKey(int64(3)) != assocKey(3) || assocKey(3) != "n:3" {
		t.Error("numeric key")
	}
	if assocKey(Symbol("x")) != "s:x" {
		t.Error("symbol key")
	}
	if assocKey(nil) != "nil" {
		t.Error("nil key")
	}
	if assocKey(true) != "o:true" {
		t.Errorf("bool key = %q", assocKey(true))
	}
}
