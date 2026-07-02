// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Record is a single model instance's attribute set with dirty tracking: it
// holds the current values and the values loaded from the database, so
// Changed/Changes report modifications the way ActiveModel::Dirty does. Values
// are type-cast to the column type on write (Set).
type Record struct {
	model    *Model
	attrs    map[string]any
	original map[string]any
	// order preserves attribute insertion order for deterministic Changes keys.
	order []string
}

// Build returns a new Record with the given initial attributes (Model.new),
// type-cast per column. A newly-built record has every set attribute considered
// changed (no persisted original), matching ActiveRecord.
func (m *Model) Build(attrs map[string]any) *Record {
	r := &Record{model: m, attrs: map[string]any{}, original: map[string]any{}}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r.Set(k, attrs[k])
	}
	return r
}

// Load returns a Record whose attributes are treated as persisted (loaded from
// the database): no attribute is changed until a subsequent Set, matching a row
// materialized by ActiveRecord.
func (m *Model) Load(attrs map[string]any) *Record {
	r := m.Build(attrs)
	// Snapshot current as original: mark clean.
	r.original = map[string]any{}
	for k, v := range r.attrs {
		r.original[k] = v
	}
	return r
}

// Set writes an attribute, type-casting to the column type. Unknown attributes
// are stored verbatim (ActiveRecord raises; a host may pre-filter). Returns the
// receiver for chaining.
func (r *Record) Set(name string, v any) *Record {
	cast := v
	if col, ok := r.model.column(name); ok {
		cast = castTo(col.Type, v)
	}
	if _, seen := r.attrs[name]; !seen {
		r.order = append(r.order, name)
	}
	r.attrs[name] = cast
	return r
}

// get returns the current value (or nil).
func (r *Record) get(name string) any { return r.attrs[name] }

// Get returns the current attribute value and whether it is set.
func (r *Record) Get(name string) (any, bool) {
	v, ok := r.attrs[name]
	return v, ok
}

// Attributes returns a copy of the current attribute map.
func (r *Record) Attributes() map[string]any {
	out := make(map[string]any, len(r.attrs))
	for k, v := range r.attrs {
		out[k] = v
	}
	return out
}

// Changed reports whether any attribute differs from its persisted original
// (ActiveModel::Dirty#changed?).
func (r *Record) Changed() bool { return len(r.changedAttrs()) > 0 }

// AttributeChanged reports whether one attribute changed (name_changed?).
func (r *Record) AttributeChanged(name string) bool {
	cur, ok := r.attrs[name]
	orig, hadOrig := r.original[name]
	if !hadOrig {
		return ok
	}
	return !valueEqual2(cur, orig)
}

// changedAttrs returns changed attribute names in insertion order.
func (r *Record) changedAttrs() []string {
	var out []string
	for _, k := range r.order {
		if r.AttributeChanged(k) {
			out = append(out, k)
		}
	}
	return out
}

// ChangedAttributeNames returns the changed attribute names (name order).
func (r *Record) ChangedAttributeNames() []string {
	return append([]string(nil), r.changedAttrs()...)
}

// Change is one attribute's [old, new] pair (ActiveModel::Dirty#changes value).
type Change struct {
	Was any
	Now any
}

// Changes returns a map of changed attribute => {old, new}, matching
// ActiveModel::Dirty#changes.
func (r *Record) Changes() map[string]Change {
	out := map[string]Change{}
	for _, k := range r.changedAttrs() {
		out[k] = Change{Was: r.original[k], Now: r.attrs[k]}
	}
	return out
}

// SaveClean snapshots the current attributes as the persisted baseline (what
// ActiveRecord does after a successful save): subsequent Changes are relative to
// now.
func (r *Record) SaveClean() {
	r.original = map[string]any{}
	for k, v := range r.attrs {
		r.original[k] = v
	}
}

// Validate runs the model's validators against this record.
func (r *Record) Validate() *Errors { return r.model.Validate(r) }

// Valid reports whether the record passes all validations.
func (r *Record) Valid() bool { return r.Validate().Empty() }

// castTo type-casts a raw value to the ActiveRecord column type, mirroring the
// coercions ActiveModel::Type performs on assignment.
func castTo(typ string, v any) any {
	switch typ {
	case "integer", "bigint":
		return castInt(v)
	case "float":
		return castFloat(v)
	case "decimal":
		return castFloat(v)
	case "boolean":
		return castBool(v)
	case "string", "text":
		return castString(v)
	case "datetime", "timestamp", "date", "time":
		return castTime(v)
	default:
		return v
	}
}

func castInt(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case int:
		return int64(x)
	case int64:
		return x
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	case *big.Int:
		return x
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case string:
		s := strings.TrimSpace(x)
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(f)
		}
		return nil
	default:
		return nil
	}
}

func castFloat(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case *big.Int:
		f, _ := new(big.Float).SetInt(x).Float64()
		return f
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f
		}
		return nil
	default:
		return nil
	}
}

func castBool(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "", "0", "f", "false", "off", "no":
			return false
		default:
			return true
		}
	default:
		return true
	}
}

func castString(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return x
	case Symbol:
		return string(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return num2(v)
	}
}

func castTime(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return x
	case string:
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
			if t, err := time.Parse(layout, strings.TrimSpace(x)); err == nil {
				return t
			}
		}
		return nil
	default:
		return nil
	}
}

// valueEqual2 compares two cast attribute values for dirty tracking. It treats
// numerically-equal ints/floats and equal times as unchanged.
func valueEqual2(a, b any) bool {
	if ta, ok := a.(time.Time); ok {
		if tb, ok := b.(time.Time); ok {
			return ta.Equal(tb)
		}
		return false
	}
	return valueEqual(a, b)
}
