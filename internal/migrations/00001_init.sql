-- +goose Up
CREATE TABLE customers (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT        NOT NULL,
  phone       TEXT        NOT NULL UNIQUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
  id            BIGSERIAL PRIMARY KEY,
  owner_id      BIGINT REFERENCES customers(id),    -- NULL for system accounts
  type          TEXT   NOT NULL CHECK (type IN ('WALLET','BANK','SYSTEM')),
  status        TEXT   NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','FROZEN')),
  balance_minor BIGINT NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- customer accounts can never go negative; the system float account can
  CONSTRAINT non_negative CHECK (type = 'SYSTEM' OR balance_minor >= 0)
);

CREATE SEQUENCE transfer_ref_seq;

CREATE TABLE transfers (
  id                  BIGSERIAL PRIMARY KEY,
  reference           TEXT   NOT NULL UNIQUE,
  source_account_id   BIGINT NOT NULL REFERENCES accounts(id),
  dest_account_id     BIGINT NOT NULL REFERENCES accounts(id),
  amount_minor        BIGINT NOT NULL CHECK (amount_minor > 0),
  channel             TEXT   NOT NULL,
  status              TEXT   NOT NULL CHECK (status IN ('COMPLETED','REVERSED','FAILED')),
  reverses_id         BIGINT REFERENCES transfers(id),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (source_account_id <> dest_account_id)
);
-- a transfer can be reversed at most once
CREATE UNIQUE INDEX one_reversal_per_transfer ON transfers(reverses_id) WHERE reverses_id IS NOT NULL;

CREATE TABLE ledger_entries (
  id            BIGSERIAL PRIMARY KEY,
  transfer_id   BIGINT NOT NULL REFERENCES transfers(id),
  account_id    BIGINT NOT NULL REFERENCES accounts(id),
  direction     TEXT   NOT NULL CHECK (direction IN ('DEBIT','CREDIT')),
  amount_minor  BIGINT NOT NULL CHECK (amount_minor > 0),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ledger_by_account ON ledger_entries(account_id, id);

CREATE TABLE idempotency_keys (
  key              TEXT PRIMARY KEY,
  request_hash     TEXT NOT NULL,
  response_status  INT,
  response_body    JSONB,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- the system float account used for deposits (id 1)
INSERT INTO accounts (type, balance_minor) VALUES ('SYSTEM', 0);

-- +goose Down
DROP TABLE idempotency_keys, ledger_entries, transfers, accounts, customers;
DROP SEQUENCE transfer_ref_seq;