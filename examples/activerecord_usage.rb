# frozen_string_literal: true

require "active_record"

# Declare a model: columns + a presence validation, no database needed.
User = ActiveRecord::Model.new("User", "users") do
  column :id, :integer
  column :name, :string
  column :age, :integer
  validates_presence_of :name
end

# Chainable, lazy relations render byte-faithful ActiveRecord SQL.
puts User.where(age: 30).order(:name).limit(5).to_sql
puts User.insert_sql(name: "bob", age: 30)

# Validations produce an ActiveModel::Errors-shaped object.
rec = User.build(age: 30)               # no name
p rec.valid?                            # => false
p rec.errors.full_messages             # => ["Name can't be blank"]

# The live route: open a connection, author the schema, then create + query.
ActiveRecord::Base.establish_connection(adapter: "sqlite3", database: ":memory:")
ActiveRecord::Schema.define do
  create_table :people do |t|
    t.string  :name
    t.integer :age
  end
end

class Person < ActiveRecord::Base
end

[["amy", 30], ["bob", 25], ["cat", 40]].each { |n, a| Person.create!(name: n, age: a) }

people = Person.where("age >= ?", 26).order(:name).to_a
puts people.map { |p| "#{p.name} (#{p.age})" }.join(", ")   # => amy (30), cat (40)
p Person.count                                              # => 3
