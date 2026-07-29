-- 0002_idempotency — insert-first idempotency for every money-mutating POST.
--
-- The race is arbitrated by the DB, not by application code: a caller INSERTs a claim row
-- and the UNIQUE(merchant_id, idem_key) constraint decides who is first. Two concurrent
-- retries can never both proceed, because the loser's INSERT fails on the constraint rather
-- than on a SELECT that raced. This is why there is deliberately no "check then insert" path.
CREATE TABLE idempotency_keys (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id   uuid NOT NULL,
  idem_key      text NOT NULL,
  request_hash  text NOT NULL,                                  -- sha256(method + path + body)
  status        text NOT NULL CHECK (status IN ('in_flight','completed')),
  response_code int,                                            -- stored only once completed
  response_body bytea,                                          -- exact response bytes, replayed verbatim
  -- NB: bytea, NOT jsonb — jsonb re-serialises (reorders keys, adds spaces), which would make
  -- replay differ byte-for-byte from the original and mangle any non-canonical/non-JSON body.
  -- Idempotent replay must return the ORIGINAL bytes, so we store them raw.
  locked_at     timestamptz NOT NULL DEFAULT now(),             -- for the orphan sweeper
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (merchant_id, idem_key)
);

-- The sweeper frees rows wedged 'in_flight' by a crash mid-request; index the columns it scans.
CREATE INDEX idempotency_sweep_idx ON idempotency_keys (status, locked_at);
