// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// mustRe compiles a regexp for the oracle validation test.
func mustRe(pat string) *regexp.Regexp { return regexp.MustCompile(pat) }

// The oracle tests run the real ActiveRecord gem and compare its
// Relation#to_sql / errors.full_messages to this package's output byte-for-byte.
// They skip themselves when ruby, the activerecord gem, or a Ruby older than
// 4.0 is present (the qemu cross-arch and Windows CI lanes), so the
// deterministic suite alone drives the 100% coverage gate there.

// rubyOracle locates a ruby whose RUBY_VERSION >= "4.0" and that can load
// active_record with a sqlite3 connection, returning its path or skipping.
func rubyOracle(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping ActiveRecord oracle")
	}
	// Version gate + gem availability probe.
	out, err := exec.Command(bin, "-e", `
    exit 2 if RUBY_VERSION < "4.0"
    begin
      require "active_record"
    rescue LoadError
      exit 3
    end
    print "ok"
  `).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Skipf("ruby>=4.0 with activerecord unavailable (%s); skipping oracle", strings.TrimSpace(string(out)))
	}
	return bin
}

// arSchema is the shared Ruby preamble: an in-memory sqlite schema + models
// mirroring testModels, so the oracle builds the same relations we do.
const arSchema = `
$stdout.sync = true
require "active_record"
ActiveRecord::Base.establish_connection(adapter: "sqlite3", database: ":memory:")
ActiveRecord::Schema.verbose = false
ActiveRecord::Schema.define do
  create_table(:users){|t| t.string :name; t.integer :age; t.string :email; t.boolean :active; t.references :company }
  create_table(:companies){|t| t.string :name }
  create_table(:posts){|t| t.string :title; t.references :user }
  create_table(:roles){|t| t.string :name }
  create_table(:roles_users, id:false){|t| t.references :user; t.references :role }
end
class Company < ActiveRecord::Base; has_many :users; has_many :posts, through: :users; end
class Post < ActiveRecord::Base; belongs_to :user; end
class Role < ActiveRecord::Base; has_and_belongs_to_many :users; end
class User < ActiveRecord::Base
  belongs_to :company; has_many :posts; has_and_belongs_to_many :roles
end
`

