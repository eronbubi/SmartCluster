package ids

import (
	"testing"
	"time"
)

func TestNewV7Format(t *testing.T) {
	id := NewV7()
	if !IsUUID(id) {
		t.Fatalf("%q is not a canonical UUID", id)
	}
	if id[14] != '7' {
		t.Errorf("version nibble = %c, want 7", id[14])
	}
	if v := id[19]; v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Errorf("variant nibble = %c, want 8-b", v)
	}
}

func TestNewV7IsTimeOrdered(t *testing.T) {
	base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	earlier := newV7(base)
	later := newV7(base.Add(time.Millisecond))
	if earlier >= later {
		t.Errorf("%s should sort before %s", earlier, later)
	}
}

func TestNewV7IsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for range 10000 {
		id := NewV7()
		if seen[id] {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = true
	}
}

func TestIsUUID(t *testing.T) {
	for s, want := range map[string]bool{
		"0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e": true,
		"0192F1C4-7E2A-7B3C-9D4E-5F6A7B8C9D0E": false,
		"0192f1c47e2a7b3c9d4e5f6a7b8c9d0e":     false,
		"":                                     false,
		"../../etc/passwd":                     false,
	} {
		if got := IsUUID(s); got != want {
			t.Errorf("IsUUID(%q) = %v, want %v", s, got, want)
		}
	}
}
