package check

import (
	"math"
	"strings"
	"testing"
)

func TestValidate_GreaterThan(t *testing.T) {
	type TestStruct struct {
		IntField    int     `check:">5"`
		Int64Field  int64   `check:">100"`
		FloatField  float64 `check:">0.5"`
		UintField   uint    `check:">10"`
	}

	// Valid case
	s := TestStruct{IntField: 10, Int64Field: 200, FloatField: 1.0, UintField: 20}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Invalid cases
	s = TestStruct{IntField: 3, Int64Field: 200, FloatField: 1.0, UintField: 20}
	if err := Validate(s); err == nil {
		t.Error("expected error for IntField <= 5")
	}

	s = TestStruct{IntField: 10, Int64Field: 50, FloatField: 1.0, UintField: 20}
	if err := Validate(s); err == nil {
		t.Error("expected error for Int64Field <= 100")
	}

	s = TestStruct{IntField: 10, Int64Field: 200, FloatField: 0.3, UintField: 20}
	if err := Validate(s); err == nil {
		t.Error("expected error for FloatField <= 0.5")
	}
}

func TestValidate_GreaterOrEqual(t *testing.T) {
	type TestStruct struct {
		Field int `check:">=10"`
	}

	s := TestStruct{Field: 10}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for Field == 10, got %v", err)
	}

	s = TestStruct{Field: 15}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for Field > 10, got %v", err)
	}

	s = TestStruct{Field: 5}
	if err := Validate(s); err == nil {
		t.Error("expected error for Field < 10")
	}
}

func TestValidate_LessThan(t *testing.T) {
	type TestStruct struct {
		Field float64 `check:"<1.0"`
	}

	s := TestStruct{Field: 0.5}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	s = TestStruct{Field: 1.5}
	if err := Validate(s); err == nil {
		t.Error("expected error for Field >= 1.0")
	}
}

func TestValidate_LessOrEqual(t *testing.T) {
	type TestStruct struct {
		Field int `check:"<=100"`
	}

	s := TestStruct{Field: 100}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for Field == 100, got %v", err)
	}

	s = TestStruct{Field: 50}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for Field < 100, got %v", err)
	}

	s = TestStruct{Field: 150}
	if err := Validate(s); err == nil {
		t.Error("expected error for Field > 100")
	}
}

func TestValidate_MultipleRules(t *testing.T) {
	type TestStruct struct {
		Field float64 `check:">0,<=1.0"`
	}

	// Valid cases
	s := TestStruct{Field: 0.5}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for 0 < Field <= 1.0, got %v", err)
	}

	s = TestStruct{Field: 1.0}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for Field == 1.0, got %v", err)
	}

	// Invalid cases
	s = TestStruct{Field: 0.0}
	if err := Validate(s); err == nil {
		t.Error("expected error for Field <= 0")
	}

	s = TestStruct{Field: 1.5}
	if err := Validate(s); err == nil {
		t.Error("expected error for Field > 1.0")
	}

	s = TestStruct{Field: -0.5}
	if err := Validate(s); err == nil {
		t.Error("expected error for Field < 0")
	}
}

func TestValidate_MultipleFields(t *testing.T) {
	type TestStruct struct {
		Field1 int     `check:">0"`
		Field2 float64 `check:">0,<=1.0"`
		Field3 int64   `check:">=100"`
	}

	// All valid
	s := TestStruct{Field1: 5, Field2: 0.5, Field3: 100}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Multiple invalid fields - should return combined errors
	s = TestStruct{Field1: -1, Field2: 1.5, Field3: 50}
	errs := Validate(s)
	if len(errs) == 0 {
		t.Fatal("expected errors for multiple invalid fields")
	}

	errMsg := strings.Join(errs, "\n")
	if !strings.Contains(errMsg, "Field1") {
		t.Error("error should mention Field1")
	}
	if !strings.Contains(errMsg, "Field2") {
		t.Error("error should mention Field2")
	}
	if !strings.Contains(errMsg, "Field3") {
		t.Error("error should mention Field3")
	}
}

func TestValidate_NoTags(t *testing.T) {
	type TestStruct struct {
		Field1 int
		Field2 float64
	}

	s := TestStruct{Field1: -100, Field2: 999.9}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for struct without check tags, got %v", err)
	}
}

