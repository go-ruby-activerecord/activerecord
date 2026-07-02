// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"sort"
	"strings"
)

// underscoreClass converts a Ruby class name to its snake_case foreign-key stem
// the way ActiveScord's ActiveSupport#underscore does for a simple (namespaced
// or bare) class name: "User" -> "user", "LineItem" -> "line_item",
// "Admin::User" -> "user" (the demodulized stem, since foreign keys use it).
func underscoreClass(name string) string {
	if i := strings.LastIndex(name, "::"); i >= 0 {
		name = name[i+2:]
	}
	return underscore(name)
}

// underscore is ActiveSupport#underscore for a demodulized identifier: insert
// "_" between a lower/digit and an upper, and between consecutive-upper then
// lower boundaries, then downcase.
func underscore(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		if isUpperRune(r) {
			if i > 0 {
				prev := rs[i-1]
				next := rune(0)
				if i+1 < len(rs) {
					next = rs[i+1]
				}
				if !isUpperRune(prev) || (next != 0 && isLowerRune(next)) {
					b.WriteByte('_')
				}
			}
			b.WriteRune(toLowerRune(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isUpperRune(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLowerRune(r rune) bool { return r >= 'a' && r <= 'z' }
func toLowerRune(r rune) rune {
	if isUpperRune(r) {
		return r + ('a' - 'A')
	}
	return r
}

// habtmJoinTable computes ActiveRecord's default HABTM join-table name: the two
// table names sorted lexicographically and joined by "_" (e.g. "roles" +
// "users" -> "roles_users").
func habtmJoinTable(a, b string) string {
	names := []string{a, b}
	sort.Strings(names)
	return names[0] + "_" + names[1]
}

// humanize converts an attribute name to ActiveRecord's default humanized
// label used in validation full messages: strip a trailing "_id", replace "_"
// with " " and upcase the first letter (e.g. "first_name" -> "First name").
func humanize(name string) string {
	s := name
	if strings.HasSuffix(s, "_id") {
		s = s[:len(s)-3]
	}
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = toUpperRune(r[0])
	return string(r)
}

func toUpperRune(r rune) rune {
	if isLowerRune(r) {
		return r - ('a' - 'A')
	}
	return r
}
