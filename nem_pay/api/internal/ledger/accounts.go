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

// Account kinds. Direct-mode kinds date from M1; the escrow kinds are new string constants only —
// no schema change, because accounts are already typed (asset/liability/revenue) and per-reference.
const (
	// Direct mode (M1).
	KindPlatformCash       = "platform_cash"       // asset  — cash NemPay holds
	KindAcquirerReceivable = "acquirer_receivable" // asset  — captured, not yet settled
	KindMerchantPayable    = "merchant_payable"    // liability — owed to the merchant (direct)

	// Escrow mode (M3).
	KindSegregatedCash  = "segregated_cash"  // asset     — customer funds held segregated, not commingled
	KindEscrowLiability = "escrow_liability" // liability  — held on behalf of the payee (per intent)
	KindPayableToPayee  = "payable_to_payee" // liability  — accrued to the payee on release (per payee)
	KindPlatformRevenue = "platform_revenue" // revenue    — the application fee earned on release
)
