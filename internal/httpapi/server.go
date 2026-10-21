package httpapi

import (
	"net/http"

	"github.com/xMurungi/minipay/internal/ledger"
)

type Server struct {
	svc *ledger.Service
}

func New(svc *ledger.Service) http.Handler {
	s := &Server{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /api/accounts", s.createAccount)
	mux.HandleFunc("GET /api/accounts/{id}/balance", s.balance)
	mux.HandleFunc("POST /api/accounts/{id}/deposit", s.deposit)
	mux.HandleFunc("POST /api/transfers", s.createTransfer)
	mux.HandleFunc("GET /api/transfers/{reference}", s.getTransfer)
	mux.HandleFunc("GET /api/accounts/{id}/transactions", s.transactions)
	mux.HandleFunc("POST /api/transfers/{reference}/reverse", s.reverseTransfer)
	mux.HandleFunc("GET /api/admin/reconciliation", s.reconciliation)
	
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(
		w,
		http.StatusOK,
		map[string]string{"status": "ok"})
}
