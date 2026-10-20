package httpapi

import (
	"net/http"
	"time"

	"github.com/xMurungi/minipay/internal/ledger"
	"github.com/xMurungi/minipay/internal/money"
)

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceAccount      int64  `json:"sourceAccount"`
		DestinationAccount int64  `json:"destinationAccount"`
		Amount             string `json:"amount"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON")
		return
	}

	amount, err := money.ParseKES(req.Amount)
	if err != nil || amount <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be positive with at most 2 decimals")
		return
	}

	if req.SourceAccount == ledger.SystemAccountID || req.DestinationAccount == ledger.SystemAccountID {
		writeError(w, http.StatusUnprocessableEntity, "SYSTEM_ACCOUNT", "system account cannot be used in a transfer")
		return
	}

	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}

	res, err := s.svc.Transfer(r.Context(), key, req.SourceAccount, req.DestinationAccount, amount)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	if res.Replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"reference": res.Reference,
		"status":    "COMPLETED",
		"amount":    money.FormatKES(amount),
	})
}

func (s *Server) getTransfer(w http.ResponseWriter, r *http.Request) {

	t, err := s.svc.GetTransfer(r.Context(), r.PathValue("reference"))
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reference":          t.Reference,
		"sourceAccount":      t.SourceID,
		"destinationAccount": t.DestID,
		"amount":             money.FormatKES(t.AmountMinor),
		"status":             t.Status,
		"createdAt":          t.CreatedAt.UTC().Format(time.RFC3339),
	})
}
