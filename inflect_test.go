// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import "testing"

func TestUnderscore(t *testing.T) {
	cases := map[string]string{
		"User": "user", "LineItem": "line_item", "Admin::User": "user",
		"HTMLParser": "html_parser", "APIKey": "api_key", "A": "a", "": "",
	}
	for in, want := range cases {
		if got := underscoreClass(in); got != want {
			t.Errorf("underscoreClass(%q) = %q want %q", in, got, want)
		}
	}
}

func TestPluralize(t *testing.T) {
	cases := map[string]string{
		// default +s
		"user": "users", "line_item": "line_items", "account": "accounts",
		// consonant + y -> ies (and vowel + y stays +s)
		"category": "categories", "day": "days",
		// s/x/z/ch/sh -> es
		"bus": "buses", "box": "boxes", "buzz": "buzzes",
		"branch": "branches", "dish": "dishes",
		// the explicit -oes set; every other -o word takes plain +s
		"hero": "heroes", "tomato": "tomatoes",
		"studio": "studios", "photo": "photos", "gizmo": "gizmos",
		// irregulars
		"person": "people", "man": "men", "child": "children",
		"mouse": "mice", "ox": "oxen", "zombie": "zombies",
		// uncountable
		"sheep": "sheep", "series": "series", "equipment": "equipment",
		// empty
		"": "",
	}
	for in, want := range cases {
		if got := Pluralize(in); got != want {
			t.Errorf("Pluralize(%q) = %q want %q", in, got, want)
		}
	}
}

func TestTableize(t *testing.T) {
	cases := map[string]string{
		"User": "users", "LineItem": "line_items", "Admin::Account": "accounts",
		"Category": "categories", "Person": "people", "": "",
	}
	for in, want := range cases {
		if got := Tableize(in); got != want {
			t.Errorf("Tableize(%q) = %q want %q", in, got, want)
		}
	}
}

func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"first_name": "First name", "company_id": "Company", "name": "Name",
		"": "", "_id": "", "email": "Email",
	}
	for in, want := range cases {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q want %q", in, got, want)
		}
	}
}

func TestHABTMJoinTable(t *testing.T) {
	if habtmJoinTable("roles", "users") != "roles_users" {
		t.Error("roles_users")
	}
	if habtmJoinTable("users", "roles") != "roles_users" {
		t.Error("sorted")
	}
}

func TestRuneCaseHelpers(t *testing.T) {
	if !isUpperRune('A') || isUpperRune('a') || !isLowerRune('z') || isLowerRune('Z') {
		t.Error("case predicates")
	}
	if toLowerRune('A') != 'a' || toLowerRune('_') != '_' {
		t.Error("toLower")
	}
	if toUpperRune('a') != 'A' || toUpperRune('1') != '1' {
		t.Error("toUpper")
	}
}
