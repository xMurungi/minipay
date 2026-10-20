package handlers

import (
	"net/http"

	"github.com/xMurungi/minipay/internal/httpapi"
)

func Healthz(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
