// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func TestAggregates(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "count", u.All().CountSQL(), `SELECT COUNT(*) FROM "users"`)
	eq(t, "count-col", u.All().CountColumnSQL("age"), `SELECT COUNT("users"."age") FROM "users"`)
	eq(t, "sum", u.All().SumSQL("age"), `SELECT SUM("users"."age") FROM "users"`)
	eq(t, "avg", u.All().AverageSQL("age"), `SELECT AVG("users"."age") FROM "users"`)
	eq(t, "min", u.All().MinimumSQL("age"), `SELECT MIN("users"."age") FROM "users"`)
	eq(t, "max", u.All().MaximumSQL("age"), `SELECT MAX("users"."age") FROM "users"`)
	eq(t, "sum-where", u.Where(map[string]any{"active": true}).SumSQL("age"),
		`SELECT SUM("users"."age") FROM "users" WHERE "users"."active" = TRUE`)
	eq(t, "count-join", u.Joins("posts").CountSQL(),
		`SELECT COUNT(*) FROM "users" INNER JOIN "posts" ON "posts"."user_id" = "users"."id"`)
	eq(t, "grouped", u.All().Group("age").CountSQL(),
		`SELECT "users"."age", COUNT(*) FROM "users" GROUP BY "users"."age"`)
	eq(t, "grouped-having", u.All().Group("age").Having("COUNT(*) > ?", 1).SumSQL("age"),
		`SELECT "users"."age", SUM("users"."age") FROM "users" GROUP BY "users"."age" HAVING (COUNT(*) > 1)`)
}

func TestPluckExists(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "pluck", u.All().PluckSQL("name"), `SELECT "users"."name" FROM "users"`)
	eq(t, "pluck-multi", u.All().PluckSQL("id", "name"),
		`SELECT "users"."id", "users"."name" FROM "users"`)
	eq(t, "pluck-where", u.Where(map[string]any{"active": true}).PluckSQL("name"),
		`SELECT "users"."name" FROM "users" WHERE "users"."active" = TRUE`)
	eq(t, "pluck-bad", u.All().PluckSQL(42), `SELECT "users".* FROM "users"`)
	eq(t, "exists", u.All().ExistsSQL(), `SELECT 1 AS one FROM "users" LIMIT 1`)
	eq(t, "exists-where", u.Where(map[string]any{"name": "x"}).ExistsSQL(),
		`SELECT 1 AS one FROM "users" WHERE "users"."name" = 'x' LIMIT 1`)
	eq(t, "exists-join", u.Joins("posts").ExistsSQL(),
		`SELECT 1 AS one FROM "users" INNER JOIN "posts" ON "posts"."user_id" = "users"."id" LIMIT 1`)
}

func TestDML(t *testing.T) {
	u, _, _, _ := testModels(SQLite)
	eq(t, "insert", u.InsertSQL(map[string]any{"name": "bob", "age": 5}),
		`INSERT INTO "users" ("age", "name") VALUES (5, 'bob')`)
	eq(t, "update-all", u.Where(map[string]any{"active": false}).UpdateAllSQL(map[string]any{"active": true}),
		`UPDATE "users" SET "active" = TRUE WHERE "users"."active" = FALSE`)
	eq(t, "update-all-nowhere", u.All().UpdateAllSQL(map[string]any{"age": 0}),
		`UPDATE "users" SET "age" = 0`)
	eq(t, "delete-all", u.Where(map[string]any{"age": 5}).DeleteAllSQL(),
		`DELETE FROM "users" WHERE "users"."age" = 5`)
	eq(t, "delete-all-nowhere", u.All().DeleteAllSQL(), `DELETE FROM "users"`)
	if itoa(7) != "7" {
		t.Error("itoa")
	}
}
