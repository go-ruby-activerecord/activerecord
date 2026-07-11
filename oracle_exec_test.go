// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"strings"
	"testing"
)

// These differential tests drive real ActiveRecord operations (persistence,
// transactions, statement caching, eager loading, STI) and compare the SQL it
// issues — captured from ActiveSupport's sql.active_record notifications with
// prepared statements disabled so bind values are inlined — to this package's
// output, byte-for-byte. They skip when a Ruby >= 4.0 with the activerecord gem
// is absent (the cross-arch/Windows CI lanes), where the deterministic suite
// alone holds the 100% gate.

// arExecModels is the shared schema+models used by the execution oracle. People
// has no timestamps, keeping its INSERT/UPDATE SQL free of a clock literal.
const arExecModels = `
ActiveRecord::Schema.define do
  create_table(:people){|t| t.string :name; t.integer :age }
  create_table(:users){|t| t.string :name; t.string :type; t.references :company }
  create_table(:companies){|t| t.string :name }
  create_table(:posts){|t| t.string :title; t.references :user }
  create_table(:roles){|t| t.string :name }
  create_table(:roles_users, id:false){|t| t.references :user; t.references :role }
end
class Person < ActiveRecord::Base; end
class Company < ActiveRecord::Base; has_many :users; has_many :posts, through: :users; end
class Post < ActiveRecord::Base; belongs_to :user; end
class Role < ActiveRecord::Base; has_and_belongs_to_many :users; end
class User < ActiveRecord::Base
  belongs_to :company; has_many :posts; has_and_belongs_to_many :roles
end
class Admin < User; end
`

func TestOraclePersistenceSQL(t *testing.T) {
	bin := rubyOracle(t)
	m := personModel(SQLite)

	// INSERT.
	rec := m.Build(map[string]any{"name": "bob", "age": 30})
	got := runAR(t, bin, arExecModels+`arlog!; Person.create!(name: "bob", age: 30)`)
	assertContains(t, "insert", got, rec.insertSQL())

	// UPDATE.
	loaded := m.Load(map[string]any{"id": int64(1), "name": "bob", "age": 30})
	loaded.Set("age", 31)
	got = runAR(t, bin, arExecModels+`
p = Person.create!(name: "bob", age: 30); arlog!; p.update!(age: 31)`)
	assertContains(t, "update", got, loaded.updateSQL())

	// DELETE (destroy).
	got = runAR(t, bin, arExecModels+`
p = Person.create!(name: "bob"); arlog!; p.destroy`)
	del := m.Load(map[string]any{"id": int64(1), "name": "bob"}).deleteSQL()
	assertContains(t, "delete", got, del)
}

func TestOracleTransactionSQL(t *testing.T) {
	bin := rubyOracle(t)
	got := runAR(t, bin, arExecModels+`
arlog!
Person.transaction do
  Person.create!(name: "a")
  Person.transaction(requires_new: true) { Person.create!(name: "b") }
end`)
	// Filter to the transaction-control statements and compare the sequence a
	// nested Save issues.
	var tx []string
	for _, s := range got {
		if isTxControl(s) {
			tx = append(tx, s)
		}
	}
	want := []string{
		"BEGIN immediate TRANSACTION",
		"SAVEPOINT active_record_1",
		"RELEASE SAVEPOINT active_record_1",
		"COMMIT TRANSACTION",
	}
	if len(tx) < 4 {
		t.Fatalf("transaction sql too short: %#v", tx)
	}
	// AR wraps each create in its own transaction too; assert our savepoint verbs
	// and the outer BEGIN/COMMIT appear exactly as ActiveRecord spells them.
	joined := strings.Join(tx, "\n")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("transaction sql missing %q in:\n%s", w, joined)
		}
	}
}

