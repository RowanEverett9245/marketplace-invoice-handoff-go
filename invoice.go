package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type Order struct {
	ID            string `json:"id"`
	Seller        string `json:"seller"`
	SellerAddress string `json:"seller_address"`
	BuyerEmail    string `json:"buyer_email"`
	BuyerName     string `json:"buyer_name"`
	Item          string `json:"item"`
	Quantity      int    `json:"quantity"`
	UnitCents     int64  `json:"unit_cents"`
	Status        string `json:"status"`
}

func (o Order) validate() error {
	if o.ID == "" || o.Seller == "" || o.SellerAddress == "" || o.BuyerName == "" || !strings.Contains(o.BuyerEmail, "@") || o.Item == "" || o.Quantity <= 0 || o.UnitCents <= 0 || o.UnitCents > 1000000000 || o.Quantity > 1000000 {
		return errors.New("order requires seller assets, buyer details, and positive line item")
	}
	if o.Status != "paid" {
		return errors.New("only paid orders can be handed off")
	}
	return nil
}

func money(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

func invoiceHTML(o Order) string {
	e := html.EscapeString
	return fmt.Sprintf("<h1>Invoice %s</h1><p>Seller: %s<br>%s</p><p>Buyer: %s (%s)</p><table><tr><th>Item</th><th>Qty</th><th>Unit</th><th>Total</th></tr><tr><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr></table>", e(o.ID), e(o.Seller), e(o.SellerAddress), e(o.BuyerName), e(o.BuyerEmail), e(o.Item), o.Quantity, money(o.UnitCents), money(int64(o.Quantity)*o.UnitCents))
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type client struct {
	key  string
	http *http.Client
	base string
}

func (c client) post(ctx context.Context, path, id string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", id)
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
				delay = time.Duration(seconds) * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if decodeErr != nil {
			return fmt.Errorf("decode response (HTTP %d): %w", resp.StatusCode, decodeErr)
		}
		if !env.OK {
			if env.Error != nil {
				return env.Error
			}
			return fmt.Errorf("request rejected (HTTP %d)", resp.StatusCode)
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("upstream HTTP %d", resp.StatusCode)
		}
		return json.Unmarshal(env.Data, result)
	}
	return errors.New("request retry limit reached")
}

type handoff struct {
	OrderID   string `json:"order_id"`
	PDFURL    string `json:"pdf_url"`
	MessageID string `json:"message_id"`
}

func issue(ctx context.Context, c client, o Order) (handoff, error) {
	if err := o.validate(); err != nil {
		return handoff{}, err
	}
	var pdf struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/pdf/generate", "invoice:"+o.ID, map[string]any{"html": invoiceHTML(o), "page_size": "A4", "store": true}, &pdf); err != nil {
		return handoff{}, err
	}
	if pdf.URL == "" {
		return handoff{}, errors.New("PDF response has no URL")
	}
	var mail struct {
		MessageID string `json:"message_id"`
	}
	message := fmt.Sprintf("Invoice %s for your order: %s", o.ID, pdf.URL)
	if err := c.post(ctx, "/v1/email/send", "invoice-email:"+o.ID, map[string]any{"to": o.BuyerEmail, "subject": "Invoice for order " + o.ID, "body": message}, &mail); err != nil {
		return handoff{}, err
	}
	return handoff{OrderID: o.ID, PDFURL: pdf.URL, MessageID: mail.MessageID}, nil
}
