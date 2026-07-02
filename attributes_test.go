// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

import (
	"math/big"
	"reflect"
	"testing"
	"time"
)

func castModel() *Model {
	return NewModel("T", "ts",
		Column{"i", "integer"}, Column{"b", "bigint"}, Column{"f", "float"},
		Column{"d", "decimal"}, Column{"flag", "boolean"}, Column{"s", "string"},
		Column{"txt", "text"}, Column{"at", "datetime"}, Column{"day", "date"})
}

func TestTypeCasting(t *testing.T) {
	m := castModel()
	r := m.Build(map[string]any{
		"i": "42", "f": "3.5", "flag": "true", "s": 7, "at": "2026-07-02 12:00:00",
	})
	if v, _ := r.Get("i"); v != int64(42) {
		t.Errorf("i = %v", v)
	}
	if v, _ := r.Get("f"); v != 3.5 {
		t.Errorf("f = %v", v)
	}
	if v, _ := r.Get("flag"); v != true {
		t.Errorf("flag = %v", v)
	}
	if v, _ := r.Get("s"); v != "7" {
		t.Errorf("s = %v", v)
	}
	if v, _ := r.Get("at"); !v.(time.Time).Equal(time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("at = %v", v)
	}
	// unknown attribute stored verbatim.
	r.Set("extra", []any{1})
	if v, _ := r.Get("extra"); !reflect.DeepEqual(v, []any{1}) {
		t.Errorf("extra = %v", v)
	}
}

func TestCastBranches(t *testing.T) {
	if castInt(nil) != nil || castInt(int(1)) != int64(1) || castInt(int64(2)) != int64(2) ||
		castInt(int32(3)) != int64(3) || castInt(3.9) != int64(3) || castInt(true) != int64(1) ||
		castInt(false) != int64(0) || castInt("5") != int64(5) || castInt("2.7") != int64(2) {
		t.Error("castInt")
	}
	if castInt("nope") != nil || castInt([]any{}) != nil {
		t.Error("castInt bad")
	}
	if castInt(big.NewInt(7)).(*big.Int).Int64() != 7 {
		t.Error("castInt bigint")
	}
	if castFloat(nil) != nil || castFloat(1.5) != 1.5 || castFloat(float32(2)) != float64(2) ||
		castFloat(3) != 3.0 || castFloat(int64(4)) != 4.0 || castFloat("5.5") != 5.5 {
		t.Error("castFloat")
	}
	if castFloat("bad") != nil || castFloat([]any{}) != nil {
		t.Error("castFloat bad")
	}
	if castFloat(big.NewInt(6)) != 6.0 {
		t.Error("castFloat bigint")
	}
	if castBool(nil) != nil || castBool(true) != true || castBool(0) != false ||
		castBool(int64(1)) != true || castBool("no") != false || castBool("yes") != true ||
		castBool([]any{}) != true {
		t.Error("castBool")
	}
	if castString(nil) != nil || castString("x") != "x" || castString(Symbol("s")) != "s" ||
		castString(true) != "true" || castString(false) != "false" || castString(9) != "9" {
		t.Error("castString")
	}
	if castTime(nil) != nil {
		t.Error("castTime nil")
	}
	now := time.Now()
	if !castTime(now).(time.Time).Equal(now) {
		t.Error("castTime passthrough")
	}
	if castTime("2026-07-02").(time.Time).Year() != 2026 {
		t.Error("castTime date")
	}
	if castTime("nonsense") != nil || castTime([]any{}) != nil {
		t.Error("castTime bad")
	}
	if castTo("unknown", "raw") != "raw" {
		t.Error("castTo default")
	}
	if castTo("decimal", "1.5") != 1.5 {
		t.Error("castTo decimal")
	}
}

func TestDirtyTracking(t *testing.T) {
	m := castModel()
	// Build: every set attribute is changed (no persisted original).
	r := m.Build(map[string]any{"i": 1, "s": "a"})
	if !r.Changed() {
		t.Error("built record should be changed")
	}
	if names := r.ChangedAttributeNames(); len(names) != 2 {
		t.Errorf("changed names = %v", names)
	}
	ch := r.Changes()
	if ch["i"].Was != nil || ch["i"].Now != int64(1) {
		t.Errorf("changes i = %+v", ch["i"])
	}

	// Load: clean until modified.
	l := m.Load(map[string]any{"i": 1, "s": "a"})
	if l.Changed() {
		t.Error("loaded record clean")
	}
	if l.AttributeChanged("i") {
		t.Error("i unchanged after load")
	}
	l.Set("i", 2)
	if !l.AttributeChanged("i") {
		t.Error("i changed")
	}
	if l.AttributeChanged("s") {
		t.Error("s still clean")
	}
	if c := l.Changes(); c["i"].Was != int64(1) || c["i"].Now != int64(2) {
		t.Errorf("i change = %+v", c["i"])
	}
	// setting back to original clears the change.
	l.Set("i", 1)
	if l.AttributeChanged("i") {
		t.Error("i reverted")
	}
	// SaveClean re-baselines.
	l.Set("i", 9)
	l.SaveClean()
	if l.Changed() {
		t.Error("clean after save")
	}
	// setting an attribute never loaded (no original) counts as changed once set.
	l.Set("f", 1.0)
	if !l.AttributeChanged("f") {
		t.Error("new attr changed")
	}
	// time-valued attribute equality.
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := m.Load(map[string]any{"at": t0})
	tr.Set("at", t0)
	if tr.AttributeChanged("at") {
		t.Error("same time unchanged")
	}
	tr.Set("at", t0.Add(time.Hour))
	if !tr.AttributeChanged("at") {
		t.Error("diff time changed")
	}
	// time vs non-time original.
	tr2 := m.Load(map[string]any{"i": 1})
	tr2.Set("i", t0)
	if !tr2.AttributeChanged("i") {
		t.Error("time replacing int changed")
	}
}

func TestRecordAttributesAndValid(t *testing.T) {
	m := NewModel("T", "ts", Column{"name", "string"})
	m.ValidatesPresence("name")
	r := m.Build(map[string]any{"name": "x"})
	if !r.Valid() {
		t.Error("valid")
	}
	if a := r.Attributes(); a["name"] != "x" {
		t.Errorf("attributes = %v", a)
	}
	if !m.Build(map[string]any{}).Validate().Empty() == false {
		// presence fails on empty; Valid() false.
	}
	if m.Build(map[string]any{}).Valid() {
		t.Error("empty invalid")
	}
}
