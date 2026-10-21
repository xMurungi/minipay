package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xMurungi/minipay/internal/httpapi"
	"github.com/xMurungi/minipay/internal/ledger"
	"github.com/xMurungi/minipay/internal/migrations"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := getenv("DATABASE_URL", "postgres://postgres:example@localhost:5432/minipay?sslmode=disable")

	if err := migrations.Run(dbURL); err != nil {
		log.Fatal("migrations: ", err)
	}

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
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Println("shutdown:", err)
		}
	}()

	log.Println("listening on", srv.Addr)
	
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	log.Println("stopped")
}
