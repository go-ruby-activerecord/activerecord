# Ruby examples

Pure-Ruby usage of the `activerecord` gem — the Ruby face of this library —
running under [go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) (rbgo)
via its `require "active_record"` binding.

```sh
rbgo examples/activerecord_usage.rb
```

| File | Shows |
| --- | --- |
| [`activerecord_usage.rb`](activerecord_usage.rb) | Model DSL + presence validation, chainable relation `#to_sql` and `#insert_sql`, `ActiveModel::Errors`, and the live `Schema.define` → `Base` subclass → `create!` → `where.order.to_a` route on an in-memory SQLite connection. |
