// Command send submits one transactional email using environment-provided values.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	viapost "github.com/ViaPost-io/viapost-go"
)

func main() {
	apiKey := requireEnvironment("VIAPOST_API_KEY")
	from := requireEnvironment("VIAPOST_FROM")
	to := requireEnvironment("VIAPOST_TO")

	client, err := viapost.NewClient(apiKey)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.Email.Send(ctx, viapost.SendRequest{
		From:    from,
		To:      []string{to},
		Subject: "ViaPost Go SDK",
		Text:    "Envio realizado pelo SDK oficial do ViaPost.",
		Stream:  viapost.StreamTransactional,
	}, viapost.WithIdempotencyKey(fmt.Sprintf("sdk-example-%d", time.Now().Unix())))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("aceitas=%d rejeitadas=%d", len(result.Accepted), len(result.Rejected))
}

func requireEnvironment(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("defina %s", name)
	}
	return value
}
