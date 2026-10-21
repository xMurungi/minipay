-- +goose Up
CREATE TABLE accounts (
  id            BIGSERIAL PRIMARY KEY,
  name          TEXT    NOT NULL,
  is_system     BOOLEAN NOT NULL DEFAULT false,
  balance_minor BIGINT  NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT non_negative CHECK (is_system OR balance_minor >= 0)
);

CREATE SEQUENCE transfer_ref_seq;

CREATE TABLE transfers (
  id                BIGSERIAL PRIMARY KEY,
  reference         TEXT   NOT NULL UNIQUE,
  source_account_id BIGINT NOT NULL REFERENCES accounts(id),
  dest_account_id   BIGINT NOT NULL REFERENCES accounts(id),
  amount_minor      BIGINT NOT NULL CHECK (amount_minor > 0),
  status            TEXT   NOT NULL,
  reverses_id       BIGINT REFERENCES transfers(id),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (source_account_id <> dest_account_id)
);
CREATE UNIQUE INDEX one_reversal_per_transfer ON transfers(reverses_id) WHERE reverses_id IS NOT NULL;

CREATE TABLE ledger_entries (
  id           BIGSERIAL PRIMARY KEY,
  transfer_id  BIGINT NOT NULL REFERENCES transfers(id),
  account_id   BIGINT NOT NULL REFERENCES accounts(id),
  direction    TEXT   NOT NULL CHECK (direction IN ('DEBIT','CREDIT')),
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0)
);
CREATE INDEX ledger_by_account ON ledger_entries(account_id, id);

CREATE TABLE idempotency_keys (
  key          TEXT PRIMARY KEY,
  request_hash TEXT NOT NULL,
  transfer_id  BIGINT REFERENCES transfers(id),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- the system "float" account; it must be id 1
INSERT INTO accounts (name, is_system) VALUES ('SYSTEM', true);

-- +goose Down
DROP TABLE idempotency_keys;
DROP TABLE ledger_entries;
DROP TABLE transfers;
DROP SEQUENCE transfer_ref_seq;
DROP TABLE accounts;