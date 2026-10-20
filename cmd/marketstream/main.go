package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"example.com/marketplace-live-operations/marketplace"
)

type server struct {
	client     *marketplace.Client
	operations marketplace.Operations
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	baseURL := os.Getenv("INFRAI_BASE_URL")
	if baseURL == "" {
		baseURL = marketplace.DefaultBaseURL
	}
	client := marketplace.NewClient(baseURL, key)
	s := server{client: client, operations: marketplace.Operations{Client: client}}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /setup", s.setup)
	mux.HandleFunc("POST /handoffs", s.handoff)
	mux.HandleFunc("POST /dashboard-token", s.dashboardToken)
	log.Println("marketstream listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (s server) setup(w http.ResponseWriter, r *http.Request) {
	if err := s.client.CreateChannel(r.Context(), marketplace.OperationsChannel, requestID()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"channel": marketplace.OperationsChannel})
}

func (s server) handoff(w http.ResponseWriter, r *http.Request) {
	var input marketplace.Handoff
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	update, err := s.operations.StreamHandoff(r.Context(), input, requestID())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, update)
}

func (s server) dashboardToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil || input.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id is required"})
		return
	}
	data, err := s.client.IssueDashboardToken(r.Context(), input.ClientID, marketplace.OperationsChannel, requestID())
	if err != nil {
		writeError(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, data)
}

func writeError(w http.ResponseWriter, err error) {
	status := marketplace.StatusFor(err)
	var apiErr *marketplace.APIError
	if errors.As(err, &apiErr) {
		writeJSON(w, status, map[string]any{"error": map[string]string{"code": apiErr.Code, "message": apiErr.Message}})
		return
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func writeRawJSON(w http.ResponseWriter, status int, value json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(value)
}

func requestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
