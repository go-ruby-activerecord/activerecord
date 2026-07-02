// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

// testModels builds a small schema (User/Post/Company/Role) with the
// associations the SQL tests exercise, for the given dialect.
func testModels(d Dialect) (user, post, company, role *Model) {
	user = NewModel("User", "users",
		Column{"id", "integer"}, Column{"name", "string"}, Column{"age", "integer"},
		Column{"email", "string"}, Column{"active", "boolean"}, Column{"company_id", "bigint"})
	post = NewModel("Post", "posts", Column{"id", "integer"}, Column{"title", "string"}, Column{"user_id", "bigint"})
	company = NewModel("Company", "companies", Column{"id", "integer"}, Column{"name", "string"})
	role = NewModel("Role", "roles", Column{"id", "integer"}, Column{"name", "string"})
	for _, m := range []*Model{user, post, company, role} {
		m.Dialect = d
		m.Register(user, post, company, role)
	}
	user.BelongsTo("company", "Company").HasMany("posts", "Post").HABTM("roles", "Role")
	post.BelongsTo("user", "User")
	company.HasMany("users", "User").HasMany("posts", "Post", Through("users"))
	return
}

func eq(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s:\n got %s\nwant %s", name, got, want)
	}
}

func TestWhereForms(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "hash", u.Where(map[string]any{"name": "bob"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" = 'bob'`)
	eq(t, "sym-hash", u.Where(map[Symbol]any{"name": "bob"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" = 'bob'`)
	eq(t, "multi", u.Where(map[string]any{"name": "bob", "age": 30}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" = 30 AND "users"."name" = 'bob'`)
	eq(t, "placeholder", u.Where("age > ?", 18).ToSQL(),
		`SELECT "users".* FROM "users" WHERE (age > 18)`)
	eq(t, "named", u.Where("age > :min", map[string]any{"min": 18}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE (age > 18)`)
	eq(t, "named-sym", u.Where("age > :min", map[Symbol]any{"min": 18}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE (age > 18)`)
	eq(t, "raw", u.Where("active").ToSQL(),
		`SELECT "users".* FROM "users" WHERE (active)`)
	eq(t, "array-in", u.Where(map[string]any{"age": []any{1, 2, 3}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" IN (1, 2, 3)`)
	eq(t, "empty-in", u.Where(map[string]any{"age": []any{}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" IN (NULL)`)
	eq(t, "nil", u.Where(map[string]any{"name": nil}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" IS NULL`)
	eq(t, "range", u.Where(map[string]any{"age": &Range{Begin: 18, End: 30}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" BETWEEN 18 AND 30`)
	eq(t, "range-val", u.Where(map[string]any{"age": Range{Begin: 18, End: 30}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" BETWEEN 18 AND 30`)
	eq(t, "range-excl", u.Where(map[string]any{"age": &Range{Begin: 18, End: 30, Exclusive: true}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" >= 18 AND "users"."age" < 30`)
	eq(t, "range-beginless", u.Where(map[string]any{"age": &Range{End: 30}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" <= 30`)
	eq(t, "range-beginless-excl", u.Where(map[string]any{"age": &Range{End: 30, Exclusive: true}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" < 30`)
	eq(t, "range-endless", u.Where(map[string]any{"age": &Range{Begin: 18}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" >= 18`)
	eq(t, "range-empty", u.Where(map[string]any{"age": &Range{}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE 1=1`)
	// no-op forms.
	eq(t, "where-none", u.All().Where().ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "where-badtype", u.Where(42).ToSQL(), `SELECT "users".* FROM "users"`)
}

func TestNotAndOr(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "not-eq", u.All().Not(map[string]any{"name": "bob"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" != 'bob'`)
	eq(t, "not-nil", u.All().Not(map[string]any{"name": nil}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" IS NOT NULL`)
	eq(t, "not-in", u.All().Not(map[string]any{"age": []any{1, 2}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" NOT IN (1, 2)`)
	eq(t, "not-in-empty", u.All().Not(map[string]any{"age": []any{}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE 1=1`)
	eq(t, "not-range", u.All().Not(map[string]any{"age": &Range{Begin: 1, End: 2}}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE NOT ("users"."age" BETWEEN 1 AND 2)`)
	eq(t, "not-sym", u.All().Not(map[Symbol]any{"name": "x"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" != 'x'`)
	eq(t, "not-none", u.All().Not().ToSQL(), `SELECT "users".* FROM "users"`)

	eq(t, "or", u.Where(map[string]any{"name": "bob"}).Or(u.Where(map[string]any{"age": 30})).ToSQL(),
		`SELECT "users".* FROM "users" WHERE ("users"."name" = 'bob' OR "users"."age" = 30)`)
	eq(t, "or-left-empty", u.All().Or(u.Where(map[string]any{"age": 30})).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."age" = 30`)
	eq(t, "or-right-empty", u.Where(map[string]any{"name": "x"}).Or(u.All()).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."name" = 'x'`)
	eq(t, "or-both-empty", u.All().Or(u.All()).ToSQL(), `SELECT "users".* FROM "users"`)
}

func TestSelectOrderGroupDistinct(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "select", u.Select("id", "name").ToSQL(),
		`SELECT "users"."id", "users"."name" FROM "users"`)
	eq(t, "select-raw", u.Select("COUNT(*)").ToSQL(),
		`SELECT COUNT(*) FROM "users"`)
	eq(t, "select-sym", u.Select(Symbol("name")).ToSQL(),
		`SELECT "users"."name" FROM "users"`)
	eq(t, "select-bad", u.Select(42).ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "distinct", u.All().Distinct().ToSQL(), `SELECT DISTINCT "users".* FROM "users"`)
	eq(t, "distinct-false", u.All().Distinct(false).ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "order", u.Order("age").ToSQL(), `SELECT "users".* FROM "users" ORDER BY "users"."age" ASC`)
	eq(t, "order-desc-map", u.Order(map[string]any{"age": "desc"}).ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."age" DESC`)
	eq(t, "order-map-sym", u.Order(map[string]any{"age": Symbol("asc")}).ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."age" ASC`)
	eq(t, "order-multi-map", u.Order(map[string]any{"age": "desc", "name": "asc"}).ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."age" DESC, "users"."name" ASC`)
	eq(t, "order-raw", u.Order("age DESC").ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY age DESC`)
	eq(t, "order-bad", u.Order(42).ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "group", u.All().Group("age").ToSQL(),
		`SELECT "users".* FROM "users" GROUP BY "users"."age"`)
	eq(t, "group-bad", u.All().Group(42).ToSQL(), `SELECT "users".* FROM "users"`)
	eq(t, "group-having", u.All().Group("age").Having("count(*) > ?", 1).ToSQL(),
		`SELECT "users".* FROM "users" GROUP BY "users"."age" HAVING (count(*) > 1)`)
	eq(t, "having-none", u.All().Having().ToSQL(), `SELECT "users".* FROM "users"`)
}

func TestLimitOffsetAndFinders(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "limit-offset", u.All().Limit(10).Offset(5).ToSQL(),
		`SELECT "users".* FROM "users" LIMIT 10 OFFSET 5`)
	eq(t, "first", u.All().First().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."id" ASC LIMIT 1`)
	eq(t, "first-with-order", u.Order("name").First().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."name" ASC LIMIT 1`)
	eq(t, "last", u.All().Last().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."id" DESC LIMIT 1`)
	eq(t, "last-with-order", u.Order("name").Last().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."name" DESC LIMIT 1`)
	eq(t, "last-multi-order", u.Order("age").Order("name").Last().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."name" DESC, "users"."age" DESC LIMIT 1`)
	eq(t, "last-desc-order", u.Order(map[string]any{"age": "desc"}).Last().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY "users"."age" ASC LIMIT 1`)
	eq(t, "last-raw-order", u.Order("age DESC").Last().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY age ASC LIMIT 1`)
	eq(t, "last-raw-noword", u.Order("length(name)").Last().ToSQL(),
		`SELECT "users".* FROM "users" ORDER BY length(name) DESC LIMIT 1`)
	eq(t, "take", u.All().Take().ToSQL(), `SELECT "users".* FROM "users" LIMIT 1`)
	eq(t, "find", u.All().Find(7).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."id" = 7 LIMIT 1`)
	eq(t, "find-by", u.All().FindBy(map[string]any{"email": "a@b"}).ToSQL(),
		`SELECT "users".* FROM "users" WHERE "users"."email" = 'a@b' LIMIT 1`)
}

func TestMergeChainScope(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	u.Scope("adults", func(r *Relation) *Relation { return r.Where("age >= ?", 18) })
	eq(t, "scope", u.All().Scope("adults").ToSQL(),
		`SELECT "users".* FROM "users" WHERE (age >= 18)`)
	eq(t, "scope-missing", u.All().Scope("nope").ToSQL(), `SELECT "users".* FROM "users"`)
	base := u.Where(map[string]any{"active": true})
	other := u.Where(map[string]any{"age": 30}).Order("name").Group("age").
		Having("count(*) > ?", 1).Limit(5).Offset(2).Distinct().Select("id").Joins("posts")
	eq(t, "merge", base.Merge(other).ToSQL(),
		`SELECT DISTINCT "users"."id" FROM "users" INNER JOIN "posts" ON "posts"."user_id" = "users"."id" WHERE "users"."active" = TRUE AND "users"."age" = 30 GROUP BY "users"."age" HAVING (count(*) > 1) ORDER BY "users"."name" ASC LIMIT 5 OFFSET 2`)
	// chaining leaves receiver unchanged.
	r := u.Where(map[string]any{"active": true})
	_ = r.Order("name")
	eq(t, "immutable", r.ToSQL(), `SELECT "users".* FROM "users" WHERE "users"."active" = TRUE`)
}
