// bank-sim is a fake acquirer/issuer. Its outcome is driven by magic tokens so tests are
// deterministic (no real randomness, no real processor):
//
//	tok_ok       → approved
//	tok_declined → declined
//	tok_timeout  → sleeps past the client's deadline (the client sees a timeout)
//
// Any other token is treated as approved, so ad-hoc curl calls "just work".
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	tokDeclined  = "tok_declined"
	tokTimeout   = "tok_timeout"
	timeoutSleep = 5 * time.Second // longer than the api's bank-call deadline
)

type authorizeRequest struct {
	IntentID string `json:"intent_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Token    string `json:"token"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/authorize", authorize)
	mux.HandleFunc("/capture", capture)

	log.Printf("bank-sim listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// authorize decides the outcome from the magic token. tok_timeout sleeps so the caller's
// context deadline fires first — modelling the "did the bank receive it?" ambiguity that the
// api must handle safely.
func authorize(w http.ResponseWriter, r *http.Request) {
	var req authorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	switch req.Token {
	case tokDeclined:
		writeJSON(w, http.StatusOK, map[string]string{"status": "declined"})
	case tokTimeout:
		time.Sleep(timeoutSleep)
		writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
	}
}

// capture always approves in this simplified acquirer: an authorized hold is captured for real.
func capture(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
