-- Reverse of 0001_ledger. Drop in dependency order: entries references transactions
-- and accounts, so it must go first. Safe to run while no money flow depends on the
-- ledger yet (nothing has been posted). Once captures exist, the ledger is append-only
-- and you unwind money with reversing entries, never by dropping tables.
DROP TABLE IF EXISTS entries;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS accounts;
