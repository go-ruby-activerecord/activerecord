<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-activerecord/brand/main/social/go-ruby-activerecord-activerecord.png" alt="go-ruby-activerecord/activerecord" width="720"></p>

# activerecord — go-ruby-activerecord

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-activerecord.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Rails'
[ActiveRecord](https://guides.rubyonrails.org/active_record_basics.html) ORM** —
the query-building, schema-DDL, association, validation, attribute, persistence,
transaction, callback, eager-loading and single-table-inheritance layers that
turn a model + relation description into SQL, run it, and materialize records,
exactly as MRI's `activerecord` gem does. The one thing that genuinely needs a
database — talking to the wire — is an injected **Adapter** host seam (wired to
[go-ruby-sqlite3](https://github.com/go-ruby-sqlite3/sqlite3) /
[go-ruby-pg](https://github.com/go-ruby-pg/pg)), so this module stays 100%
Ruby- and CGO-free and produces SQL a differential oracle compares to
ActiveRecord's own output **byte-for-byte**.

It is the ORM backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), a sibling of
[go-ruby-sequel](https://github.com/go-ruby-sequel/sequel) (whose dialect
approach it shares), and a **standalone, reusable** module.

> **The database seam.** Every SQL string this library produces — relation
> `to_sql`, the migration DDL, the association join geometry, the INSERT/UPDATE/
> DELETE for a save, the transaction and savepoint control, the eager-load
> queries and the prepared-statement templates — is rendered deterministically
> and byte-faithfully to ActiveRecord. The bytes are run through an
> [`Adapter`](adapter.go) the host implements over its driver; connection pooling
> and the actual socket I/O are the driver's job. Everything else ActiveRecord
> does around those bytes — validations, the full callback chain, transactions
> with nested savepoints, statement caching, eager loading and STI instantiation
> — lives here.

## Features

Faithful port of ActiveRecord's SQL generation + validations, validated against
the real `activerecord` gem on every supported platform.

- **Relation** — lazy, immutable, chainable: `Where` (hash / `?`- and
  `:name`-placeholder strings / IN / BETWEEN / IS NULL), `Not`, `Or`, `Select`,
  `Order` (asc/desc, map, raw), `Limit`/`Offset`, `Group`/`Having`,
  `Joins`/`LeftJoins`, `Distinct`, `From`, `Merge`, named `Scope`s, and
  `First`/`Last`/`Take`/`Find`/`FindBy` — rendered by `ToSQL()`.
- **Aggregates & DML** — `CountSQL`/`SumSQL`/`AverageSQL`/`MinimumSQL`/
  `MaximumSQL` (grouped or not), `PluckSQL`, `ExistsSQL`, `InsertSQL`,
  `UpdateAllSQL`, `DeleteAllSQL`.
- **Associations** — `belongs_to` / `has_many` / `has_one` /
  `has_and_belongs_to_many` and `:through`, emitting ActiveRecord's exact
  `INNER`/`LEFT OUTER JOIN … ON` geometry (including HABTM join tables and the
  two-hop through join with source-reflection direction).
- **Schema** — `CreateTable` (+`References`/`Timestamps`/`NoPrimaryKey`),
  `AddColumnSQL`, `AddIndexSQL` (unique, default `index_<t>_on_<cols>` name),
  `AddForeignKeySQL`, and a per-dialect column-type map.
- **Validations** — `presence` / `length` (min/max/is) / `format` /
  `numericality` (all comparators + only_integer/odd/even) / `inclusion` /
  `exclusion` / `uniqueness` (a host-seam query) — producing an
  [`Errors`](validations.go) shaped like `ActiveModel::Errors` with
  ActiveRecord's default message text and `full_messages` order.
- **Attributes** — readers/writers, per-column type casting, and dirty tracking
  (`Changed`/`Changes`/`AttributeChanged`) like `ActiveModel::Dirty`.
- **Persistence** — `Save`/`Create`/`Update`/`Destroy`/`Delete` execute the
  column-ordered INSERT/UPDATE/DELETE (with the `RETURNING` clause and the
  `created_at`/`updated_at` timestamping ActiveRecord applies), assign the
  generated primary key, and reset dirty state — all through the `Adapter`.
- **Callbacks** — the full `before/after` lifecycle for validation, save, create,
  update, destroy and the transactional `after_commit`/`after_rollback`, fired in
  ActiveSupport's order (before forward, after LIFO) with `throw :abort`
  (`ErrAbort`) halting semantics.
- **Transactions** — real `BEGIN`/`COMMIT`/`ROLLBACK` with nested `SAVEPOINT
  active_record_N`/`RELEASE`/`ROLLBACK TO`, an `ErrRollback` sentinel, and
  panic-safe unwinding; a save inside a `Transaction` automatically runs on a
  savepoint.
- **Statement cache** — a `StatementCache` of prepared `find`/`find_by`
  statements (`… = ? LIMIT ?`, `$n` on postgres) with a `PreparedAdapter` seam
  for true driver-level prepares and a transparent inline fallback.
- **Eager loading** — `Includes`/`Preload` run ActiveRecord's N+1-avoiding
  multi-query fetch for `belongs_to`/`has_many`/`has_one`/HABTM/`:through` and
  attach the targets to each record.
- **Query interface** — executable `ToArray`/`Pluck`/`Ids`/`Exists`/`Count`/
  `First`/`Last`/`Take`/`FindRecord`/`FindByRecord`/`FindEach`, plus named
  `Scope`s, `DefaultScope` and `Unscoped`.
- **Single-table inheritance** — `STI`/`Subclass` add ActiveRecord's exact type
  condition (`type = 'Admin'`, or `IN (…)` over descendants) and instantiate rows
  as the subclass named by the discriminator column.
- **Migrations** — a `Migrator` runs migrations through the adapter with a
  `schema_migrations` version ledger (idempotent up/rollback in a transaction),
  plus `DropTable`/`RemoveColumn`/`RenameColumn`/`ChangeColumnNull`/`RemoveIndex`/
  `AddTimestamps` DDL.
- **Three dialects** — SQLite, PostgreSQL and MySQL identifier quoting and value
  literalization, selectable per model.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) and three OSes.

## Install

```sh
go get github.com/go-ruby-activerecord/activerecord
```

## Usage

```go
package main

import (
	"fmt"

	ar "github.com/go-ruby-activerecord/activerecord"
)

func main() {
	users := ar.NewModel("User", "users",
		ar.Column{Name: "id", Type: "integer"},
		ar.Column{Name: "name", Type: "string"},
		ar.Column{Name: "age", Type: "integer"},
		ar.Column{Name: "company_id", Type: "bigint"})
	posts := ar.NewModel("Post", "posts",
		ar.Column{Name: "id", Type: "integer"},
		ar.Column{Name: "user_id", Type: "bigint"})
	users.Register(posts).HasMany("posts", "Post")
	posts.Register(users).BelongsTo("user", "User")

	rel := users.
		Where(map[string]any{"age": &ar.Range{Begin: 18, End: 30}}).
		Joins("posts").
		Order(map[string]any{"name": "asc"}).
		Limit(10)

	fmt.Println(rel.ToSQL())
	// SELECT "users".* FROM "users"
	//   INNER JOIN "posts" ON "posts"."user_id" = "users"."id"
	//   WHERE "users"."age" BETWEEN 18 AND 30
	//   ORDER BY "users"."name" ASC LIMIT 10
}
```

### Validations

```go
w := ar.NewModel("Widget", "widgets", ar.Column{Name: "name", Type: "string"})
w.ValidatesPresence("name")

rec := w.Build(map[string]any{}) // no name
errs := rec.Validate()
fmt.Println(errs.FullMessages()) // ["Name can't be blank"]
```

### The adapter (host) seam

Execution is injected. A host implements [`Adapter`](adapter.go) over its driver
(go-ruby-sqlite3 / go-ruby-pg) and the package's `Exists` / `Count` / `LoadAll`
helpers run the rendered SQL through it:

```go
type Adapter interface {
	Execute(sql string) ([]Row, error)
	ExecuteDML(sql string) (affected, lastInsertID int64, err error)
	AdapterName() string // "sqlite3" | "postgresql" | "mysql2"
}
```

`ValidatesUniqueness(attr, exists)` takes a callback the host wires to
`Exists(adapter, relation)` so the one query-dependent validator stays behind the
seam too. A host that offers driver-level prepared statements additionally
implements [`PreparedAdapter`](statement_cache.go) so the statement cache binds
values out-of-band; otherwise the binds are transparently inlined.

## Persistence, transactions & the lifecycle

Saving, transactions and eager loading run through the same `Adapter`:

```go
u, _ := users.Create(adapter, map[string]any{"name": "bob", "age": 30})
// BEGIN immediate TRANSACTION
// INSERT INTO "users" ("name","age","created_at","updated_at")
//   VALUES ('bob',30,'…','…') RETURNING "id"    → u.Get("id")
// COMMIT TRANSACTION

ar.Transaction(adapter, func() error {          // nests as SAVEPOINT active_record_1
	u.Update(adapter, map[string]any{"age": 31})
	return ar.ErrRollback                        // ROLLBACK TO SAVEPOINT …
})

posts, _ := ar.LoadIncludes(adapter, users.All().Includes("posts"))
// SELECT "users".* FROM "users"
// SELECT "posts".* FROM "posts" WHERE "posts"."user_id" IN (…)
```

Single-table inheritance (`base.STI("type")`, `admin := base.Subclass("Admin")`)
adds `WHERE "users"."type" = 'Admin'` to the subclass's queries and instantiates
rows as the subclass. Migrations run through a `Migrator` with an idempotent
`schema_migrations` ledger.

## The oracle

The test suite runs the real `activerecord` gem (version-gated
`RUBY_VERSION >= "4.0"`) against an in-memory SQLite connection and compares this
package's output **byte-for-byte** to ActiveRecord's — not just `Relation#to_sql`,
`errors.full_messages` and the schema DDL, but also the INSERT/UPDATE/DELETE a
save issues, the `BEGIN`/`SAVEPOINT`/`COMMIT` control sequence, the prepared
`find`/`find_by` templates, the eager-load queries and the STI type condition
(captured from `ActiveSupport::Notifications` with prepared statements disabled so
bind values inline). The deterministic, Ruby-free golden vectors alone keep
coverage at 100%, so the cross-arch (qemu) and Windows CI lanes — where no MRI is
present — still pass the gate.

## Tests & coverage

```sh
GOWORK=off go test -race -coverprofile=cover.out ./...
GOWORK=off go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-activerecord/activerecord authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
