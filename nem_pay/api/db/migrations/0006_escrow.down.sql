-- Restore the M1 (direct-only) CHECK sets. Safe only before any escrow rows exist.
ALTER TABLE payment_intents DROP CONSTRAINT IF EXISTS payment_intents_status_check;
ALTER TABLE payment_intents ADD CONSTRAINT payment_intents_status_check
  CHECK (status IN ('created','requires_confirmation','authorized','captured','settled',
                    'failed','refunded','partially_refunded'));

ALTER TABLE payment_intents DROP CONSTRAINT IF EXISTS payment_intents_settlement_mode_check;
ALTER TABLE payment_intents ADD CONSTRAINT payment_intents_settlement_mode_check
  CHECK (settlement_mode IN ('direct'));
