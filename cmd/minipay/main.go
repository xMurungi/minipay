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

		// reference, newBalance, err := deposit(r.Context(), pool, id, amount)
		res, err := transfer(r.Context(), pool, systemAccountID, id, amount)
		if err != nil {
			WriteTransferError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"reference": res.Reference,
			"accountId": id,
			"deposited": money.FormatKES(amount),
			"newBalance": money.FormatKES(res.DestBalance),
		})
	})

	mux.HandleFunc("POST /api/transfers", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SourceAccount      int64  `json:"sourceAccount"`
			DestinationAccount int64  `json:"destinationAccount"`
			Amount             string `json:"amount"`
		}

		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.DisallowUnknownFields()

		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		amount, err := money.ParseKES(req.Amount)
		if err != nil || amount <= 0 {
			http.Error(w, "amount must be positive with at most 2 decimals", http.StatusBadRequest)
			return
		}

		if req.SourceAccount == systemAccountID || req.DestinationAccount == systemAccountID {
			http.Error(w, "system account cannot be used in transfer", http.StatusUnprocessableEntity)
			return
		}

		res, err := transfer(r.Context(), pool, req.SourceAccount, req.DestinationAccount, amount)
		if err != nil {
			WriteTransferError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"reference": res.Reference,
			"status": "COMPLETED",
			"amount": money.FormatKES(amount),
		})
	})
	
	log.Println("listening on :2323")
	log.Fatal(http.ListenAndServe(":2323", mux))
}

const systemAccountID int64 = 1

var (
	errAccountNotFound = errors.New("account not found")
	errSystemAccount   = errors.New("cannot deposit into the system account")
	errSameAccount       = errors.New("source and destination must differ")
	errInsufficientFunds = errors.New("insufficient funds")
)

type transferResult struct {
	Reference string
	SourceBalance int64
	DestBalance int64
}

func transfer(ctx context.Context, pool *pgxpool.Pool, sourceID, destID, amountMinor int64) (transferResult, error) {
	if sourceID == destID {
		return transferResult{}, errSameAccount
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return transferResult{}, err
	}
	defer tx.Rollback(ctx)

	// lock both accounts, lowest id first, and read what we need to decide
	rows, err := tx.Query(ctx,
		`SELECT id, balance_minor, is_system FROM accounts
		  WHERE id = ANY($1) ORDER BY id FOR UPDATE`,
		[]int64{sourceID, destID})
	if err != nil {
		return transferResult{}, err
	}
	type acct struct {
		balance int64
		system  bool
	}
	accts := map[int64]acct{}
	for rows.Next() {
		var id int64
		var a acct
		if err := rows.Scan(&id, &a.balance, &a.system); err != nil {
			return transferResult{}, err
		}
		accts[id] = a
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return transferResult{}, err
	}

	src, okS := accts[sourceID]
	_, okD := accts[destID]
	if !okS || !okD {
		return transferResult{}, errAccountNotFound
	}
	if !src.system && src.balance < amountMinor {
		return transferResult{}, errInsufficientFunds
	}

	var res transferResult
	if err := tx.QueryRow(ctx,
		`UPDATE accounts SET balance_minor = balance_minor - $1 WHERE id = $2 RETURNING balance_minor`,
		amountMinor, sourceID).Scan(&res.SourceBalance); err != nil {
		return transferResult{}, err
	}
	if err := tx.QueryRow(ctx,
		`UPDATE accounts SET balance_minor = balance_minor + $1 WHERE id = $2 RETURNING balance_minor`,
		amountMinor, destID).Scan(&res.DestBalance); err != nil {
		return transferResult{}, err
	}

	var transferID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO transfers (reference, source_account_id, dest_account_id, amount_minor, status)
		 VALUES ('TXN-' || to_char(now(), 'YYYYMMDD') || '-' || lpad(nextval('transfer_ref_seq')::text, 6, '0'),
		         $1, $2, $3, 'COMPLETED')
		 RETURNING id, reference`,
		sourceID, destID, amountMinor).Scan(&transferID, &res.Reference); err != nil {
		return transferResult{}, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO ledger_entries (transfer_id, account_id, direction, amount_minor)
		 VALUES ($1, $2, 'DEBIT', $4), ($1, $3, 'CREDIT', $4)`,
		transferID, sourceID, destID, amountMinor); err != nil {
		return transferResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return transferResult{}, err
	}
	return res, nil
}

func WriteTransferError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAccountNotFound):
		http.Error(w, "account not found", http.StatusNotFound)
	case errors.Is(err, errSameAccount):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, errInsufficientFunds):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, errSystemAccount):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		log.Println("transfer error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

