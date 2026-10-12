# PQAPI hosted payment integration

PQAPI adds a hosted WeChat/Alipay checkout for wallet top-ups. The application
calculates the payable amount on the server, sends the amount in integer CNY
cents, and credits the balance only after a signed `payment.paid` webhook.

## Configuration

Set these options through the administrator option API. Never expose the secret
to the browser or commit a production value:

| Option | Example |
|---|---|
| `PQAPIBaseURL` | `https://shop.pqapi.shop` |
| `PQAPISiteID` | `MAIN_SITE` |
| `PQAPISecret` | shared HMAC secret |

Configure the corresponding PQAPI site's notification URL as:

```text
https://your-new-api.example/api/pqapi/webhook
```

The user-facing flow is `POST /api/user/pqapi/pay`. The server creates a local
pending top-up first, then calls `POST /api/v1/checkout-sessions` and returns the
hosted `checkout_url`. The UI navigates to that URL in the same tab, which also
works on mobile browsers supported by the configured PQAPI payment channels.

## Security behavior

- API requests use `HMAC-SHA256(timestamp + "\\n" + nonce + "\\n" + rawBody)`.
- Webhooks use `HMAC-SHA256(timestamp + "\\n" + eventID + "\\n" + rawBody)`.
- Webhook timestamps have a five-minute tolerance.
- Site ID, event ID, currency, payment status, provider order ID, and exact
  amount are checked before balance credit.
- Event IDs are claimed in `payment_provider_events`; repeated delivery does not
  credit the account twice.
- Monetary values sent to PQAPI are positive integer CNY cents.

The production secret is intentionally absent from this repository.
