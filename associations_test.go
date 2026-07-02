// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func TestJoinSQL(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "belongs-to", u.Joins("company").ToSQL(),
		`SELECT "users".* FROM "users" INNER JOIN "companies" ON "companies"."id" = "users"."company_id"`)
	eq(t, "has-many", u.Joins("posts").ToSQL(),
		`SELECT "users".* FROM "users" INNER JOIN "posts" ON "posts"."user_id" = "users"."id"`)
	eq(t, "left", u.All().LeftJoins("posts").ToSQL(),
		`SELECT "users".* FROM "users" LEFT OUTER JOIN "posts" ON "posts"."user_id" = "users"."id"`)
	eq(t, "habtm", u.Joins("roles").ToSQL(),
		`SELECT "users".* FROM "users" INNER JOIN "roles_users" ON "roles_users"."user_id" = "users"."id" INNER JOIN "roles" ON "roles"."id" = "roles_users"."role_id"`)
	eq(t, "two", u.Joins("company").Joins("posts").ToSQL(),
		`SELECT "users".* FROM "users" INNER JOIN "companies" ON "companies"."id" = "users"."company_id" INNER JOIN "posts" ON "posts"."user_id" = "users"."id"`)
	// through: Company has_many posts through users; users->posts is has_many.
	_, _, c, _ := testModels(SQLite)
	eq(t, "through", c.Joins("posts").ToSQL(),
		`SELECT "companies".* FROM "companies" INNER JOIN "users" ON "users"."company_id" = "companies"."id" INNER JOIN "posts" ON "posts"."user_id" = "users"."id"`)
	// bad / missing association is a no-op.
	eq(t, "missing", u.Joins("nope").ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "bad-arg", u.Joins(42).ToSQL(), `SELECT "users".* FROM "users"`)
}

func TestThroughBelongsToTarget(t *testing.T) {
	// A has_many :through where the target is reached via a belongs_to on the
	// intermediate (Author has_many :magazines through: :articles, articles
	// belong_to :magazine).
	author := NewModel("Author", "authors", Column{"id", "integer"})
	article := NewModel("Article", "articles", Column{"id", "integer"}, Column{"author_id", "bigint"}, Column{"magazine_id", "bigint"})
	magazine := NewModel("Magazine", "magazines", Column{"id", "integer"})
	author.Register(article, magazine)
	article.Register(author, magazine)
	author.HasMany("articles", "Article").HasMany("magazines", "Magazine", Through("articles"))
	article.BelongsTo("magazine", "Magazine").BelongsTo("author", "Author")
	eq(t, "through-belongs", author.Joins("magazines").ToSQL(),
		`SELECT "authors".* FROM "authors" INNER JOIN "articles" ON "articles"."author_id" = "authors"."id" INNER JOIN "magazines" ON "magazines"."id" = "articles"."magazine_id"`)
}

func TestAssociationOptions(t *testing.T) {
	u := NewModel("User", "users", Column{"id", "integer"})
	post := NewModel("Post", "posts", Column{"id", "integer"}, Column{"writer_id", "bigint"})
	role := NewModel("Role", "roles", Column{"id", "integer"})
	u.Register(post, role)
	u.HasMany("posts", "Post", ForeignKey("writer_id")).
		HABTM("roles", "Role", JoinTable("memberships"), ForeignKey("member_id"), AssociationForeignKey("group_id"))
	eq(t, "fk-override", u.Joins("posts").ToSQL(),
		`SELECT "users".* FROM "users" INNER JOIN "posts" ON "posts"."writer_id" = "users"."id"`)
	eq(t, "habtm-override", u.Joins("roles").ToSQL(),
		`SELECT "users".* FROM "users" INNER JOIN "memberships" ON "memberships"."member_id" = "users"."id" INNER JOIN "roles" ON "roles"."id" = "memberships"."group_id"`)
	if u.Association("posts") == nil || u.Association("posts").Kind != HasMany {
		t.Error("association lookup")
	}
	if u.Association("nope") != nil {
		t.Error("missing association")
	}
	// belongs_to fk override.
	post.BelongsTo("author", "User", ForeignKey("writer_id")).Register(u)
	eq(t, "bt-fk", post.Joins("author").ToSQL(),
		`SELECT "posts".* FROM "posts" INNER JOIN "users" ON "users"."id" = "posts"."writer_id"`)
}

func TestJoinUnresolvedTargets(t *testing.T) {
	// Associations whose target model is not registered are no-ops (each kind).
	u := NewModel("User", "users", Column{"id", "integer"})
	u.BelongsTo("company", "Company").HasMany("posts", "Post").HABTM("roles", "Role")
	u.HasMany("mags", "Magazine", Through("posts"))
	eq(t, "bt-unreg", u.Joins("company").ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "hm-unreg", u.Joins("posts").ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "habtm-unreg", u.Joins("roles").ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "through-unreg", u.Joins("mags").ToSQL(), `SELECT "users".* FROM "users"`)
	// through whose intermediate association is missing entirely.
	u2 := NewModel("U", "us", Column{"id", "integer"})
	u2.HasMany("x", "X", Through("nope"))
	eq(t, "through-nomid", u2.Joins("x").ToSQL(), `SELECT "us".* FROM "us"`)
	// through whose target model is unresolved is a no-op (AR would raise).
	mid := NewModel("Mid", "mids", Column{"id", "integer"}, Column{"u_id", "bigint"})
	u3 := NewModel("U", "us", Column{"id", "integer"}).Register(mid)
	u3.HasMany("mids", "Mid").HasMany("tg", "Target", Through("mids"))
	eq(t, "through-badtarget", u3.Joins("tg").ToSQL(), `SELECT "us".* FROM "us"`)
	// through whose intermediate model is unresolved.
	u4 := NewModel("U", "us", Column{"id", "integer"})
	u4.HasMany("mids", "Mid").HasMany("tg", "Target", Through("mids"))
	eq(t, "through-badmid", u4.Joins("tg").ToSQL(), `SELECT "us".* FROM "us"`)
}

func TestRegisterAndResolve(t *testing.T) {
	m := NewModel("A", "as").Register(nil)
	if _, ok := m.resolveModel("A"); !ok {
		t.Error("self resolve")
	}
	if _, ok := m.resolveModel("Z"); ok {
		t.Error("unknown resolve")
	}
}
