package tenant

import (
	"testing"
)

func TestDBName_Deterministic(t *testing.T) {
	id := "0123abcd-4567-89ef-0123-456789abcdef"
	a, err := DBName(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := DBName(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != b {
		t.Errorf("DBName not deterministic: %q vs %q", a, b)
	}
}

func TestDBName_FormatCorrect(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "all-zero uuid",
			id:   "00000000-0000-0000-0000-000000000000",
			want: "tenant_00000000000000000000000000000000",
		},
		{
			name: "mixed-case input is normalised to lowercase",
			id:   "0123ABCD-4567-89EF-0123-456789ABCDEF",
			want: "tenant_0123abcd456789ef0123456789abcdef",
		},
		{
			name: "all-f uuid",
			id:   "ffffffff-ffff-ffff-ffff-ffffffffffff",
			want: "tenant_ffffffffffffffffffffffffffffffff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DBName(tt.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("DBName(%q) = %q, want %q", tt.id, got, tt.want)
			}
			// Whatever DBName produces must also pass ValidateDBName.
			if err := ValidateDBName(got); err != nil {
				t.Errorf("ValidateDBName(%q) failed: %v", got, err)
			}
		})
	}
}

func TestDBName_RejectsInvalidUUID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"empty string", ""},
		{"not a uuid", "not-a-uuid"},
		{"too short", "0123abcd"},
		{"random text", "hello world"},
		{"trailing garbage", "0123abcd-4567-89ef-0123-456789abcdefXX"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DBName(tt.id)
			if err == nil {
				t.Errorf("expected error for invalid uuid %q", tt.id)
			}
		})
	}
}

func TestValidateDBName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Accept canonical
		{"canonical zeroes", "tenant_00000000000000000000000000000000", false},
		{"canonical mixed hex", "tenant_0123abcd456789ef0123456789abcdef", false},
		{"canonical all f", "tenant_ffffffffffffffffffffffffffffffff", false},

		// Reject malformed
		{"empty", "", true},
		{"missing prefix", "00000000000000000000000000000000", true},
		{"wrong prefix", "tenants_00000000000000000000000000000000", true},
		{"too short hex", "tenant_00", true},
		{"too long hex", "tenant_000000000000000000000000000000000000", true},
		{"uppercase hex (Postgres lower-only convention)", "tenant_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true},
		{"non-hex chars", "tenant_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", true},
		{"with hyphens", "tenant_00000000-0000-0000-0000-000000000000", true},
		{"trailing whitespace", "tenant_00000000000000000000000000000000 ", true},
		{"leading whitespace", " tenant_00000000000000000000000000000000", true},
		{"injection attempt", "tenant_00000000000000000000000000000000;DROP DATABASE foo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDBName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
			}
		})
	}
}
