package main

import "testing"

func TestSemanticallyEqualYAML_IgnoresSerializationDifferences(t *testing.T) {
	compact := []byte("openapi: 3.1.1\ntags: [Email, Domains]\n")
	expanded := []byte("openapi: 3.1.1\ntags:\n  - Email\n  - Domains\n")

	equal, err := semanticallyEqualYAML(compact, expanded)
	if err != nil {
		t.Fatalf("semanticallyEqualYAML() error = %v", err)
	}
	if !equal {
		t.Fatal("semanticallyEqualYAML() = false, want true")
	}
}

func TestSemanticallyEqualYAML_DetectsContractChange(t *testing.T) {
	first := []byte("openapi: 3.1.1\ninfo: {version: 1.0.0}\n")
	second := []byte("openapi: 3.1.1\ninfo: {version: 1.1.0}\n")

	equal, err := semanticallyEqualYAML(first, second)
	if err != nil {
		t.Fatalf("semanticallyEqualYAML() error = %v", err)
	}
	if equal {
		t.Fatal("semanticallyEqualYAML() = true, want false")
	}
}
