-- 0007_portal_users — human login identities for the read-only merchant portal (spec 005).
--
-- A user belongs to exactly ONE merchant: single-tenant-per-user for the first cut (no roles, no
-- teams, no invitations — those are a later lesson). Only a bcrypt hash of the password is stored,
-- never plaintext. Email is the unique login handle across the table.
CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id   uuid NOT NULL REFERENCES merchants(id),
  email         text NOT NULL UNIQUE,
  password_hash text NOT NULL,          -- bcrypt(password); the raw password is never stored
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_merchant_idx ON users (merchant_id);
