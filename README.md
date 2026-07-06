<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-activerecord/brand/main/social/go-ruby-activerecord-activerecord.png" alt="go-ruby-activerecord/activerecord" width="720"></p>

# activerecord — go-ruby-activerecord

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-activerecord.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the deterministic core of Rails'
[ActiveRecord](https://guides.rubyonrails.org/active_record_basics.html) ORM** —
the query-building, schema-DDL, association, validation and attribute layers that
turn a model + relation description into SQL and run validations, exactly as
MRI's `activerecord` gem does. The one thing that genuinely needs a database —
executing the SQL — is an injected **Adapter** host seam (wired to
[go-ruby-sqlite3](https://github.com/go-ruby-sqlite3/sqlite3) /
[go-ruby-pg](https://github.com/go-ruby-pg/pg)), so this module stays 100%
Ruby- and CGO-free and produces SQL a differential oracle compares to
ActiveRecord's own `Relation#to_sql` **byte-for-byte**.

It is the ORM backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), a sibling of
[go-ruby-sequel](https://github.com/go-ruby-sequel/sequel) (whose dialect
approach it shares), and a **standalone, reusable** module.

> **What it is — and isn't.** Building the SQL string for a relation
> (`where`/`order`/`joins`/aggregates/…), the migration DDL, the association join
> geometry, and the validation logic + message text is fully deterministic and
> needs **no interpreter and no live database** — so it lives here as pure Go.
> Actually running the statements, connection pooling, transaction *execution*,
> the full callback chain and STI edge cases are the host's job; this library
> renders the SQL and exposes an [`Adapter`](adapter.go) the host implements.

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
seam too.

## Scope & what's deferred

This is the **deterministic core**. Documented host seams / deferred: statement
execution, connection pooling, transaction *execution*, the full `before/after`
callback chain execution (registration is a host concern; bodies run in `rbgo`),
STI edge cases, and eager-loading materialization (`includes` query *planning*
lives here; the multi-query fetch is the host's).

## The oracle

The test suite runs the real `activerecord` gem (version-gated
`RUBY_VERSION >= "4.0"`) against an in-memory SQLite connection and compares
`Relation#to_sql`, `errors.full_messages`, and the schema DDL to this package's
output **byte-for-byte**. The deterministic, Ruby-free golden vectors alone keep
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
