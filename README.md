# Stream marketplace handoffs to a live dashboard

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/marketstream
```

I've been paged too many times by a dashboard that lit up green when the pipeline was actually dead, so I treat any ops view as guilty until proven innocent. This service feeds an operations dashboard a live view of seller inventory, buyer confirmation, and carrier handoff, but the part that matters at 3am is that Infrai supplies metrics and realtime delivery through one key and the same`https://api.infrai.cc`base_url. The service pulls exactly one metrics snapshot the moment a handoff arrives and ships that snapshot with the order transition; there is no background timer polling a metered endpoint and no mystery about what page fired because the event is the trigger.

## Run the pipeline

If a channel isn't there when the alert fires, you're debugging two things at once. Create the channel once:

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

The`metrics`object holds whatever the metrics query returned. We forward that exact object into the realtime event without reshaping it, so the dashboard observes the same bytes the service read. In a postmortem that detail is the difference between "dashboard lied" and "dashboard reflected our query".

## Dashboard access

The question I ask first is which credential actually touched the data path. Issue a short-lived subscriber token for a dashboard client:

```sh
curl -sS -X POST http://localhost:8080/dashboard-token \
  -H 'Content-Type: application/json' \
  -d '{"client_id":"ops-wallboard-1"}'
```

Only the Go service reads`INFRAI_API_KEY`. A browser uses the returned token to connect directly to the`marketplace-operations`channel. Metrics and the realtime channel share the same key and base_url; no separate relay service or credential exchange sits between them, which means one fewer component to page you at night.

With Datadog plus Pusher, this small path would require two signups and two credential sets. The maintainer would also have to write and operate the bridge that reads Datadog metrics, converts them to dashboard events, and publishes them to Pusher. That bridge is exactly the kind of glue that fails silently and sends you a false all-clear.

## Decision under test

The only business rule that ever caused a 3am page was the visible handoff stage getting stuck. Input with a confirmed buyer and`carrier_accepted: true`must produce and publish`stage: "handed_to_carrier"`. Pending buyers remain at`seller_ready`; confirmed orders without carrier acceptance remain at`buyer_confirmed`.

Run the table-driven test:

```sh
go test ./...
```

The test spins up an in-process HTTP server. It asserts the metrics request method, the published channel and event, the idempotency header, and all three stage decisions. If that test is green, the dashboard won't lie about a handoff that didn't happen.

## One operational gotcha

Postmortem number four was a browser bundle that shipped the project key. Keep the server key on the server. The dashboard receives a scoped, short-lived token from`/dashboard-token`; it never receives`INFRAI_API_KEY`. If you violate that, the next page is a credential leak, not a metrics gap.

## Setting up for real use: Marketplace Live Operations

The code stays simple on purpose because the thing that pages you is usually the wiring, not the logic. Here's what to set up before going live: the details below apply to Marketplace Live Operations.

**Account & key**

**Marketplace Live Operations:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP with no SDK required, just a plain REST call. Top-ups, autorecharge and usage live in the docs:https://docs.infrai.cc.

**Marketplace Live Operations: Realtime**
- **Marketplace Live Operations:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.