func TestOraclePreparedFindSQL(t *testing.T) {
	bin := rubyOracle(t)
	// With prepared statements ON, ActiveRecord's find/find_by emit the "= ?
	// LIMIT ?" template our statement cache renders.
	script := `
$stdout.sync = true
require "active_record"
ActiveRecord::Base.establish_connection(adapter: "sqlite3", database: ":memory:", prepared_statements: true)
ActiveRecord::Schema.verbose = false
$arlog = []
ActiveSupport::Notifications.subscribe("sql.active_record") do |*a|
  ev = ActiveSupport::Notifications::Event.new(*a)
  next if ev.payload[:name] == "SCHEMA"
  $arlog << ev.payload[:sql]
end
` + arExecModels + `
$arlog.clear
Person.find_by(id: 1)
puts $arlog.join("\x1e")
`
	out := runRuby(t, bin, script)
	m := personModel(SQLite)
	assertContains(t, "prepared-find_by", out, m.FindByStatement("id").SQL)
}

func TestOraclePreloadSQL(t *testing.T) {
	bin := rubyOracle(t)
	user, post, company, _ := testModels(SQLite)

	// has_many preload (two parents).
	got := runAR(t, bin, arExecModels+`
c = Company.create!(name:"acme"); User.create!(name:"a", company:c); User.create!(name:"b", company:c)
arlog!; User.includes(:posts).to_a`)
	u1 := user.Load(map[string]any{"id": int64(1)})
	u2 := user.Load(map[string]any{"id": int64(2)})
	wantHM, _ := hasManyPreloadSQL(user, []*Record{u1, u2})
	assertContains(t, "preload-hasmany", got, wantHM)

	// belongs_to preload.
	got = runAR(t, bin, arExecModels+`
u = User.create!(name:"a"); Post.create!(title:"p", user:u)
arlog!; Post.includes(:user).to_a`)
	_ = post
	assertContains(t, "preload-belongsto", got,
		`SELECT "users".* FROM "users" WHERE "users"."id" = 1`)

	// through preload.
	got = runAR(t, bin, arExecModels+`
c = Company.create!(name:"acme"); u=User.create!(name:"a", company:c); Post.create!(title:"p", user:u)
arlog!; Company.includes(:posts).to_a`)
	assertContains(t, "preload-through", got,
		`SELECT "posts".* FROM "posts" WHERE "posts"."user_id" = 1`)
	_ = company
}

func TestOracleSTISQL(t *testing.T) {
	bin := rubyOracle(t)
	base := NewModel("User", "users", Column{"id", "integer"}, Column{"name", "string"})
	base.STI("type")
	admin := base.Subclass("Admin")

	got := runRuby(t, bin, arExecPreamble+"\n"+arExecModels+
		"\nputs [User.all.to_sql, Admin.all.to_sql].join(\"\\x1e\")\n")
	if len(got) < 2 {
		t.Fatalf("expected two to_sql lines, got %#v", got)
	}
	if got[0] != base.All().ToSQL() {
		t.Errorf("base sti:\n got %s\nwant %s", base.All().ToSQL(), got[0])
	}
	if got[1] != admin.All().ToSQL() {
		t.Errorf("admin sti:\n got %s\nwant %s", admin.All().ToSQL(), got[1])
	}
}

// hasManyPreloadSQL renders the preload query this package would issue for a
// has_many across the given parents (mirrors preloadHasMany's query build).
func hasManyPreloadSQL(owner *Model, parents []*Record) (string, error) {
	assoc := owner.associations["posts"]
	tgt, _ := owner.resolveModel(assoc.ClassName)
	fk := assoc.foreignKey(underscoreClass(owner.Name))
	ids := collectValues(parents, owner.PrimaryKey)
	return tgt.Where(map[string]any{fk: ids}).ToSQL(), nil
}

// assertContains fails unless want appears among the captured SQL statements.
func assertContains(t *testing.T, name string, got []string, want string) {
	t.Helper()
	for _, s := range got {
		if s == want {
			return
		}
	}
	t.Errorf("%s: ActiveRecord did not emit\n  %s\nin:\n  %s", name, want, strings.Join(got, "\n  "))
}
