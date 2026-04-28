package cache

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewProgress(t *testing.T) {
	p := NewProgress("cloning", 10, &bytes.Buffer{}, false)
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
	p := NewProgress("test", 0, &bytes.Buffer{}, false)
	if p.total != 1 {
		t.Errorf("total = %d, want 1", p.total)
	}
}

func TestProgressQuiet(t *testing.T) {
	var out bytes.Buffer

	p := NewProgress("test", 5, &out, true)
	p.Start()
	p.Add()
	p.Finish()
	if out.Len() != 0 {
		t.Fatalf("expected no output in quiet mode, got %q", out.String())
	}
}

func TestProgressNonQuiet(t *testing.T) {
	var out bytes.Buffer

	p := NewProgress("test", 5, &out, false)
	p.Start()
	p.Add()
	p.Finish()
	got := out.String()
	if !strings.Contains(got, "test:") {
		t.Fatalf("expected progress label in output, got %q", got)
	}
}
