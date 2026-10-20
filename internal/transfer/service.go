package transfer

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSameAccount       = errors.New("source and destination must differ")
	ErrAccountNotFound   = errors.New("account not found")
	ErrAccountFrozen     = errors.New("account not active")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrKeyReuse          = errors.New("idempotency key reused with a different request")
)

type Input struct {
	Key, RequestHash string
	Source, Dest     int64
	AmountMinor      int64
	Channel          string
}

type Result struct {
	Status int             // HTTP status to return
	Body   json.RawMessage // response body to return
}

type Service struct{ Pool *pgxpool.Pool }

func (s *Service) Transfer(ctx context.Context, in Input) (Result, error) {
	if in.Source == in.Dest {
		return Result{}, ErrSameAccount
	}

	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{}) // READ COMMITTED + row locks is enough here
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	// 1. idempotency: the unique key makes the database pick one winner.
	tag, err := tx.Exec(ctx,
		`INSERT INTO idempotency_keys (key, request_hash) VALUES ($1, $2)
		 ON CONFLICT (key) DO NOTHING`, in.Key, in.RequestHash)
	if err != nil {
		return Result{}, err
	}
	if tag.RowsAffected() == 0 {
		// someone else owns this key. A concurrent twin blocks on the insert
		// above until the first transaction commits, so we see its final result.
		var hash string
		var status int
		var body []byte
		err := tx.QueryRow(ctx,
			`SELECT request_hash, response_status, response_body
			   FROM idempotency_keys WHERE key = $1`, in.Key).Scan(&hash, &status, &body)
		if err != nil {
			return Result{}, err
		}
		if hash != in.RequestHash {
			return Result{}, ErrKeyReuse
		}
		return Result{Status: status, Body: body}, nil
	}

	// 2. lock both accounts, always in id order
	rows, err := tx.Query(ctx,
		`SELECT id, balance_minor, status FROM accounts
		  WHERE id = ANY($1) ORDER BY id FOR UPDATE`, []int64{in.Source, in.Dest})
	if err != nil {
		return Result{}, err
	}
	type acct struct {
		balance int64
		status  string
	}
	accts := map[int64]acct{}
	for rows.Next() {
		var id int64
		var a acct
		if err := rows.Scan(&id, &a.balance, &a.status); err != nil {
			return Result{}, err
		}
		accts[id] = a
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Result{}, err
	}

	// 3. validate
	src, okS := accts[in.Source]
	dst, okD := accts[in.Dest]
	if !okS || !okD {
		return Result{}, ErrAccountNotFound
	}
	if src.status != "ACTIVE" || dst.status != "ACTIVE" {
		return Result{}, ErrAccountFrozen
	}
	if src.balance < in.AmountMinor {
		return Result{}, ErrInsufficientFunds
	}

	// 4. move the money and write the ledger
	// TODO: UPDATE accounts SET balance_minor = balance_minor - $1 WHERE id = $2 (source)
	// TODO: UPDATE accounts SET balance_minor = balance_minor + $1 WHERE id = $2 (dest)
	// TODO: INSERT INTO transfers (...) VALUES (
	//         'TXN-' || to_char(now(),'YYYYMMDD') || '-' || lpad(nextval('transfer_ref_seq')::text, 6, '0'),
	//         ...) RETURNING id, reference
	// TODO: INSERT two ledger_entries rows: DEBIT source, CREDIT dest

	// 5. save the response against the key, then commit
	// TODO: build body JSON {reference,status,amount}; UPDATE idempotency_keys SET response_status=201, response_body=$2 WHERE key=$1
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{ /* Status: 201, Body: ... */ }, nil
}
