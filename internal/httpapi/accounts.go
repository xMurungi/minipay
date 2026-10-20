package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

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

	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}

	res, err := s.svc.Transfer(r.Context(), key, ledger.SystemAccountID, id, amount)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	out := map[string]any{
		"reference": res.Reference,
		"accountId": id,
		"deposited": money.FormatKES(amount),
	}
	if res.Replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	} else {
		out["newBalance"] = money.FormatKES(res.DestBalance)
	}

	writeJSON(
		w,
		http.StatusCreated,
		out)
}

func (s *Server) transactions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid account id")
		return
	}

	limit := 20	
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 100")
			return
		}
		limit = n
	}

	var before int64
	if v := r.URL.Query().Get("before"); v != "" {
		before, err := strconv.ParseInt(v, 10, 64)
		if err != nil || before < 0 {
			writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "before must be a non-negative number")
			return
		}
	}

	if _, err := s.svc.GetAccount(r.Context(), id); err != nil {
		writeDomainError(w, err)
		return
	}

	entries, err := s.svc.ListEntries(r.Context(), id, limit, before)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		items = append(items, map[string]any{
			"entryId":   e.EntryID,
			"reference": e.Reference,
			"type":      e.Direction,
			"amount":    money.FormatKES(e.AmountMinor),
			"createdAt": e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"accountId": id, "transactions": items})
}
