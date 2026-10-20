package httpapi

import (
	"net/http"

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

	res, err := s.svc.Transfer(r.Context(), req.SourceAccount, req.DestinationAccount, amount)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"reference": res.Reference,
		"status":    "COMPLETED",
		"amount":    money.FormatKES(amount),
	})
}
