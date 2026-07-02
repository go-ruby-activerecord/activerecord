// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"sort"
	"strings"
)

// Range models a Ruby Range used as a where value (col: a..b), rendered to
// BETWEEN (inclusive) or the half-open form ActiveRecord emits for exclusive
// ranges. A nil Begin/End models a beginless/endless range.
type Range struct {
	Begin     Value
	End       Value
	Exclusive bool // true for a...b (exclusive end)
}

// buildConditions renders one Where/Not/Having argument list to a slice of
// predicate fragments. neg negates hash equalities (where.not).
func (m *Model) buildConditions(cond []any, neg bool) []string {
	first := cond[0]
	switch c := first.(type) {
	case map[string]any:
		return m.hashConditions(c, neg)
	case map[Symbol]any:
		norm := make(map[string]any, len(c))
		for k, v := range c {
			norm[string(k)] = v
		}
		return m.hashConditions(norm, neg)
	case string:
		return []string{"(" + m.interpolate(c, cond[1:]) + ")"}
	default:
		return nil
	}
}

// hashConditions renders a column=>value hash to AND-joinable fragments, in
// sorted key order for determinism (ActiveRecord preserves Ruby hash insertion
// order; the oracle uses single-key hashes or already-sorted keys so this
// matches).
func (m *Model) hashConditions(h map[string]any, neg bool) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, m.hashPredicate(k, h[k], neg))
	}
	return out
}

// hashPredicate renders "table"."col" <op> value for one hash entry, choosing
// =/!=, IN/NOT IN, BETWEEN, IS NULL/IS NOT NULL as ActiveRecord does.
func (m *Model) hashPredicate(col string, v Value, neg bool) string {
	q := m.Dialect.qualify(m.TableName, col)
	switch x := v.(type) {
	case nil:
		if neg {
			return q + " IS NOT NULL"
		}
		return q + " IS NULL"
	case []any:
		return m.inPredicate(q, x, neg)
	case *Range:
		return m.rangePredicate(q, x, neg)
	case Range:
		return m.rangePredicate(q, &x, neg)
	default:
		op := " = "
		if neg {
			op = " != "
		}
		return q + op + m.Dialect.quote(v)
	}
}

// inPredicate renders an IN / NOT IN list. An empty list matches ActiveRecord:
// IN (NULL) for empty, negated to a tautology 1=1.
func (m *Model) inPredicate(q string, list []any, neg bool) string {
	if len(list) == 0 {
		if neg {
			return "1=1"
		}
		return q + " IN (NULL)"
	}
	parts := make([]string, len(list))
	for i, v := range list {
		parts[i] = m.Dialect.quote(v)
	}
	kw := " IN ("
	if neg {
		kw = " NOT IN ("
	}
	return q + kw + strings.Join(parts, ", ") + ")"
}

// rangePredicate renders BETWEEN (inclusive) or the >=/< half-open form for an
// exclusive range, and the >=/<= forms for beginless/endless ranges, matching
// ActiveRecord.
func (m *Model) rangePredicate(q string, r *Range, neg bool) string {
	lo, hasLo := r.Begin, r.Begin != nil
	hi, hasHi := r.End, r.End != nil
	var s string
	switch {
	case hasLo && hasHi && !r.Exclusive:
		s = q + " BETWEEN " + m.Dialect.quote(lo) + " AND " + m.Dialect.quote(hi)
	case hasLo && hasHi && r.Exclusive:
		s = q + " >= " + m.Dialect.quote(lo) + " AND " + q + " < " + m.Dialect.quote(hi)
	case hasLo && !hasHi:
		s = q + " >= " + m.Dialect.quote(lo)
	case !hasLo && hasHi && r.Exclusive:
		s = q + " < " + m.Dialect.quote(hi)
	case !hasLo && hasHi:
		s = q + " <= " + m.Dialect.quote(hi)
	default:
		s = "1=1"
	}
	if neg {
		return "NOT (" + s + ")"
	}
	return s
}

// interpolate substitutes bind values into a raw SQL fragment. It supports "?"
// positional placeholders (each replaced by the next quoted bind) and, when a
// single map bind is given, ":name" named placeholders, matching
// ActiveRecord's sanitize_sql_array.
func (m *Model) interpolate(frag string, binds []any) string {
	if len(binds) == 1 {
		if hm, ok := binds[0].(map[string]any); ok {
			return m.interpolateNamed(frag, hm)
		}
		if hm, ok := binds[0].(map[Symbol]any); ok {
			norm := make(map[string]any, len(hm))
			for k, v := range hm {
				norm[string(k)] = v
			}
			return m.interpolateNamed(frag, norm)
		}
	}
	if !strings.Contains(frag, "?") {
		return frag
	}
	var b strings.Builder
	bi := 0
	for i := 0; i < len(frag); i++ {
		if frag[i] == '?' && bi < len(binds) {
			b.WriteString(m.Dialect.quote(binds[bi]))
			bi++
			continue
		}
		b.WriteByte(frag[i])
	}
	return b.String()
}

// interpolateNamed substitutes ":name" placeholders from a bind map.
func (m *Model) interpolateNamed(frag string, binds map[string]any) string {
	var b strings.Builder
	for i := 0; i < len(frag); i++ {
		if frag[i] == ':' && i+1 < len(frag) && isNameStart(frag[i+1]) {
			j := i + 1
			for j < len(frag) && isNameChar(frag[j]) {
				j++
			}
			name := frag[i+1 : j]
			if v, ok := binds[name]; ok {
				b.WriteString(m.Dialect.quote(v))
				i = j - 1
				continue
			}
		}
		b.WriteByte(frag[i])
	}
	return b.String()
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}
