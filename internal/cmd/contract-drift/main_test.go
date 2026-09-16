package main

import (
	"net/http"
	"net/url"
	"testing"
)

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

func TestNewContractHTTPClient_RequiresHTTPS(t *testing.T) {
	_, err := newContractHTTPClient("http://docs.viapost.io/openapi/public.yaml")
	if err == nil {
		t.Fatal("newContractHTTPClient() error = nil, want insecure URL rejection")
	}
}

func TestContractRedirectPolicy_RejectsCrossOriginRedirect(t *testing.T) {
	origin, err := url.Parse("https://docs.viapost.io/openapi/public.yaml")
	if err != nil {
		t.Fatal(err)
	}
	policy := contractRedirectPolicy(origin)
	request, err := http.NewRequest(http.MethodGet, "https://example.com/openapi/public.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy(request, nil); err == nil {
		t.Fatal("redirect policy error = nil, want cross-origin rejection")
	}
}

func TestContractRedirectPolicy_AllowsBoundedSameOriginRedirect(t *testing.T) {
	origin, err := url.Parse("https://docs.viapost.io/openapi/public.yaml")
	if err != nil {
		t.Fatal(err)
	}
	policy := contractRedirectPolicy(origin)
	request, err := http.NewRequest(http.MethodGet, "https://docs.viapost.io/openapi/canonical.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy(request, []*http.Request{{}, {}, {}}); err != nil {
		t.Fatalf("redirect policy error = %v, want nil", err)
	}
	if err := policy(request, []*http.Request{{}, {}, {}, {}}); err == nil {
		t.Fatal("redirect policy error = nil, want redirect-count rejection")
	}
}
