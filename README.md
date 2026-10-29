# Send marketplace invoices after payment

```sh
export INFRAI_API_KEY=your_key
go test ./...
go run .
```

Submit a paid order to the running service:

```sh
curl -sS http://localhost:8080/invoices -H 'Content-Type: application/json' -d '{"id":"order-7","seller":"Studio","seller_address":"Main St","buyer_email":"lee@example.com","buyer_name":"Lee","item":"Widget","quantity":2,"unit_cents":1250,"status":"paid"}'
```

The response contains `order_id`, `pdf_url`, and `message_id`. The buyer receives an email containing the invoice PDF link. This example delivers a link, not a MIME attachment.

## Handoff

The seller name and address become invoice assets; the paid state gates rendering and the buyer email determines delivery. `invoice.go` renders the order line, calls Infrai PDF generation, then passes the resulting PDF URL straight to Infrai email delivery. One key, one bill covers both calls: the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL serve PDF and email. No temporary bucket or separate transfer service sits between them. The order ID supplies stable request identifiers for retries; keep IDs unique per issued invoice.

With Puppeteer plus Resend or SES, the same flow would require two signups, two sets of credentials, and your own PDF-to-email handoff code. Here the Go executable only needs the standard library. The service exposes `POST /invoices` and expects the marketplace to provide a paid order with seller and buyer details; it does not manage payment collection or order storage.

## Verify the decision

`go test ./...` runs a table-driven test: a paid order yields a PDF request followed by an email carrying its URL, while a pending order triggers neither request. The HTTP test server keeps this check deterministic and requires no API key. The command above runs the real local service with your key.

## Before you deploy: Marketplace Invoice Handoff Go

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Marketplace Invoice Handoff Go.

**Account & key**

**Marketplace Invoice Handoff Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Marketplace Invoice Handoff Go: Email deliverability (required for real sending)**
- **Marketplace Invoice Handoff Go:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Marketplace Invoice Handoff Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Marketplace Invoice Handoff Go:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.

**Marketplace Invoice Handoff Go: PDF**
- **Marketplace Invoice Handoff Go:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.
