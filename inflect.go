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

// pluralUncountable holds the words ActiveSupport treats as identical in the
// singular and plural (the default uncountable set); Pluralize returns them
// unchanged.
var pluralUncountable = map[string]bool{
	"equipment":   true,
	"information": true,
	"rice":        true,
	"money":       true,
	"species":     true,
	"series":      true,
	"fish":        true,
	"sheep":       true,
	"jeans":       true,
	"police":      true,
}

// pluralOES holds the "-o" nouns that pluralize to "-oes" (the classic English
// set ActiveSupport's default rules single out); every other "-o" word takes a
// plain "-s" ("studio" -> "studios", "photo" -> "photos").
var pluralOES = map[string]bool{
	"hero":     true,
	"potato":   true,
	"tomato":   true,
	"buffalo":  true,
	"echo":     true,
	"veto":     true,
	"volcano":  true,
	"mango":    true,
	"torpedo":  true,
	"mosquito": true,
}

// pluralIrregular holds the irregular singular=>plural pairs from
// ActiveSupport's default inflections (the ones a model name realistically
// hits); it is consulted before the suffix rules.
var pluralIrregular = map[string]string{
	"person": "people",
	"man":    "men",
	"child":  "children",
	"mouse":  "mice",
	"ox":     "oxen",
	"zombie": "zombies",
}

// Pluralize returns the plural form of a lowercase singular English word using
// ActiveSupport's default inflection rules (the subset a Rails model name
// exercises): the uncountable and irregular sets first, then the common suffix
// rules ("y"->"ies" after a consonant, "s"/"x"/"z"/"ch"/"sh"->"es",
// "o"->"oes" after a consonant), falling back to appending "s". Input is
// assumed already underscored/lowercased (Tableize's path); a trailing
// pluralization is a host concern in the query core, so this is the one place
// that owns it.
func Pluralize(word string) string {
	if word == "" {
		return ""
	}
	if pluralUncountable[word] {
		return word
	}
	if p, ok := pluralIrregular[word]; ok {
		return p
	}
	switch {
	case endsWithAny(word, "s", "x", "z", "ch", "sh"):
		return word + "es"
	case endsConsonantY(word):
		return word[:len(word)-1] + "ies"
	case pluralOES[word]:
		return word + "es"
	default:
		return word + "s"
	}
}

// Tableize returns the table name ActiveRecord infers for a class name: the
// demodulized name underscored and pluralized ("User" -> "users",
// "LineItem" -> "line_items", "Admin::Account" -> "accounts"). It is the
// inference ActiveRecord::Base does when a model does not set table_name.
func Tableize(className string) string {
	return Pluralize(underscoreClass(className))
}

// endsWithAny reports whether s ends with any of the given suffixes.
func endsWithAny(s string, suffixes ...string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

// endsConsonantY reports whether s ends in a consonant followed by "y" (the
// "...y" -> "...ies" rule; a vowel before "y" keeps the plain "s").
func endsConsonantY(s string) bool {
	if !strings.HasSuffix(s, "y") || len(s) < 2 {
		return false
	}
	return !isVowel(rune(s[len(s)-2]))
}

// isVowel reports whether r is an ASCII vowel.
func isVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
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
