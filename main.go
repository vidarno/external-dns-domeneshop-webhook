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

	// Add the /healthz endpoint
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
	            w.WriteHeader(http.StatusMethodNotAllowed)
	            return
	        }
        	w.WriteHeader(http.StatusOK)
        	w.Write([]byte("OK"))
    	})

	fmt.Println("Server is listening on port 8888...")
	log.Fatal(http.ListenAndServe(":8888", nil))

}
