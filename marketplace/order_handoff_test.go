package marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamHandoffStage(t *testing.T) {
	tests := []struct {
		name            string
		buyerStatus     string
		carrierAccepted bool
		wantStage       string
	}{
		{name: "seller inventory ready", buyerStatus: "pending", wantStage: "seller_ready"},
		{name: "buyer confirmed", buyerStatus: "confirmed", wantStage: "buyer_confirmed"},
		{name: "carrier owns handoff", buyerStatus: "confirmed", carrierAccepted: true, wantStage: "handed_to_carrier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var published DashboardUpdate
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/metrics/query":
					if r.Method != http.MethodGet {
						t.Fatalf("metrics method = %s", r.Method)
					}
					w.Write([]byte(`{"ok":true,"data":{"orders_active":12},"error":null,"metadata":{}}`))
				case "/v1/realtime/publish":
					if r.Method != http.MethodPost {
						t.Fatalf("publish method = %s", r.Method)
					}
					var request struct {
						Channel string          `json:"channel"`
						Event   string          `json:"event"`
						Data    DashboardUpdate `json:"data"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Fatal(err)
					}
					if request.Channel != OperationsChannel || request.Event != "marketplace.handoff" {
						t.Fatalf("unexpected route: %#v", request)
					}
					if r.Header.Get("Idempotency-Key") != "req-test" {
						t.Fatal("missing idempotency key")
					}
					published = request.Data
					w.Write([]byte(`{"ok":true,"data":{"published":true},"error":null,"metadata":{}}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key")
			got, err := (Operations{Client: client}).StreamHandoff(context.Background(), Handoff{
				OrderID:         "ord-2048",
				Asset:           Asset{SKU: "lamp-7", SellerID: "seller-3", UnitsReady: 1},
				Buyer:           BuyerUpdate{BuyerID: "buyer-9", Status: tt.buyerStatus},
				CarrierAccepted: tt.carrierAccepted,
			}, "req-test")
			if err != nil {
				t.Fatal(err)
			}
			if got.Stage != tt.wantStage || published.Stage != tt.wantStage {
				t.Fatalf("stage = %q, published = %q, want %q", got.Stage, published.Stage, tt.wantStage)
			}
		})
	}
}
