package main

import "testing"

func TestRun(t *testing.T) {
	// run() delegates to cmd.Execute(); with no args it prints help and returns nil.
	err := run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
