// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// validation is one declared validator on an attribute.
type validation struct {
	attr string
	run  func(m *Model, rec *Record, errs *Errors)
	// order preserves declaration order for message emission.
}

// Errors is the pure-Go shape of ActiveModel::Errors: per-attribute message
// lists plus the full_messages rendering ("Attr message"). Attributes and
// messages are kept in declaration/insertion order to match ActiveRecord.
type Errors struct {
	model    *Model
	attrs    []string            // attribute insertion order
	byAttr   map[string][]string // attribute => messages
}

func newErrors(m *Model) *Errors {
	return &Errors{model: m, byAttr: map[string][]string{}}
}

// Add records a message for an attribute (ActiveModel::Errors#add), preserving
// first-seen attribute order.
func (e *Errors) Add(attr, message string) {
	if _, ok := e.byAttr[attr]; !ok {
		e.attrs = append(e.attrs, attr)
	}
	e.byAttr[attr] = append(e.byAttr[attr], message)
}

// Empty reports whether there are no errors (ActiveModel::Errors#empty?).
func (e *Errors) Empty() bool { return len(e.byAttr) == 0 }

// Count returns the total number of messages.
func (e *Errors) Count() int {
	n := 0
	for _, ms := range e.byAttr {
		n += len(ms)
	}
	return n
}

// Messages returns the per-attribute message map (a copy).
func (e *Errors) Messages() map[string][]string {
	out := make(map[string][]string, len(e.byAttr))
	for k, v := range e.byAttr {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// On returns the messages for one attribute.
func (e *Errors) On(attr string) []string {
	return append([]string(nil), e.byAttr[attr]...)
}

// FullMessages returns the "Humanized attr message" list, in attribute
// insertion order, matching ActiveModel::Errors#full_messages.
func (e *Errors) FullMessages() []string {
	var out []string
	for _, a := range e.attrs {
		label := humanize(a)
		for _, msg := range e.byAttr[a] {
			out = append(out, e.fullMessage(label, msg))
		}
	}
	return out
}

// fullMessage joins the humanized label and the message ("Name can't be
// blank"), matching ActiveModel's default "%{attribute} %{message}" format.
func (e *Errors) fullMessage(label, msg string) string {
	if label == "" {
		return msg
	}
	return label + " " + msg
}

// -- validator declarations --------------------------------------------------

// ValidatesPresence adds a presence validator (validates :attr, presence:true).
func (m *Model) ValidatesPresence(attr string) *Model {
	m.validations = append(m.validations, validation{attr: attr, run: func(m *Model, rec *Record, errs *Errors) {
		if isBlank(rec.get(attr)) {
			errs.Add(attr, "can't be blank")
		}
	}})
	return m
}

// LengthOpts configures a length validator (nil field = unset).
type LengthOpts struct {
	Minimum *int
	Maximum *int
	Is      *int
}

// ValidatesLength adds a length validator (validates :attr, length:{...}).
func (m *Model) ValidatesLength(attr string, o LengthOpts) *Model {
	m.validations = append(m.validations, validation{attr: attr, run: func(m *Model, rec *Record, errs *Errors) {
		v := rec.get(attr)
		if v == nil {
			// presence is a separate validator; nil skips length in AR unless
			// allow_nil is false and minimum set — AR treats nil length as 0.
		}
		n := runeLen(v)
		if o.Is != nil && n != *o.Is {
			errs.Add(attr, "is the wrong length (should be "+plural(*o.Is, "character")+")")
		}
		if o.Minimum != nil && n < *o.Minimum {
			errs.Add(attr, "is too short (minimum is "+plural(*o.Minimum, "character")+")")
		}
		if o.Maximum != nil && n > *o.Maximum {
			errs.Add(attr, "is too long (maximum is "+plural(*o.Maximum, "character")+")")
		}
	}})
	return m
}

// ValidatesFormat adds a format validator (validates :attr, format:{with:re}).
func (m *Model) ValidatesFormat(attr string, with *regexp.Regexp) *Model {
	m.validations = append(m.validations, validation{attr: attr, run: func(m *Model, rec *Record, errs *Errors) {
		v := rec.get(attr)
		if v == nil || !with.MatchString(toStr(v)) {
			errs.Add(attr, "is invalid")
		}
	}})
	return m
}

// NumericalityOpts configures a numericality validator.
type NumericalityOpts struct {
	OnlyInteger      bool
	GreaterThan      *float64
	GreaterThanOrEq  *float64
	LessThan         *float64
	LessThanOrEq     *float64
	EqualTo          *float64
	Other            *float64 // other_than
	Odd              bool
	Even             bool
}

// ValidatesNumericality adds a numericality validator.
func (m *Model) ValidatesNumericality(attr string, o NumericalityOpts) *Model {
	m.validations = append(m.validations, validation{attr: attr, run: func(m *Model, rec *Record, errs *Errors) {
		v := rec.get(attr)
		f, intOK, ok := toNumber(v)
		if !ok || (o.OnlyInteger && !intOK) {
			if o.OnlyInteger && ok && !intOK {
				errs.Add(attr, "must be an integer")
			} else {
				errs.Add(attr, "is not a number")
			}
			return
		}
		if o.GreaterThan != nil && !(f > *o.GreaterThan) {
			errs.Add(attr, "must be greater than "+num(*o.GreaterThan))
		}
		if o.GreaterThanOrEq != nil && !(f >= *o.GreaterThanOrEq) {
			errs.Add(attr, "must be greater than or equal to "+num(*o.GreaterThanOrEq))
		}
		if o.LessThan != nil && !(f < *o.LessThan) {
			errs.Add(attr, "must be less than "+num(*o.LessThan))
		}
		if o.LessThanOrEq != nil && !(f <= *o.LessThanOrEq) {
			errs.Add(attr, "must be less than or equal to "+num(*o.LessThanOrEq))
		}
		if o.EqualTo != nil && f != *o.EqualTo {
			errs.Add(attr, "must be equal to "+num(*o.EqualTo))
		}
		if o.Other != nil && f == *o.Other {
			errs.Add(attr, "must be other than "+num(*o.Other))
		}
		if o.Odd && int64(f)%2 == 0 {
			errs.Add(attr, "must be odd")
		}
		if o.Even && int64(f)%2 != 0 {
			errs.Add(attr, "must be even")
		}
	}})
	return m
}

// ValidatesInclusion adds an inclusion validator (validates :attr,
// inclusion:{in:list}).
func (m *Model) ValidatesInclusion(attr string, in []any) *Model {
	m.validations = append(m.validations, validation{attr: attr, run: func(m *Model, rec *Record, errs *Errors) {
		if !contains(in, rec.get(attr)) {
			errs.Add(attr, "is not included in the list")
		}
	}})
	return m
}

// ValidatesExclusion adds an exclusion validator (validates :attr,
// exclusion:{in:list}).
func (m *Model) ValidatesExclusion(attr string, in []any) *Model {
	m.validations = append(m.validations, validation{attr: attr, run: func(m *Model, rec *Record, errs *Errors) {
		if contains(in, rec.get(attr)) {
			errs.Add(attr, "is reserved")
		}
	}})
	return m
}

// ValidatesUniqueness adds a uniqueness validator. Uniqueness needs a database
// query, which is a host seam: the exists callback reports whether a conflicting
// row exists (the host runs [Relation.ExistsSQL] through its Adapter). When
// exists is nil the validator is skipped (documented).
func (m *Model) ValidatesUniqueness(attr string, exists func(rec *Record) bool) *Model {
	m.validations = append(m.validations, validation{attr: attr, run: func(m *Model, rec *Record, errs *Errors) {
		if exists != nil && exists(rec) {
			errs.Add(attr, "has already been taken")
		}
	}})
	return m
}

// Validate runs every validator against rec and returns the collected Errors.
func (m *Model) Validate(rec *Record) *Errors {
	errs := newErrors(m)
	for _, v := range m.validations {
		v.run(m, rec, errs)
	}
	return errs
}

// -- helpers -----------------------------------------------------------------

// isBlank mirrors ActiveSupport#blank? for the value types we carry: nil, "",
// whitespace-only string, false, and empty array are blank.
func isBlank(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case Symbol:
		return strings.TrimSpace(string(x)) == ""
	case bool:
		return !x
	case []any:
		return len(x) == 0
	default:
		return false
	}
}

func runeLen(v any) int {
	return len([]rune(toStr(v)))
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case Symbol:
		return string(x)
	case nil:
		return ""
	default:
		return num2(v)
	}
}

