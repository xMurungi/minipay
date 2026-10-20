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
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(
		w, 
		http.StatusOK, 
		map[string]string{"status": "ok"})
}
