package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvoiceHandoff(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		calls        int
	}{{"paid order", "paid", 2}, {"unpaid order", "pending", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			paths := []string{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") == "" {
					t.Errorf("request boundary: %s", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/pdf/generate" {
					if !strings.Contains(body["html"].(string), "Widget") {
						t.Error("invoice line missing")
					}
					w.Write([]byte(`{"ok":true,"data":{"url":"https://example.test/invoice.pdf"},"metadata":{}}`))
				} else {
					if !strings.Contains(body["body"].(string), "https://example.test/invoice.pdf") {
						t.Error("PDF URL missing from buyer update")
					}
					w.Write([]byte(`{"ok":true,"data":{"message_id":"msg-1"},"metadata":{}}`))
				}
			}))
			defer srv.Close()
			o := Order{ID: "order-7", Seller: "Studio", SellerAddress: "Main St", BuyerName: "Lee", BuyerEmail: "lee@example.test", Item: "Widget", Quantity: 2, UnitCents: 1250, Status: tc.status}
			got, err := issue(context.Background(), client{key: "test-key", base: srv.URL, http: srv.Client()}, o)
			if len(paths) != tc.calls {
				t.Fatalf("calls = %v", paths)
			}
			if tc.calls == 0 {
				if err == nil {
					t.Fatal("expected rejection")
				}
				return
			}
			if err != nil || got.MessageID != "msg-1" || got.OrderID != o.ID {
				t.Fatalf("handoff = %+v, %v", got, err)
			}
		})
	}
}
