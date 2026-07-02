// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func iptr(n int) *int         { return &n }
func fptr(f float64) *float64 { return &f }

func full(e *Errors) string { return strings.Join(e.FullMessages(), " | ") }

func TestValidationMessages(t *testing.T) {
	w := NewModel("Widget", "widgets",
		Column{"name", "string"}, Column{"qty", "integer"}, Column{"email", "string"},
		Column{"code", "string"}, Column{"size", "string"})
	w.ValidatesPresence("name").
		ValidatesNumericality("qty", NumericalityOpts{GreaterThan: fptr(0)}).
		ValidatesFormat("email", regexp.MustCompile(`\A[^@]+@[^@]+\z`)).
		ValidatesLength("code", LengthOpts{Minimum: iptr(3), Maximum: iptr(5)}).
		ValidatesInclusion("size", []any{"S", "M", "L"})

	if got := full(w.Validate(w.Build(map[string]any{}))); got !=
		"Name can't be blank | Qty is not a number | Email is invalid | Code is too short (minimum is 3 characters) | Size is not included in the list" {
		t.Errorf("empty = %q", got)
	}
	if got := full(w.Validate(w.Build(map[string]any{"name": "x", "qty": -1, "email": "bad", "code": "ab", "size": "XL"}))); got !=
		"Qty must be greater than 0 | Email is invalid | Code is too short (minimum is 3 characters) | Size is not included in the list" {
		t.Errorf("bad = %q", got)
	}
	if got := full(w.Validate(w.Build(map[string]any{"name": "x", "qty": 5, "email": "a@b", "code": "abcdef", "size": "M"}))); got !=
		"Code is too long (maximum is 5 characters)" {
		t.Errorf("long = %q", got)
	}
	// fully valid.
	if e := w.Validate(w.Build(map[string]any{"name": "x", "qty": 5, "email": "a@b", "code": "abc", "size": "M"})); !e.Empty() {
		t.Errorf("valid but errors: %v", e.FullMessages())
	}
}

func TestLengthIs(t *testing.T) {
	m := NewModel("M", "ms", Column{"code", "string"})
	m.ValidatesLength("code", LengthOpts{Is: iptr(3)})
	if got := full(m.Validate(m.Build(map[string]any{"code": "ab"}))); got != "Code is the wrong length (should be 3 characters)" {
		t.Errorf("is = %q", got)
	}
	if got := full(m.Validate(m.Build(map[string]any{"code": "ab"}))); got == "" {
		t.Error("should fail")
	}
	if e := m.Validate(m.Build(map[string]any{"code": "abc"})); !e.Empty() {
		t.Error("exact len ok")
	}
	// single-character messages.
	m2 := NewModel("M", "ms", Column{"code", "string"})
	m2.ValidatesLength("code", LengthOpts{Minimum: iptr(1)})
	if got := full(m2.Validate(m2.Build(map[string]any{"code": ""}))); got != "Code is too short (minimum is 1 character)" {
		t.Errorf("min1 = %q", got)
	}
	// nil length branch.
	m3 := NewModel("M", "ms", Column{"code", "string"})
	m3.ValidatesLength("code", LengthOpts{Maximum: iptr(2)})
	if e := m3.Validate(m3.Build(map[string]any{})); !e.Empty() {
		t.Errorf("nil length under max ok, got %v", e.FullMessages())
	}
}

func TestNumericalityVariants(t *testing.T) {
	m := NewModel("M", "ms", Column{"n", "integer"})
	only := NewModel("M", "ms", Column{"n", "string"})
	only.ValidatesNumericality("n", NumericalityOpts{OnlyInteger: true})
	if got := full(only.Validate(only.Build(map[string]any{"n": "abc"}))); got != "N is not a number" {
		t.Errorf("nan = %q", got)
	}
	if got := full(only.Validate(only.Build(map[string]any{"n": "1.5"}))); got != "N must be an integer" {
		t.Errorf("nonint = %q", got)
	}
	if e := only.Validate(only.Build(map[string]any{"n": "3"})); !e.Empty() {
		t.Error("int string ok")
	}
	_ = m
	// all comparison operators.
	full2 := func(o NumericalityOpts, v any) string {
		mm := NewModel("M", "ms", Column{"n", "float"})
		mm.ValidatesNumericality("n", o)
		return full(mm.Validate(mm.Build(map[string]any{"n": v})))
	}
	if got := full2(NumericalityOpts{GreaterThanOrEq: fptr(5)}, 4); got != "N must be greater than or equal to 5" {
		t.Errorf("gte = %q", got)
	}
	if got := full2(NumericalityOpts{LessThan: fptr(5)}, 6); got != "N must be less than 5" {
		t.Errorf("lt = %q", got)
	}
	if got := full2(NumericalityOpts{LessThanOrEq: fptr(5)}, 6); got != "N must be less than or equal to 5" {
		t.Errorf("lte = %q", got)
	}
	if got := full2(NumericalityOpts{EqualTo: fptr(5)}, 6); got != "N must be equal to 5" {
		t.Errorf("eq = %q", got)
	}
	if got := full2(NumericalityOpts{Other: fptr(5)}, 5); got != "N must be other than 5" {
		t.Errorf("other = %q", got)
	}
	if got := full2(NumericalityOpts{Odd: true}, 4); got != "N must be odd" {
		t.Errorf("odd = %q", got)
	}
	if got := full2(NumericalityOpts{Even: true}, 3); got != "N must be even" {
		t.Errorf("even = %q", got)
	}
	// float non-integer message with fractional bound.
	if got := full2(NumericalityOpts{GreaterThan: fptr(2.5)}, 1); got != "N must be greater than 2.5" {
		t.Errorf("gt-frac = %q", got)
	}
	// passing values produce no errors.
	if got := full2(NumericalityOpts{GreaterThanOrEq: fptr(5), LessThanOrEq: fptr(10), Odd: true}, 7); got != "" {
		t.Errorf("valid num = %q", got)
	}
	// big.Int and float32 acceptance.
	bi := big.NewInt(9)
	if got := full2(NumericalityOpts{GreaterThan: fptr(0)}, bi); got != "" {
		t.Errorf("bigint = %q", got)
	}
	if got := full2(NumericalityOpts{GreaterThan: fptr(0)}, float32(2)); got != "" {
		t.Errorf("float32 = %q", got)
	}
}

