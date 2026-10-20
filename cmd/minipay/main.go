package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xMurungi/minipay/internal/handlers"
	"github.com/xMurungi/minipay/internal/money"
)

func main() {
	fmt.Println("Hello world")
	run()
}

func run() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://postgres:example@localhost:5432/minipay?sslmode=disable")

	if err != nil {
		log.Fatal("cannot reach database", err)
	}

	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal("cannot reach database: ", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handlers.Healthz)
	mux.HandleFunc("GET /api/accounts/{id}/balance", func (w http.ResponseWriter, r *http.Request) {

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid account id", http.StatusBadRequest)
			return
		}

		var name string
		var balanceMinor int64

		err = pool.QueryRow(r.Context(),
			"SELECT name, balance_minor FROM accounts WHERE id = $1", id).Scan(&name, &balanceMinor)

		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}

		if err != nil {
			log.Println("query error:", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"accountId": id,
			"name": name,
			"balance": fmt.Sprintf("%d.%02d", balanceMinor/100, balanceMinor%100),
		})
	})

	type createAccountRequest struct {
		Name string `json:"name"`
	}

	mux.HandleFunc("POST /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		var req createAccountRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.DisallowUnknownFields()

		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		var id int64
		err := pool.QueryRow(r.Context(), `
			INSERT INTO accounts (name) VALUES ($1) RETURNING id`, req.Name).Scan(&id)
		
		if err != nil {
			log.Println("insert error:", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"accountId": id,
			"name": req.Name,
			"balance": "0.00",
		})

	})

	mux.HandleFunc("POST /api/accounts/{id}/deposit", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid account id", http.StatusBadRequest)
			return
		}

		var req struct {
			Amount string `json:"amount"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.DisallowUnknownFields()

		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		amount, err := money.ParseKES(req.Amount)
		if err != nil || amount < 0 {
			http.Error(w, "amount must be positive with at most 2 decimals", http.StatusBadRequest)
			return
		}

		reference, newBalance, err := deposit(r.Context(), pool, id, amount)
		if err != nil {
			switch {
			case errors.Is(err, errAccountNotFound):
				http.Error(w, "account not found", http.StatusNotFound)
			case errors.Is(err, errSystemAccount):
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			default:
				log.Println("deposit error:", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"reference": reference,
			"accountId": id,
			"deposited": money.FormatKES(amount),
			"newBalance": money.FormatKES(newBalance),
		})
	})
	
	log.Println("listening on :2323")
	log.Fatal(http.ListenAndServe(":2323", mux))
}

const systemAccountID int64 = 1

var (
	errAccountNotFound = errors.New("account not found")
	errSystemAccount   = errors.New("cannot deposit into the system account")
)

func deposit(ctx context.Context, pool *pgxpool.Pool, accountID, amountMinor int64) (reference string, newBalance int64, err error) {
	if accountID == systemAccountID {
		return "", 0, errSystemAccount
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback(ctx) // does nothing after a successful commit

	// lock both accounts, lowest id first
	rows, err := tx.Query(ctx,
		`SELECT id FROM accounts WHERE id = ANY($1) ORDER BY id FOR UPDATE`,
		[]int64{systemAccountID, accountID})
	if err != nil {
		return "", 0, err
	}
	found := 0
	for rows.Next() {
		found++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", 0, err
	}
	if found != 2 {
		return "", 0, errAccountNotFound
	}

	if _, err := tx.Exec(ctx,
		`UPDATE accounts SET balance_minor = balance_minor - $1 WHERE id = $2`,
		amountMinor, systemAccountID); err != nil {
		return "", 0, err
	}
	if err := tx.QueryRow(ctx,
		`UPDATE accounts SET balance_minor = balance_minor + $1 WHERE id = $2 RETURNING balance_minor`,
		amountMinor, accountID).Scan(&newBalance); err != nil {
		return "", 0, err
	}

	var transferID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO transfers (reference, source_account_id, dest_account_id, amount_minor, status)
		 VALUES ('TXN-' || to_char(now(), 'YYYYMMDD') || '-' || lpad(nextval('transfer_ref_seq')::text, 6, '0'),
		         $1, $2, $3, 'COMPLETED')
		 RETURNING id, reference`,
		systemAccountID, accountID, amountMinor).Scan(&transferID, &reference); err != nil {
		return "", 0, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO ledger_entries (transfer_id, account_id, direction, amount_minor)
		 VALUES ($1, $2, 'DEBIT', $4), ($1, $3, 'CREDIT', $4)`,
		transferID, systemAccountID, accountID, amountMinor); err != nil {
		return "", 0, err
	}
	
	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	return reference, newBalance, nil
}