func TestValidate_MixedTagsAndNoTags(t *testing.T) {
	type TestStruct struct {
		Checked   int `check:">0"`
		Unchecked int
	}

	s := TestStruct{Checked: 5, Unchecked: -100}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	s = TestStruct{Checked: -5, Unchecked: 100}
	if err := Validate(s); err == nil {
		t.Error("expected error for Checked field")
	}
}

func TestValidate_AllIntTypes(t *testing.T) {
	type TestStruct struct {
		I   int   `check:">0"`
		I8  int8  `check:">0"`
		I16 int16 `check:">0"`
		I32 int32 `check:">0"`
		I64 int64 `check:">0"`
	}

	s := TestStruct{I: 1, I8: 1, I16: 1, I32: 1, I64: 1}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	s = TestStruct{I: -1, I8: -1, I16: -1, I32: -1, I64: -1}
	errs := Validate(s)
	if len(errs) == 0 {
		t.Fatal("expected errors for negative values")
	}
	errMsg := strings.Join(errs, "\n")
	if !strings.Contains(errMsg, "I:") {
		t.Error("error should mention field I")
	}
}

func TestValidate_AllUintTypes(t *testing.T) {
	type TestStruct struct {
		U   uint   `check:">5"`
		U8  uint8  `check:">5"`
		U16 uint16 `check:">5"`
		U32 uint32 `check:">5"`
		U64 uint64 `check:">5"`
	}

	s := TestStruct{U: 10, U8: 10, U16: 10, U32: 10, U64: 10}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	s = TestStruct{U: 3, U8: 3, U16: 3, U32: 3, U64: 3}
	err := Validate(s)
	if err == nil {
		t.Fatal("expected error for values <= 5")
	}
}

func TestValidate_AllFloatTypes(t *testing.T) {
	type TestStruct struct {
		F32 float32 `check:">0.5"`
		F64 float64 `check:">0.5"`
	}

	s := TestStruct{F32: 1.0, F64: 1.0}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	s = TestStruct{F32: 0.3, F64: 0.3}
	err := Validate(s)
	if err == nil {
		t.Fatal("expected error for values <= 0.5")
	}
}

func TestValidate_Pointer(t *testing.T) {
	type TestStruct struct {
		Field int `check:">0"`
	}

	s := &TestStruct{Field: 5}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error for pointer to struct, got %v", err)
	}

	s = &TestStruct{Field: -5}
	if err := Validate(s); err == nil {
		t.Error("expected error for invalid field in pointer to struct")
	}
}


func TestValidate_NaN(t *testing.T) {
	type TestStruct struct {
		Field1 float64 `check:">0"`
		Field2 float64 `check:">=0"`
		Field3 float64 `check:"<1"`
		Field4 float64 `check:"<=1"`
	}

	// NaN should fail all checks (NaN comparisons always return false)
	s := TestStruct{
		Field1: math.NaN(),
		Field2: math.NaN(),
		Field3: math.NaN(),
		Field4: math.NaN(),
	}
	errs := Validate(s)
	if len(errs) == 0 {
		t.Fatal("expected errors for NaN values")
	}

	errMsg := strings.Join(errs, "\n")
	// All 4 fields should fail
	if !strings.Contains(errMsg, "Field1") || !strings.Contains(errMsg, "Field2") ||
		!strings.Contains(errMsg, "Field3") || !strings.Contains(errMsg, "Field4") {
		t.Errorf("expected all 4 fields to fail: %v", errs)
	}
}

func TestValidate_InvalidTag(t *testing.T) {
	type TestStruct struct {
		Field int `check:"invalid"`
	}

	s := TestStruct{Field: 5}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid tag")
		} else if !strings.Contains(r.(string), "invalid check tag") {
			t.Errorf("expected 'invalid check tag' in panic message, got %v", r)
		}
	}()
	Validate(s)
}

