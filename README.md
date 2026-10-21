# MiniPay

A simulated payments engine: accounts, a double-entry ledger, safe transfers,
reversals and reconciliation. Go + PostgreSQL. Learning project, not production software.

## Run it

```bash
docker compose up --build
./scripts/smoke.sh
```

API on http://localhost:2323. Requires Docker.

## API

| Endpoint | Purpose |
|---|---|
| `POST /api/accounts` | Open an account |
| `GET /api/accounts/{id}/balance` | Balance |
| `POST /api/accounts/{id}/deposit` | Simulated cash-in (needs `Idempotency-Key`) |
| `POST /api/transfers` | Move money (needs `Idempotency-Key`) |
| `GET /api/transfers/{reference}` | Transfer status |
| `GET /api/accounts/{id}/transactions?limit=&before=` | Statement, newest first |
| `POST /api/transfers/{reference}/reverse` | Reverse a completed transfer |
| `GET /api/admin/reconciliation` | Check balances against the ledger |

Amounts are strings like `"1500.00"`.

## How it works

- **Ledger:** every transfer writes one debit and one credit line. Lines are never edited or deleted.
- **Money:** stored as integer cents. No floats anywhere.
- **Concurrency:** both accounts are locked with `SELECT ... FOR UPDATE` in id order,
  and the balance check happens after the lock.
- **Idempotency:** the `Idempotency-Key` header is saved with a hash of the request.
  Retries return the original result; the same key with a different request returns 409.
- **Reversals:** a new transfer in the opposite direction linked to the original.
  A unique index allows at most one reversal per transfer.
- **Reconciliation:** compares each stored balance to its ledger sum, and checks the whole ledger nets to zero.

## Decisions I made

- (Write in your own words: why a rejected transfer rolls back its idempotency key.)
- (Why the SYSTEM account may go negative.)
- (What happens when someone tries to reverse a transfer the recipient already spent.)

## Not included

Authentication, rate limiting, multiple currencies, real bank integrations,
and anything needed for real money (fraud controls, regulatory compliance).