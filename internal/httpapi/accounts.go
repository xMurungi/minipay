package httpapi

import (
	"net/http"
	"strings"

	"github.com/xMurungi/minipay/internal/ledger"
	"github.com/xMurungi/minipay/internal/money"
)

func accountJSON(a ledger.Account) map[string]any {
	return map[string]any{
		"accountId": a.ID,
		"name":      a.Name,
		"balance":   money.FormatKES(a.BalanceMinor),
	}
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "NAME_REQUIRED", "name is required")
		return
	}
	acct, err := s.svc.CreateAccount(r.Context(), name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, accountJSON(acct))
}

func (s *Server) balance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid account id")
		return
	}
	acct, err := s.svc.GetAccount(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accountJSON(acct))
}

func (s *Server) deposit(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid account id")
		return
	}
	var req struct {
		Amount string `json:"amount"`
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
	if id == ledger.SystemAccountID {
		writeError(w, http.StatusUnprocessableEntity, "SYSTEM_ACCOUNT", "cannot deposit into the system account")
		return
	}

	res, err := s.svc.Transfer(r.Context(), ledger.SystemAccountID, id, amount)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"reference":  res.Reference,
		"accountId":  id,
		"deposited":  money.FormatKES(amount),
		"newBalance": money.FormatKES(res.DestBalance),
	})
}