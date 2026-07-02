// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "strings"

// TableDef accumulates a create_table definition, emitting the CREATE TABLE DDL
// ActiveRecord's schema statements produce for the dialect.
type TableDef struct {
	dialect Dialect
	name    string
	pk      string // primary-key column name; "" disables the implicit PK
	cols    []colDef
}

type colDef struct {
	name      string
	typ       string
	null      bool // true when NULL is allowed (default); NOT NULL emitted when false
	notNull   bool // explicit null:false
	hasDef    bool
	def       Value
	limit     int
	precision int
}

// CreateTable begins a create_table definition. By default an "id" integer
// primary key is added (ActiveRecord's default), matching create_table :name.
func CreateTable(dialect Dialect, name string) *TableDef {
	return &TableDef{dialect: dialect, name: name, pk: "id"}
}

// NoPrimaryKey disables the implicit primary key (create_table id:false).
func (t *TableDef) NoPrimaryKey() *TableDef { t.pk = ""; return t }

// ColOpt configures a column in a table definition.
type ColOpt func(*colDef)

// NotNull marks the column NOT NULL (null:false).
func NotNull() ColOpt { return func(c *colDef) { c.notNull = true } }

// Default sets a column default (default: v).
func Default(v Value) ColOpt { return func(c *colDef) { c.hasDef = true; c.def = v } }

// Limit sets a column limit (limit: n) — currently used for string sizing.
func Limit(n int) ColOpt { return func(c *colDef) { c.limit = n } }

// Column adds a column of the given type.
func (t *TableDef) Column(name, typ string, opts ...ColOpt) *TableDef {
	c := colDef{name: name, typ: typ, null: true}
	for _, o := range opts {
		o(&c)
	}
	t.cols = append(t.cols, c)
	return t
}

// Timestamps adds created_at/updated_at NOT NULL datetime columns
// (t.timestamps), matching ActiveRecord's precision-6 datetimes.
func (t *TableDef) Timestamps() *TableDef {
	t.Column("created_at", "datetime", NotNull())
	t.Column("updated_at", "datetime", NotNull())
	return t
}

// References adds a "<name>_id" bigint foreign-key column (t.references).
func (t *TableDef) References(name string, opts ...ColOpt) *TableDef {
	return t.Column(name+"_id", "bigint", opts...)
}

// ToSQL renders the CREATE TABLE statement.
func (t *TableDef) ToSQL() string {
	var parts []string
	if t.pk != "" {
		parts = append(parts, t.dialect.quoteColumnName(t.pk)+" "+t.dialect.primaryKeySQL())
	}
	for _, c := range t.cols {
		parts = append(parts, t.columnSQL(c))
	}
	return "CREATE TABLE " + t.dialect.quoteTableName(t.name) + " (" + strings.Join(parts, ", ") + ")"
}

// columnSQL renders one column clause.
func (t *TableDef) columnSQL(c colDef) string {
	s := t.dialect.quoteColumnName(c.name) + " " + t.dialect.typeToSQL(c.typ)
	if c.hasDef {
		s += " DEFAULT " + t.dialect.quote(c.def)
	}
	if c.notNull {
		s += " NOT NULL"
	}
	return s
}

// AddColumnSQL renders ALTER TABLE ... ADD "col" type (add_column).
func AddColumnSQL(dialect Dialect, table, name, typ string, opts ...ColOpt) string {
	c := colDef{name: name, typ: typ, null: true}
	for _, o := range opts {
		o(&c)
	}
	td := &TableDef{dialect: dialect}
	return "ALTER TABLE " + dialect.quoteTableName(table) + " ADD " + td.columnSQL(c)
}

