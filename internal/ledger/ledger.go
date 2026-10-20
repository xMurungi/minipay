package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SystemAccountID int64 = 1

// Each error is defined once, here. Callers match them with errors.Is.
var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrSameAccount       = errors.New("source and destination must differ")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrKeyReuse          = errors.New("idempotency key reused with a different request")
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

type Account struct {
	ID           int64
	Name         string
	BalanceMinor int64
}

type TransferResult struct {
	Reference     string
	SourceBalance int64
	DestBalance   int64
	Replayed      bool
}

func requestHash(src, dst, amount int64) string {
	// sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%d", src, dst, amount)))
	sum := sha256.Sum256(fmt.Appendf(nil, "%d|%d|%d", src, dst, amount))
	return hex.EncodeToString(sum[:])
}

func (s *Service) CreateAccount(ctx context.Context, name string) (Account, error) {
	a := Account{Name: name}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ($1) RETURNING id, balance_minor`, name,
	).Scan(&a.ID, &a.BalanceMinor)
	return a, err
}

func (s *Service) GetAccount(ctx context.Context, id int64) (Account, error) {
	a := Account{ID: id}
	err := s.pool.QueryRow(ctx,
		`SELECT name, balance_minor FROM accounts WHERE id = $1`, id,
	).Scan(&a.Name, &a.BalanceMinor)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	return a, err
}

func (s *Service) Transfer(ctx context.Context, key string, sourceID, destID, amountMinor int64) (TransferResult, error) {
	if sourceID == destID {
		return TransferResult{}, ErrSameAccount
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	defer tx.Rollback(ctx)

	hash := requestHash(sourceID, destID, amountMinor)
	tag, err := tx.Exec(ctx, `
		INSERT INTO idempotency_keys (key, request_hash) VALUES ($1, $2)
		ON CONFLICT (key) DO NOTHING`, key, hash)

	if err != nil {
		return TransferResult{}, err
	}

	if tag.RowsAffected() == 0 {
		// key already used: return the original transfer instead of moving money again
		var savedHash, reference string
		err := tx.QueryRow(ctx, `
			SELECT k.request_hash, t.reference
			FROM idempotency_keys k JOIN transfers t ON t.id = k.transfer_id
			WHERE k.key = $1`, key).Scan(&savedHash, &reference)

		if err != nil {
			return TransferResult{}, err
		}
		if savedHash != hash {
			return TransferResult{}, ErrKeyReuse
		}

		return TransferResult{Reference: reference, Replayed: true}, nil
	}

	rows, err := tx.Query(ctx,
		`SELECT id, balance_minor, is_system FROM accounts
		  WHERE id = ANY($1) ORDER BY id FOR UPDATE`,
		[]int64{sourceID, destID})
	if err != nil {
		return TransferResult{}, err
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
			return TransferResult{}, err
		}
		accts[id] = a
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return TransferResult{}, err
	}

	src, okS := accts[sourceID]
	_, okD := accts[destID]
	if !okS || !okD {
		return TransferResult{}, ErrAccountNotFound
	}
	if !src.system && src.balance < amountMinor {
		return TransferResult{}, ErrInsufficientFunds
	}

	var res TransferResult
	if err := tx.QueryRow(ctx,
		`UPDATE accounts SET balance_minor = balance_minor - $1 WHERE id = $2 RETURNING balance_minor`,
		amountMinor, sourceID).Scan(&res.SourceBalance); err != nil {
		return TransferResult{}, err
	}
	if err := tx.QueryRow(ctx,
		`UPDATE accounts SET balance_minor = balance_minor + $1 WHERE id = $2 RETURNING balance_minor`,
		amountMinor, destID).Scan(&res.DestBalance); err != nil {
		return TransferResult{}, err
	}

	var transferID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO transfers (reference, source_account_id, dest_account_id, amount_minor, status)
		 VALUES ('TXN-' || to_char(now(), 'YYYYMMDD') || '-' || lpad(nextval('transfer_ref_seq')::text, 6, '0'),
		         $1, $2, $3, 'COMPLETED')
		 RETURNING id, reference`,
		sourceID, destID, amountMinor).Scan(&transferID, &res.Reference); err != nil {
		return TransferResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_keys SET transfer_id = $1 WHERE key = $2`,
		transferID, key); err != nil {
		return TransferResult{}, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO ledger_entries (transfer_id, account_id, direction, amount_minor)
		 VALUES ($1, $2, 'DEBIT', $4), ($1, $3, 'CREDIT', $4)`,
		transferID, sourceID, destID, amountMinor); err != nil {
		return TransferResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return TransferResult{}, err
	}
	return res, nil
}
