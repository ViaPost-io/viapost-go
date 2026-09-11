package main

import (
	"fmt"
	"os"

	"github.com/ViaPost-io/viapost-go/internal/generate"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: generate <openapi.yaml> <target-directory>")
		os.Exit(2)
	}
	if err := generate.Generate(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