// AddIndexSQL renders CREATE [UNIQUE] INDEX. The index name defaults to
// ActiveRecord's "index_<table>_on_<cols>" convention.
func AddIndexSQL(dialect Dialect, table string, cols []string, unique bool, name string) string {
	if name == "" {
		name = "index_" + table + "_on_" + strings.Join(cols, "_and_")
	}
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = dialect.quoteColumnName(c)
	}
	kw := "CREATE INDEX "
	if unique {
		kw = "CREATE UNIQUE INDEX "
	}
	return kw + dialect.quoteColumnName(name) + " ON " + dialect.quoteTableName(table) +
		" (" + strings.Join(q, ", ") + ")"
}

// AddForeignKeySQL renders ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY,
// matching add_foreign_key's default constraint name and column convention.
func AddForeignKeySQL(dialect Dialect, fromTable, toTable, column, primaryKey, name string) string {
	if column == "" {
		column = singular(toTable) + "_id"
	}
	if primaryKey == "" {
		primaryKey = "id"
	}
	if name == "" {
		name = "fk_rails_" + fromTable + "_" + column
	}
	return "ALTER TABLE " + dialect.quoteTableName(fromTable) +
		" ADD CONSTRAINT " + dialect.quoteColumnName(name) +
		" FOREIGN KEY (" + dialect.quoteColumnName(column) + ")" +
		" REFERENCES " + dialect.quoteTableName(toTable) +
		" (" + dialect.quoteColumnName(primaryKey) + ")"
}

// primaryKeySQL returns the dialect's primary-key column type.
func (d Dialect) primaryKeySQL() string {
	switch d {
	case Postgres:
		return "bigserial primary key"
	case MySQL:
		return "bigint auto_increment PRIMARY KEY"
	default:
		return "integer PRIMARY KEY AUTOINCREMENT NOT NULL"
	}
}

// typeToSQL maps an ActiveRecord type symbol to the dialect's column type,
// matching connection.type_to_sql for the common types.
func (d Dialect) typeToSQL(typ string) string {
	switch d {
	case Postgres:
		return postgresType(typ)
	case MySQL:
		return mysqlType(typ)
	default:
		return sqliteType(typ)
	}
}

func sqliteType(typ string) string {
	switch typ {
	case "string":
		return "varchar"
	case "text":
		return "text"
	case "integer":
		return "integer"
	case "bigint":
		return "bigint"
	case "float":
		return "float"
	case "decimal":
		return "decimal"
	case "datetime":
		return "datetime(6)"
	case "timestamp":
		return "datetime(6)"
	case "time":
		return "time(6)"
	case "date":
		return "date"
	case "binary":
		return "blob"
	case "boolean":
		return "boolean"
	default:
		return typ
	}
}

func postgresType(typ string) string {
	switch typ {
	case "string":
		return "character varying"
	case "text":
		return "text"
	case "integer":
		return "integer"
	case "bigint":
		return "bigint"
	case "float":
		return "float"
	case "decimal":
		return "decimal"
	case "datetime":
		return "timestamp(6)"
	case "timestamp":
		return "timestamp(6)"
	case "time":
		return "time(6)"
	case "date":
		return "date"
	case "binary":
		return "bytea"
	case "boolean":
		return "boolean"
	default:
		return typ
	}
}

func mysqlType(typ string) string {
	switch typ {
	case "string":
		return "varchar(255)"
	case "text":
		return "text"
	case "integer":
		return "int"
	case "bigint":
		return "bigint"
	case "float":
		return "float"
	case "decimal":
		return "decimal(10,0)"
	case "datetime":
		return "datetime(6)"
	case "timestamp":
		return "timestamp(6)"
	case "time":
		return "time(6)"
	case "date":
		return "date"
	case "binary":
		return "blob"
	case "boolean":
		return "tinyint(1)"
	default:
		return typ
	}
}

// singular strips a trailing "s" from a table name for the default foreign-key
// stem in AddForeignKeySQL (a deliberately simple inflection; hosts pass the
// column explicitly for irregulars).
func singular(table string) string {
	if strings.HasSuffix(table, "s") {
		return table[:len(table)-1]
	}
	return table
}
