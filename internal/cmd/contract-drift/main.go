// Command contract-drift compares the vendored contract with the canonical
// public document by meaning rather than YAML serialization.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/ghodss/yaml"
)

const maxContractBytes = 16 << 20

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: contract-drift <canonical-url> <local-openapi.yaml>")
		os.Exit(2)
	}
	canonical, err := downloadContract(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	local, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read local contract: %v\n", err)
		os.Exit(1)
	}
	equal, err := semanticallyEqualYAML(canonical, local)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !equal {
		fmt.Fprintln(os.Stderr, "public OpenAPI contract drift detected; synchronize openapi.yaml and regenerate api/")
		os.Exit(1)
	}
	fmt.Println("public OpenAPI contract is semantically synchronized")
}

func downloadContract(rawURL string) ([]byte, error) {
	client, err := newContractHTTPClient(rawURL)
	if err != nil {
		return nil, err
	}
	response, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("download canonical contract: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download canonical contract: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxContractBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read canonical contract: %w", err)
	}
	if len(body) > maxContractBytes {
		return nil, fmt.Errorf("canonical contract exceeds %d bytes", maxContractBytes)
	}
	return body, nil
}

func newContractHTTPClient(rawURL string) (*http.Client, error) {
	origin, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(origin.Scheme, "https") || origin.Hostname() == "" || origin.User != nil || origin.Fragment != "" {
		return nil, fmt.Errorf("canonical contract URL must be an absolute HTTPS URL without credentials or fragment")
	}
	return &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: contractRedirectPolicy(origin),
	}, nil
}

func contractRedirectPolicy(origin *url.URL) func(*http.Request, []*http.Request) error {
	wantOrigin := contractOrigin(origin)
	return func(request *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("canonical contract redirect limit exceeded")
		}
		if request == nil || request.URL == nil || request.URL.User != nil || contractOrigin(request.URL) != wantOrigin {
			return fmt.Errorf("canonical contract redirect changed origin")
		}
		return nil
	}
}

func contractOrigin(target *url.URL) string {
	if target == nil || !strings.EqualFold(target.Scheme, "https") || target.Hostname() == "" {
		return ""
	}
	port := target.Port()
	if port == "" {
		port = "443"
	}
	return "https://" + net.JoinHostPort(strings.ToLower(target.Hostname()), port)
}

func semanticallyEqualYAML(first, second []byte) (bool, error) {
	decode := func(source []byte) (any, error) {
		asJSON, err := yaml.YAMLToJSON(source)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(asJSON))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	}
	left, err := decode(first)
	if err != nil {
		return false, fmt.Errorf("decode canonical contract: %w", err)
	}
	right, err := decode(second)
	if err != nil {
		return false, fmt.Errorf("decode local contract: %w", err)
	}
	return reflect.DeepEqual(left, right), nil
}
