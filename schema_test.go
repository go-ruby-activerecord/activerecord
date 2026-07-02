// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func TestCreateTableSQLite(t *testing.T) {
	tbl := CreateTable(SQLite, "users").
		Column("name", "string", NotNull()).
		Column("age", "integer", Default(0)).
		References("company").
		Timestamps()
	eq(t, "create", tbl.ToSQL(),
		`CREATE TABLE "users" ("id" integer PRIMARY KEY AUTOINCREMENT NOT NULL, "name" varchar NOT NULL, "age" integer DEFAULT 0, "company_id" bigint, "created_at" datetime(6) NOT NULL, "updated_at" datetime(6) NOT NULL)`)
	// no primary key.
	np := CreateTable(SQLite, "joins").NoPrimaryKey().References("user").References("role")
	eq(t, "nopk", np.ToSQL(),
		`CREATE TABLE "joins" ("user_id" bigint, "role_id" bigint)`)
	// limit option accepted (no visible effect for sqlite varchar).
	l := CreateTable(SQLite, "t").NoPrimaryKey().Column("n", "string", Limit(20))
	eq(t, "limit", l.ToSQL(), `CREATE TABLE "t" ("n" varchar)`)
}

func TestCreateTablePerDialect(t *testing.T) {
	pg := CreateTable(Postgres, "users").Column("name", "string").ToSQL()
	eq(t, "pg", pg, `CREATE TABLE "users" ("id" bigserial primary key, "name" character varying)`)
	my := CreateTable(MySQL, "users").Column("name", "string").ToSQL()
	eq(t, "mysql", my, "CREATE TABLE `users` (`id` bigint auto_increment PRIMARY KEY, `name` varchar(255))")
}

func TestAddColumnIndexFK(t *testing.T) {
	eq(t, "add-col", AddColumnSQL(SQLite, "users", "email", "string", Limit(100)),
		`ALTER TABLE "users" ADD "email" varchar`)
	eq(t, "add-col-notnull", AddColumnSQL(SQLite, "users", "flag", "boolean", NotNull(), Default(false)),
		`ALTER TABLE "users" ADD "flag" boolean DEFAULT FALSE NOT NULL`)
	eq(t, "index", AddIndexSQL(SQLite, "users", []string{"name"}, false, ""),
		`CREATE INDEX "index_users_on_name" ON "users" ("name")`)
	eq(t, "uindex", AddIndexSQL(SQLite, "users", []string{"email"}, true, ""),
		`CREATE UNIQUE INDEX "index_users_on_email" ON "users" ("email")`)
	eq(t, "multi-index", AddIndexSQL(SQLite, "users", []string{"a", "b"}, false, ""),
		`CREATE INDEX "index_users_on_a_and_b" ON "users" ("a", "b")`)
	eq(t, "named-index", AddIndexSQL(SQLite, "users", []string{"name"}, false, "by_name"),
		`CREATE INDEX "by_name" ON "users" ("name")`)
	eq(t, "fk", AddForeignKeySQL(SQLite, "posts", "users", "", "", ""),
		`ALTER TABLE "posts" ADD CONSTRAINT "fk_rails_posts_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id")`)
	eq(t, "fk-explicit", AddForeignKeySQL(Postgres, "posts", "people", "author_id", "uid", "fk1"),
		`ALTER TABLE "posts" ADD CONSTRAINT "fk1" FOREIGN KEY ("author_id") REFERENCES "people" ("uid")`)
	// table not ending in "s".
	eq(t, "fk-singular", AddForeignKeySQL(SQLite, "a", "media", "", "", "fkm"),
		`ALTER TABLE "a" ADD CONSTRAINT "fkm" FOREIGN KEY ("media_id") REFERENCES "media" ("id")`)
	eq(t, "fk-nos", AddForeignKeySQL(SQLite, "a", "fish", "", "", "fkf"),
		`ALTER TABLE "a" ADD CONSTRAINT "fkf" FOREIGN KEY ("fish_id") REFERENCES "fish" ("id")`)
}

func TestTypeToSQLAllTypes(t *testing.T) {
	types := []string{"string", "text", "integer", "bigint", "float", "decimal",
		"datetime", "timestamp", "time", "date", "binary", "boolean", "custom"}
	for _, d := range []Dialect{SQLite, Postgres, MySQL} {
		for _, ty := range types {
			if got := d.typeToSQL(ty); got == "" {
				t.Errorf("%v.typeToSQL(%q) empty", d, ty)
			}
		}
		if got := d.typeToSQL("custom"); got != "custom" {
			t.Errorf("%v custom = %q", d, got)
		}
	}
	// spot-check a few mappings.
	if SQLite.typeToSQL("string") != "varchar" || Postgres.typeToSQL("string") != "character varying" ||
		MySQL.typeToSQL("string") != "varchar(255)" {
		t.Error("string types")
	}
	if SQLite.typeToSQL("binary") != "blob" || Postgres.typeToSQL("binary") != "bytea" ||
		MySQL.typeToSQL("boolean") != "tinyint(1)" {
		t.Error("binary/bool types")
	}
	// primaryKeySQL per dialect.
	if SQLite.primaryKeySQL() == "" || Postgres.primaryKeySQL() == "" || MySQL.primaryKeySQL() == "" {
		t.Error("pk sql")
	}
}
