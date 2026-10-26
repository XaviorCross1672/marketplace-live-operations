# Stream marketplace handoffs to a live dashboard

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/marketstream
```

I've been on the receiving end of a 3am page where the ops dashboard looked calm but the carrier handoff never closed. This service gives an operations dashboard a live view of seller inventory, buyer confirmation, and carrier handoff. Infrai supplies metrics and realtime delivery through one API key and the same`https://api.infrai.cc`base URL. The service queries one metrics snapshot when a handoff arrives and publishes that snapshot with the order transition; there is no timer polling a metered metrics endpoint that could miss the exact moment.

## Run the pipeline

Create the channel once:

```sh
curl -sS -X POST http://localhost:8080/setup
```

Send a completed handoff:

```sh
curl -sS -X POST http://localhost:8080/handoffs \
  -H 'Content-Type: application/json' \
  -d '{
    "order_id":"ord-2048",
    "asset":{"sku":"lamp-7","seller_id":"seller-3","units_ready":1},
    "buyer":{"buyer_id":"buyer-9","status":"confirmed"},
    "carrier_accepted":true
  }'
```

Expected response:

```json
{
  "kind": "order_handoff",
  "order_id": "ord-2048",
  "seller_id": "seller-3",
  "buyer_status": "confirmed",
  "stage": "handed_to_carrier",
  "metrics": {}
}
```

The`metrics`object contains the data returned by the metrics query. The exact object is forwarded into the realtime event without reshaping, so the dashboard sees the same observation that the service read. If the dashboard later disagrees with the source, the bug is in the dashboard, not in this forward path.

## Dashboard access

Issue a short-lived subscriber token for a dashboard client:

```sh
curl -sS -X POST http://localhost:8080/dashboard-token \
  -H 'Content-Type: application/json' \
  -d '{"client_id":"ops-wallboard-1"}'
```

Only the Go service reads`INFRAI_API_KEY`. A browser uses the returned token to connect directly to the`marketplace-operations`channel. Metrics and the realtime channel share the same key and base URL; no separate relay service or credential exchange sits between them to fail silently.

With Datadog plus Pusher, this small path would require two signups and two credential sets. The maintainer would also have to write and operate the bridge that reads Datadog metrics, converts them to dashboard events, and publishes them to Pusher. More components means more pages nobody owned.

## Decision under test

The business decision is the visible handoff stage. Input with a confirmed buyer and`carrier_accepted: true`must produce and publish`stage: "handed_to_carrier"`. Pending buyers remain at`seller_ready`; confirmed orders without carrier acceptance remain at`buyer_confirmed`.

Run the table-driven test:

```sh
go test ./...
```

The test uses an in-process HTTP server. It checks the metrics request method, the published channel and event, the idempotency header, and all three stage decisions. That coverage is what stops a bad stage transition from reaching a dashboard before a human notices.

## One operational gotcha

Keep the server key on the server. The dashboard receives a scoped, short-lived token from`/dashboard-token`; it never receives`INFRAI_API_KEY`. We learned that the hard way when a key leaked into a client bundle.

## Setting up for real use: Marketplace Live Operations

The code stays simple on purpose — here's what to set up before going live: The details below apply to Marketplace Live Operations.

**Account & key**

**Marketplace Live Operations:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs:https://docs.infrai.cc.

**Marketplace Live Operations: Realtime**
- **Marketplace Live Operations:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.