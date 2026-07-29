// Package ledger is NemPay's double-entry ledger — the source of truth for all money.
//
// Balance convention (locked here, relied on by every posting in M1.4/M3): each entry has
// two non-negative columns, debit and credit, exactly one non-zero. A transaction is
// balanced when Σdebit = Σcredit. An account's balance is Σdebit − Σcredit (debit-normal),
// so ASSET accounts carry a positive balance and LIABILITY/REVENUE accounts a negative one.
// If a future posting hits the wrong side the ledger skews silently — the golden balance
// tests exist to catch exactly that.
package ledger

// Account types — the accounting classification, mirrored by the accounts.type CHECK.
const (
	TypeAsset     = "asset"
	TypeLiability = "liability"
	TypeRevenue   = "revenue"
)

// Account kinds in use for M1 (direct mode only). Escrow kinds
// (escrow_liability, payable_to_payee, platform_revenue, refund_payable) arrive in M3 as
// new constants — no schema change, because accounts are already typed and per-reference.
const (
	KindPlatformCash       = "platform_cash"       // asset  — cash NemPay holds
	KindAcquirerReceivable = "acquirer_receivable" // asset  — captured, not yet settled
	KindMerchantPayable    = "merchant_payable"    // liability — owed to the merchant (direct)
)
