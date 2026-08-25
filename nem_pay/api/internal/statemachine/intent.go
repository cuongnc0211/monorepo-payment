// Package statemachine is the ONE place payment-intent transitions are defined. Every handler
// and service asks CanTransition rather than checking status inline — so the legal edges live
// in a single map and can't drift. M3 adds escrow by APPENDING edges here, never by scattering
// new checks across handlers (escrow-adaptability guard #4).
package statemachine

// Settlement modes (mirror the payment_intents.settlement_mode CHECK).
const (
	SettlementDirect = "direct"
	SettlementEscrow = "escrow" // M3: hold captured funds for a third-party payee
)

// Intent statuses (mirror the payment_intents.status CHECK).
const (
	StatusCreated              = "created"
	StatusRequiresConfirmation = "requires_confirmation"
	StatusAuthorized           = "authorized"
	StatusCaptured             = "captured"
	StatusSettled              = "settled" // direct: cash in hand
	StatusHeldInEscrow         = "held_in_escrow" // escrow: funds settled into segregation, held for the payee
	StatusReleased             = "released"        // escrow: released to the payee minus the fee (terminal)
	StatusFailed               = "failed"
	StatusRefunded             = "refunded"
	StatusPartiallyRefunded    = "partially_refunded"
)

// allowedEdges maps a settlement mode to its status → next-statuses graph. Keeping it mode-aware
// (rather than one shared graph) is deliberate: the two modes genuinely diverge after capture, and
// keying by mode makes an illegal cross-mode edge (e.g. a direct intent → held_in_escrow, or an
// escrow intent → settled) structurally impossible instead of a convention.
//
//	direct:  created → requires_confirmation → authorized → captured → settled
//	escrow:  created → requires_confirmation → authorized → captured → held_in_escrow → released
//	both:    created | requires_confirmation | authorized → failed   (decline / expiry sweep)
//	direct:  captured | settled → refunded | partially_refunded
//	escrow:  held_in_escrow → refunded   (full refund from escrow; released is terminal)
//
// The pre-capture → failed edges are a conscious refinement of the constitution (which draws only
// `authorized → failed`) so a decline/expiry lands in `failed` without a wrong hop through
// `authorized`.
var allowedEdges = map[string]map[string][]string{
	SettlementDirect: {
		StatusCreated:              {StatusRequiresConfirmation, StatusFailed},
		StatusRequiresConfirmation: {StatusAuthorized, StatusFailed},
		StatusAuthorized:           {StatusCaptured, StatusFailed},
		StatusCaptured:             {StatusSettled, StatusRefunded, StatusPartiallyRefunded},
		StatusSettled:              {StatusRefunded, StatusPartiallyRefunded},
		// terminal: failed, refunded, partially_refunded
	},
	SettlementEscrow: {
		StatusCreated:              {StatusRequiresConfirmation, StatusFailed},
		StatusRequiresConfirmation: {StatusAuthorized, StatusFailed},
		StatusAuthorized:           {StatusCaptured, StatusFailed},
		StatusCaptured:             {StatusHeldInEscrow},
		StatusHeldInEscrow:         {StatusReleased, StatusRefunded},
		// terminal: failed, released, refunded
	},
}

// CanTransition reports whether from → to is a legal edge for the given settlement mode.
func CanTransition(mode, from, to string) bool {
	for _, next := range allowedEdges[mode][from] {
		if next == to {
			return true
		}
	}
	return false
}
