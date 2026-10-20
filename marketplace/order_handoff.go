package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
)

const OperationsChannel = "marketplace-operations"

type Asset struct {
	SKU        string `json:"sku"`
	SellerID   string `json:"seller_id"`
	UnitsReady int    `json:"units_ready"`
}

type BuyerUpdate struct {
	BuyerID string `json:"buyer_id"`
	Status  string `json:"status"`
}

type Handoff struct {
	OrderID         string      `json:"order_id"`
	Asset           Asset       `json:"asset"`
	Buyer           BuyerUpdate `json:"buyer"`
	CarrierAccepted bool        `json:"carrier_accepted"`
}

type DashboardUpdate struct {
	Kind        string          `json:"kind"`
	OrderID     string          `json:"order_id"`
	SellerID    string          `json:"seller_id"`
	BuyerStatus string          `json:"buyer_status"`
	Stage       string          `json:"stage"`
	Metrics     json.RawMessage `json:"metrics"`
}

type Operations struct {
	Client *Client
}

func (o Operations) StreamHandoff(ctx context.Context, input Handoff, requestID string) (DashboardUpdate, error) {
	if input.OrderID == "" || input.Asset.SKU == "" || input.Asset.SellerID == "" || input.Buyer.BuyerID == "" {
		return DashboardUpdate{}, fmt.Errorf("order_id, sku, seller_id, and buyer_id are required")
	}
	if input.Asset.UnitsReady < 1 {
		return DashboardUpdate{}, fmt.Errorf("units_ready must be positive")
	}

	stage := "seller_ready"
	if input.Buyer.Status == "confirmed" {
		stage = "buyer_confirmed"
	}
	if input.Buyer.Status == "confirmed" && input.CarrierAccepted {
		stage = "handed_to_carrier"
	}

	metrics, err := o.Client.QueryMetrics(ctx)
	if err != nil {
		return DashboardUpdate{}, fmt.Errorf("query metrics: %w", err)
	}
	update := DashboardUpdate{
		Kind:        "order_handoff",
		OrderID:     input.OrderID,
		SellerID:    input.Asset.SellerID,
		BuyerStatus: input.Buyer.Status,
		Stage:       stage,
		Metrics:     metrics,
	}
	if err := o.Client.Publish(ctx, OperationsChannel, "marketplace.handoff", update, requestID); err != nil {
		return DashboardUpdate{}, fmt.Errorf("publish dashboard update: %w", err)
	}
	return update, nil
}