func TestValidate_NestedStruct(t *testing.T) {
	type Inner struct {
		Value int `toml:"value" check:">0"`
	}
	type Outer struct {
		Inner Inner `toml:"inner"`
	}

	// Valid
	s := Outer{Inner: Inner{Value: 10}}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Invalid - should have qualified name "inner.value"
	s = Outer{Inner: Inner{Value: -5}}
	errs := Validate(s)
	if len(errs) == 0 {
		t.Fatal("expected error for nested invalid field")
	}
	errMsg := strings.Join(errs, "\n")
	if !strings.Contains(errMsg, "inner.value") {
		t.Errorf("expected 'inner.value' in error, got %v", errs)
	}
}

func TestValidate_DeeplyNested(t *testing.T) {
	type Level3 struct {
		Field int `toml:"field" check:">0"`
	}
	type Level2 struct {
		L3 Level3 `toml:"l3"`
	}
	type Level1 struct {
		L2 Level2 `toml:"l2"`
	}

	// Valid
	s := Level1{L2: Level2{L3: Level3{Field: 10}}}
	if err := Validate(s); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Invalid - should have qualified name "l2.l3.field"
	s = Level1{L2: Level2{L3: Level3{Field: -5}}}
	errs := Validate(s)
	if len(errs) == 0 {
		t.Fatal("expected error for deeply nested invalid field")
	}
	errMsg := strings.Join(errs, "\n")
	if !strings.Contains(errMsg, "l2.l3.field") {
		t.Errorf("expected 'l2.l3.field' in error, got %v", errs)
	}
}

func TestValidate_MultipleNestedErrors(t *testing.T) {
	type Inner1 struct {
		Value int `toml:"value" check:">0"`
	}
	type Inner2 struct {
		Value int `toml:"value" check:">10"`
	}
	type Outer struct {
		Inner1 Inner1 `toml:"inner1"`
		Inner2 Inner2 `toml:"inner2"`
		Top    int    `toml:"top" check:">100"`
	}

	// All invalid
	s := Outer{
		Inner1: Inner1{Value: -1},
		Inner2: Inner2{Value: 5},
		Top:    50,
	}
	errs := Validate(s)
	if len(errs) == 0 {
		t.Fatal("expected errors for multiple nested invalid fields")
	}

	errMsg := strings.Join(errs, "\n")
	if !strings.Contains(errMsg, "inner1.value") {
		t.Error("error should mention inner1.value")
	}
	if !strings.Contains(errMsg, "inner2.value") {
		t.Error("error should mention inner2.value")
	}
	if !strings.Contains(errMsg, "top") {
		t.Error("error should mention top")
	}
}

func TestValidate_NestedWithoutTomlTag(t *testing.T) {
	type Inner struct {
		Value int `check:">0"`
	}
	type Outer struct {
		MyInner Inner
	}

	// Invalid - should use field name "MyInner" when toml tag is missing
	s := Outer{MyInner: Inner{Value: -5}}
	errs := Validate(s)
	if len(errs) == 0 {
		t.Fatal("expected error for nested invalid field")
	}
	errMsg := strings.Join(errs, "\n")
	if !strings.Contains(errMsg, "MyInner.Value") {
		t.Errorf("expected 'MyInner.Value' in error, got %v", errs)
	}
}

func TestValidate_PointerField(t *testing.T) {
	type TestStruct struct {
		Power *float64 `toml:"power" check:">0,<=10"`
	}

	// Nil pointer - should skip validation
	s := TestStruct{Power: nil}
	if errs := Validate(s); errs != nil {
		t.Errorf("expected nil pointer to skip validation, got %v", errs)
	}

	// Valid non-nil pointer
	v := 2.0
	s = TestStruct{Power: &v}
	if errs := Validate(s); errs != nil {
		t.Errorf("expected valid pointer value to pass, got %v", errs)
	}

	// Invalid non-nil pointer (too low)
	v = 0.0
	s = TestStruct{Power: &v}
	errs := Validate(s)
	if errs == nil {
		t.Fatal("expected error for power <= 0")
	}
	errMsg := strings.Join(errs, "\n")
	if !strings.Contains(errMsg, "power") {
		t.Errorf("expected 'power' in error, got %v", errs)
	}

	// Invalid non-nil pointer (too high)
	v = 15.0
	s = TestStruct{Power: &v}
	errs = Validate(s)
	if errs == nil {
		t.Fatal("expected error for power > 10")
	}
}
