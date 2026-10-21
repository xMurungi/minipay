package httpapi

import (
	"net/http"

	"github.com/xMurungi/minipay/internal/money"
)

func (s *Server) reconciliation(w http.ResponseWriter, r *http.Request) {
	
	rec, err := s.svc.Reconcile(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}

	mismatches := make([]map[string]any, 0, len(rec.Mismatches))

	for _, m := range rec.Mismatches {
		mismatches = append(mismatches, map[string]any{
			"accountId":     m.AccountID,
			"storedBalance": money.FormatKES(m.BalanceMinor),
			"ledgerBalance": money.FormatKES(m.LedgerMinor),
			"difference":    money.FormatKES(m.BalanceMinor - m.LedgerMinor),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              rec.OK,
		"accountsChecked": rec.AccountsChecked,
		"ledgerNet":       money.FormatKES(rec.LedgerNetMinor),
		"mismatches":      mismatches,
	})
}
