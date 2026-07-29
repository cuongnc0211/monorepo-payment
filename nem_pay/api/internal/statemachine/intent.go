// Package statemachine is the ONE place payment-intent transitions are defined. Every handler
// and service asks CanTransition rather than checking status inline — so the legal edges live
// in a single map and can't drift. M3 adds escrow by APPENDING edges here, never by scattering
// new checks across handlers (escrow-adaptability guard #4).
package statemachine

// Settlement modes (mirror the payment_intents.settlement_mode CHECK). M1 is direct only;
// M3 adds SettlementEscrow.
const (
	SettlementDirect = "direct"
)

// Intent statuses (mirror the payment_intents.status CHECK).
const (
	StatusCreated              = "created"
	StatusRequiresConfirmation = "requires_confirmation"
	StatusAuthorized           = "authorized"
	StatusCaptured             = "captured"
	StatusSettled              = "settled"
	StatusFailed               = "failed"
	StatusRefunded             = "refunded"
	StatusPartiallyRefunded    = "partially_refunded"
)

// allowedEdges maps a status to the statuses it may transition to (direct mode, M1).
//
//	created → requires_confirmation → authorized → captured → settled
//	created | requires_confirmation | authorized → failed
//	captured | settled → refunded | partially_refunded
//
// The → failed edges cover two real outcomes: a DECLINED authorization fails a pre-capture
// intent (requires_confirmation → failed), and the expiry sweep fails intents that sit too long
// in any pre-capture state (created/requires_confirmation/authorized → failed). The constitution
// (nem_pay/CLAUDE.md) draws only `authorized → failed`; the two extra pre-capture fail edges are
// a conscious refinement so a decline/expiry lands in `failed` without a semantically-wrong hop
// through `authorized`. M3 appends the escrow edges (captured → held_in_escrow → released,
// held_in_escrow → refunded).
var allowedEdges = map[string][]string{
	StatusCreated:              {StatusRequiresConfirmation, StatusFailed},
	StatusRequiresConfirmation: {StatusAuthorized, StatusFailed},
	StatusAuthorized:           {StatusCaptured, StatusFailed},
	StatusCaptured:             {StatusSettled, StatusRefunded, StatusPartiallyRefunded},
	StatusSettled:              {StatusRefunded, StatusPartiallyRefunded},
	// terminal: failed, refunded, partially_refunded
}

// CanTransition reports whether from → to is a legal direct-mode edge.
func CanTransition(from, to string) bool {
	for _, next := range allowedEdges[from] {
		if next == to {
			return true
		}
	}
	return false
}