// num2 renders a number-ish value to a string for length/format checks.
func num2(v any) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case *big.Int:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// toNumber coerces a value to float64, reporting whether it is integral and
// whether it is numeric at all, matching ActiveRecord's numericality parsing
// (numeric types pass; strings are parsed).
func toNumber(v any) (f float64, isInt bool, ok bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true, true
	case int64:
		return float64(x), true, true
	case int32:
		return float64(x), true, true
	case *big.Int:
		bf := new(big.Float).SetInt(x)
		g, _ := bf.Float64()
		return g, true, true
	case float64:
		return x, x == float64(int64(x)), true
	case float32:
		g := float64(x)
		return g, g == float64(int64(g)), true
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false, false
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return float64(i), true, true
		}
		if g, err := strconv.ParseFloat(s, 64); err == nil {
			return g, g == float64(int64(g)), true
		}
		return 0, false, false
	default:
		return 0, false, false
	}
}

// num renders a validation-message number the way Ruby to_s does (integers have
// no ".0").
func num(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// plural renders "N word" with an English plural for count-based length
// messages ("3 characters", "1 character").
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// contains reports whether list holds a value equal to v (value equality over
// the carried scalar types).
func contains(list []any, v any) bool {
	for _, x := range list {
		if valueEqual(x, v) {
			return true
		}
	}
	return false
}

// valueEqual compares two carried values for inclusion/exclusion.
func valueEqual(a, b any) bool {
	sa, aok := symbolName(a)
	sb, bok := symbolName(b)
	if aok && bok {
		return sa == sb
	}
	fa, _, na := toNumber(a)
	fb, _, nb := toNumber(b)
	if na && nb {
		return fa == fb
	}
	return a == b
}

// sortedKeys is a small helper kept for deterministic map iteration in tests.
func sortedKeys(m map[string][]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
