package db

import (
	"testing"
)

func TestParseZookieString(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"1766433684599320881.0000000000", 1766433684599320881, false},
		{"42", 42, false},
		{"not-a-timestamp", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseZookieString(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseZookieString(%q) expected error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseZookieString(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseZookieString(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateZookie(t *testing.T) {
	tests := []struct {
		name   string
		zookie int64
		want   bool
	}{
		{"positive zookie", 1234567890, true},
		{"zero zookie", 0, true},
		{"negative zookie", -1, false},
		{"large zookie", int64(999999999999999999), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateZookie(tt.zookie)
			if got != tt.want {
				t.Errorf("ValidateZookie(%d) = %v, want %v", tt.zookie, got, tt.want)
			}
		})
	}
}

// Note: Integration tests for AS OF SYSTEM TIME would require a real CockroachDB instance
// These would test:
// 1. Time-travel prevention: Write tuple, check with old zookie (should see write)
// 2. Zookie monotonicity: Verify zookies always increase
// 3. MAX(zookie, current_time) behavior
