-- 0006_escrow — widen two CHECK constraints so payment intents can run in escrow mode.
--
-- Additive by design (escrow-adaptability guard, M1): the settlement_mode/payee_id/application_fee
-- columns already exist; escrow only needs the allowed VALUE sets widened. Inline column CHECKs are
-- auto-named <table>_<column>_check, so we drop and re-add them.
ALTER TABLE payment_intents DROP CONSTRAINT IF EXISTS payment_intents_status_check;
ALTER TABLE payment_intents ADD CONSTRAINT payment_intents_status_check
  CHECK (status IN ('created','requires_confirmation','authorized','captured','settled',
                    'held_in_escrow','released','failed','refunded','partially_refunded'));

ALTER TABLE payment_intents DROP CONSTRAINT IF EXISTS payment_intents_settlement_mode_check;
ALTER TABLE payment_intents ADD CONSTRAINT payment_intents_settlement_mode_check
  CHECK (settlement_mode IN ('direct','escrow'));
