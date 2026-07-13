package main

import "testing"

func TestResolveBindDefaultsToLoopback(t *testing.T) {
	if got := resolveBind(""); got != "127.0.0.1" {
		t.Fatalf("resolveBind(\"\") = %q, want loopback", got)
	}
	if got := resolveBind("0.0.0.0"); got != "0.0.0.0" {
		t.Fatalf("resolveBind(explicit) = %q", got)
	}
}
