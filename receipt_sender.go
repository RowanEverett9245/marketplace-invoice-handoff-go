package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("set INFRAI_API_KEY")
	}
	c := client{key: key, base: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
	http.HandleFunc("POST /invoices", func(w http.ResponseWriter, r *http.Request) {
		var order Order
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&order); err != nil {
			http.Error(w, "invalid order JSON", http.StatusBadRequest)
			return
		}
		result, err := issue(r.Context(), c, order)
		if err != nil {
			status := http.StatusBadGateway
			var rejected *apiError
			if errors.As(err, &rejected) || err.Error() == "only paid orders can be handed off" || err.Error() == "order requires seller assets, buyer details, and positive line item" {
				status = http.StatusUnprocessableEntity
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