func TestExclusionUniqueness(t *testing.T) {
	m := NewModel("M", "ms", Column{"name", "string"})
	m.ValidatesExclusion("name", []any{"admin", "root"})
	if got := full(m.Validate(m.Build(map[string]any{"name": "admin"}))); got != "Name is reserved" {
		t.Errorf("excl = %q", got)
	}
	if e := m.Validate(m.Build(map[string]any{"name": "bob"})); !e.Empty() {
		t.Error("excl ok")
	}
	// uniqueness with a host seam callback.
	u := NewModel("U", "us", Column{"email", "string"})
	u.ValidatesUniqueness("email", func(rec *Record) bool {
		v, _ := rec.Get("email")
		return v == "taken@x"
	})
	if got := full(u.Validate(u.Build(map[string]any{"email": "taken@x"}))); got != "Email has already been taken" {
		t.Errorf("uniq = %q", got)
	}
	if e := u.Validate(u.Build(map[string]any{"email": "free@x"})); !e.Empty() {
		t.Error("uniq free ok")
	}
	// nil callback is skipped.
	u2 := NewModel("U", "us", Column{"email", "string"})
	u2.ValidatesUniqueness("email", nil)
	if e := u2.Validate(u2.Build(map[string]any{"email": "x"})); !e.Empty() {
		t.Error("nil uniq skipped")
	}
}

func TestErrorsShape(t *testing.T) {
	m := NewModel("M", "ms", Column{"a", "string"}, Column{"b", "string"})
	m.ValidatesPresence("a").ValidatesPresence("b")
	e := m.Validate(m.Build(map[string]any{}))
	if e.Empty() {
		t.Fatal("expected errors")
	}
	if e.Count() != 2 {
		t.Errorf("count = %d", e.Count())
	}
	if !reflect.DeepEqual(e.On("a"), []string{"can't be blank"}) {
		t.Errorf("on(a) = %v", e.On("a"))
	}
	if got := e.Messages(); !reflect.DeepEqual(got["a"], []string{"can't be blank"}) {
		t.Errorf("messages = %v", got)
	}
	// Add dedup of attribute order.
	e.Add("a", "second")
	if got := e.On("a"); len(got) != 2 {
		t.Errorf("add second = %v", got)
	}
	if want := sortedKeys(e.Messages()); !reflect.DeepEqual(want, []string{"a", "b"}) {
		t.Errorf("keys = %v", want)
	}
	// full message with empty label (attribute name that humanizes to "").
	empty := newErrors(m)
	empty.Add("", "boom")
	if got := empty.FullMessages(); !reflect.DeepEqual(got, []string{"boom"}) {
		t.Errorf("empty label = %v", got)
	}
}

func TestValidHelpers(t *testing.T) {
	if !isBlank(nil) || !isBlank("") || !isBlank("  ") || !isBlank(false) ||
		!isBlank([]any{}) || !isBlank(Symbol(" ")) {
		t.Error("blank")
	}
	if isBlank("x") || isBlank(true) || isBlank(0) || isBlank([]any{1}) || isBlank(Symbol("x")) {
		t.Error("not-blank")
	}
	if plural(1, "character") != "1 character" || plural(3, "character") != "3 characters" {
		t.Error("plural")
	}
	if num(5) != "5" || num(2.5) != "2.5" {
		t.Error("num")
	}
	if !valueEqual(Symbol("a"), "a") || !valueEqual(1, 1.0) || valueEqual("a", "b") {
		t.Error("valueEqual")
	}
	if !valueEqual(nil, nil) {
		t.Error("valueEqual nil")
	}
	// toStr / num2 branches.
	if toStr(nil) != "" || toStr(Symbol("s")) != "s" || toStr(42) != "42" {
		t.Error("toStr")
	}
	if num2(int64(3)) != "3" || num2(big.NewInt(4)) != "4" || num2(true) != "true" ||
		num2(false) != "false" || num2(1.5) != "1.5" || num2([]any{}) != "" {
		t.Error("num2")
	}
	// toNumber edge cases.
	if _, _, ok := toNumber(""); ok {
		t.Error("empty num")
	}
	if _, _, ok := toNumber([]any{}); ok {
		t.Error("slice num")
	}
	if f, isInt, ok := toNumber("2.0"); !ok || !isInt || f != 2 {
		t.Error("float-int string")
	}
	if _, _, ok := toNumber("1.2.3"); ok {
		t.Error("bad num string")
	}
	if _, _, ok := toNumber(int32(5)); !ok {
		t.Error("int32")
	}
	if !contains([]any{1, 2}, 2) || contains([]any{1}, 9) {
		t.Error("contains")
	}
	if runeLen("héllo") != 5 {
		t.Error("runeLen")
	}
}
