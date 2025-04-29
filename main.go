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

	// Main server for the webhook
    mainMux := http.NewServeMux()
	mainMux.HandleFunc("/", webhook.DomainFilter)
	mainMux.HandleFunc("/records", webhook.Records)
	mainMux.HandleFunc("/adjustendpoints", webhook.AdjustEndpoints)

	// Health check server
    healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
	            w.WriteHeader(http.StatusMethodNotAllowed)
	            return
	        }
        	w.WriteHeader(http.StatusOK)
        	w.Write([]byte("OK"))
    	})

	// Start the main server on port 8888
    go func() {
		fmt.Println("Server is listening on port 8888...")
		log.Fatal(http.ListenAndServe("localhost:8888", mainMux))
	}()

	// Start the health check server on port 8080
    fmt.Println("Health check server is listening on port 8080...")
    log.Fatal(http.ListenAndServe(":8080", healthMux))

}
