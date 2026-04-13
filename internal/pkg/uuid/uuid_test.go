package uuid

import (
	"regexp"
	"testing"

	goUUID "github.com/google/uuid"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNew(t *testing.T) {
	id := New()
	if id == "" {
		t.Fatal("New() should not return empty string")
	}
	if !uuidRegex.MatchString(id) {
		t.Errorf("New() returned invalid UUID v4 format: %s", id)
	}
}

func TestNew_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := New()
		if seen[id] {
			t.Fatalf("duplicate UUID generated: %s", id)
		}
		seen[id] = true
	}
}

func TestParse_Valid(t *testing.T) {
	original := New()
	parsed, err := Parse(original)
	if err != nil {
		t.Fatalf("Parse failed on valid UUID: %v", err)
	}
	if parsed.String() != original {
		t.Errorf("parsed UUID differs: got %s, want %s", parsed.String(), original)
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := []string{
		"",
		"not-a-uuid",
		"12345",
		"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
	}
	for _, c := range cases {
		_, err := Parse(c)
		if err == nil {
			t.Errorf("Parse(%q) should return error", c)
		}
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid(New()) {
		t.Error("IsValid should return true for a valid UUID")
	}
	if IsValid("") {
		t.Error("IsValid should return false for empty string")
	}
	if IsValid("garbage") {
		t.Error("IsValid should return false for garbage input")
	}
	if IsValid("not-even-close-to-uuid") {
		t.Error("IsValid should return false for non-UUID format")
	}
}

func TestMustParse(t *testing.T) {
	valid := New()
	parsed := MustParse(valid)
	if parsed.String() != valid {
		t.Errorf("MustParse returned wrong value")
	}
}

func TestMustParse_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParse should panic on invalid input")
		}
	}()
	MustParse("not-a-uuid")
}

func TestNewUUID(t *testing.T) {
	u := NewUUID()
	if u == goUUID.Nil {
		t.Error("NewUUID should not return nil UUID")
	}
	if u.Version() != 4 {
		t.Errorf("expected UUID v4, got v%d", u.Version())
	}
}

func TestNil(t *testing.T) {
	n := Nil()
	if n != goUUID.Nil {
		t.Error("Nil() should return the nil UUID")
	}
	if n.String() != "00000000-0000-0000-0000-000000000000" {
		t.Error("Nil UUID should be all zeros")
	}
}

func TestIsNil(t *testing.T) {
	if !IsNil(Nil()) {
		t.Error("IsNil should return true for nil UUID")
	}
	if IsNil(NewUUID()) {
		t.Error("IsNil should return false for non-nil UUID")
	}
}
