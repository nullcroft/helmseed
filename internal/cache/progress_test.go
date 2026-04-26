package cache

import "testing"

func TestNewProgress(t *testing.T) {
	p := NewProgress("cloning", 10)
	if p == nil {
		t.Fatal("expected non-nil Progress")
	}
	if p.label != "cloning" {
		t.Errorf("label = %q, want cloning", p.label)
	}
	if p.total != 10 {
		t.Errorf("total = %d, want 10", p.total)
	}
}

func TestNewProgress_ZeroTotal(t *testing.T) {
	p := NewProgress("test", 0)
	if p.total != 1 {
		t.Errorf("total = %d, want 1", p.total)
	}
}

func TestProgressQuiet(t *testing.T) {
	SetQuiet(true)
	defer SetQuiet(false)

	p := NewProgress("test", 5)
	p.Start()
	p.Add()
	p.Finish()
	// Should not panic when quiet
}

func TestProgressNonQuiet(t *testing.T) {
	SetQuiet(false)

	p := NewProgress("test", 5)
	p.Start()
	p.Add()
	p.Finish()
	// Should not panic when not quiet
}
