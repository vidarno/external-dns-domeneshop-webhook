package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/vidarno/external-dns-domeneshop-webhook/pkg/webhook"

)

func main() {

	apiToken := os.Getenv("TOKEN")
	apiSecret := os.Getenv("SECRET")

	webhook := webhook.New(apiToken, apiSecret)

	// Set up the server to listen on port 8888 and use the handler
	http.HandleFunc("/", webhook.DomainFilter)
	http.HandleFunc("/records", webhook.Records)
	http.HandleFunc("/adjustendpoints", webhook.AdjustEndpoints)

	fmt.Println("Server is listening on port 8888...")
	log.Fatal(http.ListenAndServe(":8888", nil))

}
