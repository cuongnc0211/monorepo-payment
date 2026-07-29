package ledger

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/repository/db"
)

// newTestDB connects to the Postgres named by TEST_DATABASE_URL (see Makefile `test-db`),
// truncates the ledger for a clean slate, and returns queries + pool. Skips when unset so
// the suite is green on machines without a database.
func newTestDB(t *testing.T) (*db.Queries, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping ledger DB tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE entries, transactions, accounts"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db.New(pool), pool
}

func singleton(t *testing.T, q *db.Queries, m uuid.UUID, typ, kind, cur string) uuid.UUID {
	t.Helper()
	a, err := q.GetOrCreateSingletonAccount(context.Background(), db.GetOrCreateSingletonAccountParams{
		MerchantID: m, Type: typ, Kind: kind, Currency: cur,
	})
	if err != nil {
		t.Fatalf("GetOrCreateSingletonAccount(%s): %v", kind, err)
	}
	return a.ID
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// A balanced capture-shaped posting yields the expected derived balances:
// asset positive (debit-normal), liability negative.
func TestPostTransaction_BalancedBalances(t *testing.T) {
	q, pool := newTestDB(t)
	ctx := context.Background()
	m := uuid.New()

	cash := singleton(t, q, m, TypeAsset, KindPlatformCash, "USD")
	payable := singleton(t, q, m, TypeLiability, KindMerchantPayable, "USD")

	const amt = 5000
	if _, err := PostTransaction(ctx, q, m, "capture", nil, []Entry{
		{AccountID: cash, Debit: amt, Currency: "USD"},
		{AccountID: payable, Credit: amt, Currency: "USD"},
	}); err != nil {
		t.Fatalf("PostTransaction: %v", err)
	}

	if got, _ := Balance(ctx, q, cash); got != amt {
		t.Errorf("platform_cash balance = %d, want %d", got, amt)
	}
	if got, _ := Balance(ctx, q, payable); got != -amt {
		t.Errorf("merchant_payable balance = %d, want %d", got, -amt)
	}
	if n := count(t, pool, "transactions"); n != 1 {
		t.Errorf("transactions = %d, want 1", n)
	}
	if n := count(t, pool, "entries"); n != 2 {
		t.Errorf("entries = %d, want 2", n)
	}
}

// Σdebit ≠ Σcredit → ErrUnbalanced, and NOTHING is written.
func TestPostTransaction_Unbalanced(t *testing.T) {
	q, pool := newTestDB(t)
	ctx := context.Background()
	m := uuid.New()
	cash := singleton(t, q, m, TypeAsset, KindPlatformCash, "USD")
	payable := singleton(t, q, m, TypeLiability, KindMerchantPayable, "USD")

	_, err := PostTransaction(ctx, q, m, "capture", nil, []Entry{
		{AccountID: cash, Debit: 5000, Currency: "USD"},
		{AccountID: payable, Credit: 4000, Currency: "USD"},
	})
	if !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("err = %v, want ErrUnbalanced", err)
	}
	if n := count(t, pool, "transactions"); n != 0 {
		t.Errorf("transactions = %d, want 0 (nothing written)", n)
	}
	if n := count(t, pool, "entries"); n != 0 {
		t.Errorf("entries = %d, want 0 (nothing written)", n)
	}
}

// The app-layer guard rejects malformed legs before touching the DB. (The DB CHECK is a
// second line of defence, proven separately in the migration test.)
func TestPostTransaction_BadEntry(t *testing.T) {
	q, _ := newTestDB(t)
	ctx := context.Background()
	m := uuid.New()
	acc := singleton(t, q, m, TypeAsset, KindPlatformCash, "USD")

	cases := map[string]Entry{
		"both zero":     {AccountID: acc, Debit: 0, Credit: 0, Currency: "USD"},
		"both non-zero": {AccountID: acc, Debit: 10, Credit: 10, Currency: "USD"},
		"negative":      {AccountID: acc, Debit: -1, Currency: "USD"},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := PostTransaction(ctx, q, m, "capture", nil, []Entry{e})
			if !errors.Is(err, ErrBadEntry) {
				t.Fatalf("err = %v, want ErrBadEntry", err)
			}
		})
	}
}

func TestPostTransaction_MixedCurrency(t *testing.T) {
	q, _ := newTestDB(t)
	ctx := context.Background()
	m := uuid.New()
	cash := singleton(t, q, m, TypeAsset, KindPlatformCash, "USD")
	payable := singleton(t, q, m, TypeLiability, KindMerchantPayable, "USD")

	_, err := PostTransaction(ctx, q, m, "capture", nil, []Entry{
		{AccountID: cash, Debit: 5000, Currency: "USD"},
		{AccountID: payable, Credit: 5000, Currency: "EUR"},
	})
	if !errors.Is(err, ErrMixedCurrency) {
		t.Fatalf("err = %v, want ErrMixedCurrency", err)
	}
}

// GetOrCreateAccount returns the same row on repeat calls (idempotent).
func TestGetOrCreateAccount_Idempotent(t *testing.T) {
	q, pool := newTestDB(t)
	m := uuid.New()
	first := singleton(t, q, m, TypeAsset, KindPlatformCash, "USD")
	second := singleton(t, q, m, TypeAsset, KindPlatformCash, "USD")
	if first != second {
		t.Errorf("ids differ: %s != %s", first, second)
	}
	if n := count(t, pool, "accounts"); n != 1 {
		t.Errorf("accounts = %d, want 1", n)
	}
}

// Under concurrency, the partial unique index arbitrates: many racers, exactly one row,
// all callers observe the same id. (Insert-first, never SELECT-then-INSERT.)
func TestGetOrCreateAccount_ConcurrentRace(t *testing.T) {
	q, pool := newTestDB(t)
	m := uuid.New()

	const racers = 16
	ids := make([]uuid.UUID, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a, err := q.GetOrCreateSingletonAccount(context.Background(), db.GetOrCreateSingletonAccountParams{
				MerchantID: m, Type: TypeAsset, Kind: KindPlatformCash, Currency: "USD",
			})
			if err != nil {
				t.Errorf("racer %d: %v", i, err)
				return
			}
			ids[i] = a.ID
		}(i)
	}
	wg.Wait()

	if n := count(t, pool, "accounts"); n != 1 {
		t.Fatalf("accounts = %d, want exactly 1", n)
	}
	for i, id := range ids {
		if id != ids[0] {
			t.Errorf("racer %d saw %s, want %s", i, id, ids[0])
		}
	}
}

// Guard: no ledger table has a stored (mutable) balance column — balances are derived.
func TestNoStoredBalanceColumn(t *testing.T) {
	_, pool := newTestDB(t)
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name IN ('accounts','transactions','entries')
		  AND column_name = 'balance'`).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Errorf("found %d 'balance' column(s); ledger balances must be derived, never stored", n)
	}
}
