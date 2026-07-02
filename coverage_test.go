// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

// This file drives the remaining branches (rarely-reached options and helper
// edge cases) to keep the deterministic suite at 100% statement coverage.
package activerecord

import (
	"math/big"
	"testing"
	"time"
)

func TestFromOverride(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "from", u.All().From("archived_users").ToSQL(),
		`SELECT "users".* FROM "archived_users"`)
	eq(t, "from-where", u.Where(map[string]any{"name": "x"}).From("t").ToSQL(),
		`SELECT "users".* FROM "t" WHERE "users"."name" = 'x'`)
	// clearing restores the model table.
	eq(t, "from-clear", u.All().From("t").From("").ToSQL(), `SELECT "users".* FROM "users"`)
}

func TestOrderMapRawKey(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	// A map key that is a raw expression passes through in qualifyOrder.
	eq(t, "order-map-raw", u.Order(map[string]any{"length(name)": "desc"}).ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY length(name) DESC`)
}

func TestIsBareIdentifierEmpty(t *testing.T) {
	if isBareIdentifier("") {
		t.Error("empty is not a bare identifier")
	}
	if !isBareIdentifier("abc_1") {
		t.Error("abc_1 is bare")
	}
	if isBareIdentifier("a b") {
		t.Error("space not bare")
	}
}

func TestHasManyToSourceFK(t *testing.T) {
	// has_many :through where the intermediate's source has_many uses a custom
	// foreign key: Blog has_many comments through posts, posts has_many comments
	// with foreign_key "article_id".
	blog := NewModel("Blog", "blogs", Column{"id", "integer"})
	post := NewModel("Post", "posts", Column{"id", "integer"}, Column{"blog_id", "bigint"})
	comment := NewModel("Comment", "comments", Column{"id", "integer"}, Column{"article_id", "bigint"})
	blog.Register(post, comment)
	post.Register(blog, comment)
	blog.HasMany("posts", "Post").HasMany("comments", "Comment", Through("posts"))
	post.HasMany("comments", "Comment", ForeignKey("article_id"))
	eq(t, "source-fk", blog.Joins("comments").ToSQL(),
		`SELECT "blogs".* FROM "blogs" INNER JOIN "posts" ON "posts"."blog_id" = "blogs"."id" INNER JOIN "comments" ON "comments"."article_id" = "posts"."id"`)
}

func TestValueEqual2Branches(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !valueEqual2(t0, t0) {
		t.Error("time == time")
	}
	if valueEqual2(t0, "not-a-time") {
		t.Error("time vs non-time")
	}
	if !valueEqual2(1, 1) {
		t.Error("int == int")
	}
}

func TestToNumberIntTypes(t *testing.T) {
	if f, isInt, ok := toNumber(int64(5)); !ok || !isInt || f != 5 {
		t.Error("int64")
	}
	if f, isInt, ok := toNumber(5); !ok || !isInt || f != 5 {
		t.Error("int")
	}
	if _, isInt, _ := toNumber(2.5); isInt {
		t.Error("float non-int")
	}
	if _, isInt, _ := toNumber(float32(2.5)); isInt {
		t.Error("float32 non-int")
	}
}

func TestJoinInvalidKind(t *testing.T) {
	// An association with an out-of-range Kind reaches joinClauses' final nil.
	m := NewModel("A", "as", Column{"id", "integer"})
	m.associations["weird"] = &Association{Kind: AssocKind(99), Name: "weird", ClassName: "B"}
	eq(t, "invalid-kind", m.Joins("weird").ToSQL(), `SELECT "as".* FROM "as"`)
}

func TestHasManyToNoMatch(t *testing.T) {
	a := NewModel("A", "as", Column{"id", "integer"})
	b := NewModel("B", "bs", Column{"id", "integer"})
	// A has only a belongs_to B, so hasManyTo(B) finds nothing.
	a.BelongsTo("b", "B")
	if a.hasManyTo(b) != nil {
		t.Error("hasManyTo should be nil")
	}
	// And a has_one to a different target is skipped.
	c := NewModel("C", "cs", Column{"id", "integer"})
	a.HasOne("c", "C")
	if a.hasManyTo(b) != nil {
		t.Error("hasManyTo still nil for B")
	}
	if a.hasManyTo(c) == nil {
		t.Error("hasManyTo finds C")
	}
}

func TestToNumberBigAndInt32(t *testing.T) {
	if f, isInt, ok := toNumber(int32(7)); !ok || !isInt || f != 7 {
		t.Error("int32")
	}
	bi := big.NewInt(42)
	if f, isInt, ok := toNumber(bi); !ok || !isInt || f != 42 {
		t.Error("bigint")
	}
}

func TestHasOneAssociation(t *testing.T) {
	// has_one uses the has_many join shape (target holds the FK).
	u := NewModel("User", "users", Column{"id", "integer"})
	acct := NewModel("Account", "accounts", Column{"id", "integer"}, Column{"user_id", "bigint"})
	u.Register(acct)
	u.HasOne("account", "Account")
	eq(t, "has-one", u.Joins("account").ToSQL(),
		`SELECT "users".* FROM "users" INNER JOIN "accounts" ON "accounts"."user_id" = "users"."id"`)
}