// rubyToSQL runs a Ruby expression that returns a Relation and prints its
// to_sql.
func rubyToSQL(t *testing.T, bin, expr string) string {
	t.Helper()
	script := arSchema + "\nputs (" + expr + ").to_sql\n"
	out, err := exec.Command(bin, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error for %q: %v\n%s", expr, err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

func TestOracleToSQL(t *testing.T) {
	bin := rubyOracle(t)
	u, _, c, _ := testModels(SQLite)

	cases := []struct {
		name string
		expr string
		got  string
	}{
		{"where_hash", `User.where(name: "bob")`, u.Where(map[string]any{"name": "bob"}).ToSQL()},
		{"where_str", `User.where("age > ?", 18)`, u.Where("age > ?", 18).ToSQL()},
		{"where_range", `User.where(age: 18..30)`, u.Where(map[string]any{"age": &Range{Begin: 18, End: 30}}).ToSQL()},
		{"where_range_excl", `User.where(age: 18...30)`, u.Where(map[string]any{"age": &Range{Begin: 18, End: 30, Exclusive: true}}).ToSQL()},
		{"where_array", `User.where(age: [1,2,3])`, u.Where(map[string]any{"age": []any{1, 2, 3}}).ToSQL()},
		{"where_nil", `User.where(name: nil)`, u.Where(map[string]any{"name": nil}).ToSQL()},
		{"not", `User.where.not(name: "bob")`, u.All().Not(map[string]any{"name": "bob"}).ToSQL()},
		{"or", `User.where(name: "bob").or(User.where(age: 30))`, u.Where(map[string]any{"name": "bob"}).Or(u.Where(map[string]any{"age": 30})).ToSQL()},
		{"order", `User.order(:age)`, u.Order("age").ToSQL()},
		{"order_desc", `User.order(age: :desc)`, u.Order(map[string]any{"age": "desc"}).ToSQL()},
		{"limit_offset", `User.limit(10).offset(5)`, u.All().Limit(10).Offset(5).ToSQL()},
		{"select", `User.select(:id, :name)`, u.Select("id", "name").ToSQL()},
		{"distinct", `User.distinct`, u.All().Distinct().ToSQL()},
		{"group_having", `User.group(:age).having("count(*) > ?", 1)`, u.All().Group("age").Having("count(*) > ?", 1).ToSQL()},
		{"joins_bt", `User.joins(:company)`, u.Joins("company").ToSQL()},
		{"joins_hm", `User.joins(:posts)`, u.Joins("posts").ToSQL()},
		{"left_joins", `User.left_joins(:posts)`, u.All().LeftJoins("posts").ToSQL()},
		{"joins_habtm", `User.joins(:roles)`, u.Joins("roles").ToSQL()},
		{"through", `Company.joins(:posts)`, c.Joins("posts").ToSQL()},
		{"find_by", `User.where(id: 1).limit(1)`, u.All().FindBy(map[string]any{"id": 1}).ToSQL()},
		{"chain", `User.where(active: true).order(:name).limit(3)`, u.Where(map[string]any{"active": true}).Order("name").Limit(3).ToSQL()},
	}
	for _, tc := range cases {
		want := rubyToSQL(t, bin, tc.expr)
		if tc.got != want {
			t.Errorf("%s:\n got %s\nwant %s (ActiveRecord)", tc.name, tc.got, want)
		}
	}
}

func TestOracleAggregates(t *testing.T) {
	bin := rubyOracle(t)
	u, _, _, _ := testModels(SQLite)
	// ActiveRecord's calculate emits the aggregate SELECT; compare via arel.
	cases := []struct {
		name string
		expr string
		got  string
	}{
		{"count", `User.all.select("COUNT(*)")`, u.All().CountSQL()},
		{"sum", `User.all.select(User.arel_table[:age].sum)`, u.All().SumSQL("age")},
		{"avg", `User.all.select(User.arel_table[:age].average)`, u.All().AverageSQL("age")},
		{"min", `User.all.select(User.arel_table[:age].minimum)`, u.All().MinimumSQL("age")},
		{"max", `User.all.select(User.arel_table[:age].maximum)`, u.All().MaximumSQL("age")},
	}
	for _, tc := range cases {
		want := rubyToSQL(t, bin, tc.expr)
		if tc.got != want {
			t.Errorf("%s:\n got %s\nwant %s", tc.name, tc.got, want)
		}
	}
}

func TestOracleValidations(t *testing.T) {
	bin := rubyOracle(t)
	script := `
$stdout.sync = true
require "active_record"
ActiveRecord::Base.establish_connection(adapter: "sqlite3", database: ":memory:")
ActiveRecord::Schema.verbose = false
ActiveRecord::Schema.define do
  create_table(:widgets){|t| t.string :name; t.integer :qty; t.string :email; t.string :code; t.string :size }
end
class Widget < ActiveRecord::Base
  validates :name, presence: true
  validates :qty, numericality: { greater_than: 0 }
  validates :email, format: { with: /\A[^@]+@[^@]+\z/ }
  validates :code, length: { minimum: 3, maximum: 5 }
  validates :size, inclusion: { in: %w[S M L] }
end
w = Widget.new; w.valid?
puts w.errors.full_messages.join("\x1f")
`
	out, err := exec.Command(bin, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\n%s", err, out)
	}
	want := strings.TrimRight(string(out), "\n")

	iptr := func(n int) *int { return &n }
	fptr := func(f float64) *float64 { return &f }
	w := NewModel("Widget", "widgets",
		Column{"name", "string"}, Column{"qty", "integer"}, Column{"email", "string"},
		Column{"code", "string"}, Column{"size", "string"})
	w.ValidatesPresence("name").
		ValidatesNumericality("qty", NumericalityOpts{GreaterThan: fptr(0)}).
		ValidatesFormat("email", mustRe(`\A[^@]+@[^@]+\z`)).
		ValidatesLength("code", LengthOpts{Minimum: iptr(3), Maximum: iptr(5)}).
		ValidatesInclusion("size", []any{"S", "M", "L"})
	got := strings.Join(w.Validate(w.Build(map[string]any{})).FullMessages(), "\x1f")
	if got != want {
		t.Errorf("validations:\n got %q\nwant %q", got, want)
	}
}

func TestOracleDDL(t *testing.T) {
	bin := rubyOracle(t)
	script := `
$stdout.sync = true
require "active_record"
ActiveRecord::Base.establish_connection(adapter: "sqlite3", database: ":memory:")
conn = ActiveRecord::Base.connection
td = conn.send(:create_table_definition, "users")
td.primary_key "id"
td.string :name, null: false
td.integer :age, default: 0
td.timestamps
puts conn.schema_creation.accept(td)
`
	out, err := exec.Command(bin, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\n%s", err, out)
	}
	want := strings.TrimRight(string(out), "\n")
	got := CreateTable(SQLite, "users").
		Column("name", "string", NotNull()).
		Column("age", "integer", Default(0)).
		Timestamps().ToSQL()
	if got != want {
		t.Errorf("create_table:\n got %s\nwant %s", got, want)
	}
}
