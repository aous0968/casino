CREATE SCHEMA IF NOT EXISTS wallet;

CREATE TABLE wallet.accounts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type TEXT NOT NULL,
    owner_id   TEXT NOT NULL,
    currency   TEXT NOT NULL DEFAULT 'USD',
    balance    BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT accounts_owner_type_check CHECK (owner_type IN ('user', 'system')),
    CONSTRAINT accounts_currency_check   CHECK (currency IN ('USD')),
    CONSTRAINT accounts_unique_owner     UNIQUE (owner_type, owner_id, currency),
    CONSTRAINT user_balance_non_negative CHECK (owner_type = 'system' OR balance >= 0)
);

CREATE TABLE wallet.transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            TEXT NOT NULL,
    idempotency_key TEXT UNIQUE,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT transactions_kind_check CHECK (kind IN (
        'deposit', 'withdrawal', 'bet', 'win', 'bonus', 'refund', 'adjustment'
    ))
);

CREATE INDEX idx_transactions_created_at ON wallet.transactions (created_at DESC);

CREATE TABLE wallet.ledger_entries (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES wallet.transactions(id) ON DELETE RESTRICT,
    account_id     UUID NOT NULL REFERENCES wallet.accounts(id) ON DELETE RESTRICT,
    amount         BIGINT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT ledger_amount_nonzero CHECK (amount <> 0)
);

CREATE INDEX idx_ledger_account_created ON wallet.ledger_entries (account_id, created_at DESC);
CREATE INDEX idx_ledger_transaction     ON wallet.ledger_entries (transaction_id);

INSERT INTO wallet.accounts (owner_type, owner_id, currency)
VALUES
  ('system', 'external', 'USD'),
  ('system', 'house',    'USD');