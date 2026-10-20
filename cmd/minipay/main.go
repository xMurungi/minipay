package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xMurungi/minipay/internal/httpapi"
	"github.com/xMurungi/minipay/internal/ledger"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx := context.Background()

	dbURL := getenv("DATABASE_URL", "postgres://postgres:example@localhost:5432/minipay?sslmode=disable")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}

	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal("cannot reach database: ", err)
	}

	srv := &http.Server{
		Addr:              getenv("ADDR", ":8080"),
		Handler:           httpapi.New(ledger.NewService(pool)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("listening on", srv.Addr)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
