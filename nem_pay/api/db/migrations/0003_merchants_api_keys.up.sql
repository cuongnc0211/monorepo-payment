-- 0003_merchants_api_keys — who a request is for, and how it authenticates.
--
-- Two kinds of key: 'publishable' (safe for the browser / tokenization SDK) and 'secret'
-- (server-to-server; required for every money-mutating call). We store only a HASH of the
-- token, never the raw secret — a leaked table must not hand out working keys. token_prefix
-- is an indexed, non-secret slice of the token used to narrow the lookup before the hash
-- comparison.
CREATE TABLE merchants (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name       text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id  uuid NOT NULL REFERENCES merchants(id),
  kind         text NOT NULL CHECK (kind IN ('publishable','secret')),
  token_prefix text NOT NULL,          -- e.g. 'pk_test_' / 'sk_test_' — indexed lookup, not secret
  token_hash   text NOT NULL,          -- sha256(raw token); the raw token is never stored
  created_at   timestamptz NOT NULL DEFAULT now(),
  revoked_at   timestamptz
);

CREATE INDEX api_keys_lookup ON api_keys (token_prefix);
