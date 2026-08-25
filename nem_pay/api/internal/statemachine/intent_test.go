package statemachine

import "testing"

type edge struct{ mode, from, to string }

func TestCanTransition_LegalEdges(t *testing.T) {
	legal := []edge{
		// direct (unchanged from M1)
		{SettlementDirect, StatusCreated, StatusRequiresConfirmation},
		{SettlementDirect, StatusRequiresConfirmation, StatusAuthorized},
		{SettlementDirect, StatusAuthorized, StatusCaptured},
		{SettlementDirect, StatusAuthorized, StatusFailed},
		{SettlementDirect, StatusCaptured, StatusSettled},
		{SettlementDirect, StatusCaptured, StatusRefunded},
		{SettlementDirect, StatusCaptured, StatusPartiallyRefunded},
		{SettlementDirect, StatusSettled, StatusRefunded},
		{SettlementDirect, StatusCreated, StatusFailed},
		{SettlementDirect, StatusRequiresConfirmation, StatusFailed},
		// escrow (M3)
		{SettlementEscrow, StatusCreated, StatusRequiresConfirmation},
		{SettlementEscrow, StatusRequiresConfirmation, StatusAuthorized},
		{SettlementEscrow, StatusAuthorized, StatusCaptured},
		{SettlementEscrow, StatusAuthorized, StatusFailed},
		{SettlementEscrow, StatusCaptured, StatusHeldInEscrow},
		{SettlementEscrow, StatusHeldInEscrow, StatusReleased},
		{SettlementEscrow, StatusHeldInEscrow, StatusRefunded},
	}
	for _, e := range legal {
		if !CanTransition(e.mode, e.from, e.to) {
			t.Errorf("expected legal edge [%s] %s → %s", e.mode, e.from, e.to)
		}
	}
}

func TestCanTransition_IllegalEdges(t *testing.T) {
	illegal := []edge{
		// direct
		{SettlementDirect, StatusCreated, StatusCaptured},
		{SettlementDirect, StatusAuthorized, StatusSettled},
		{SettlementDirect, StatusFailed, StatusAuthorized},
		{SettlementDirect, StatusCaptured, StatusAuthorized},
		{SettlementDirect, "nonsense", StatusCaptured},
		// cross-mode: a direct intent must never enter escrow states, and vice-versa
		{SettlementDirect, StatusCaptured, StatusHeldInEscrow},
		{SettlementEscrow, StatusCaptured, StatusSettled},
		{SettlementEscrow, StatusHeldInEscrow, StatusPartiallyRefunded}, // no partial refund from escrow
		{SettlementEscrow, StatusReleased, StatusRefunded},              // released is terminal
		{"unknown_mode", StatusCreated, StatusRequiresConfirmation},
	}
	for _, e := range illegal {
		if CanTransition(e.mode, e.from, e.to) {
			t.Errorf("expected illegal edge [%s] %s → %s to be rejected", e.mode, e.from, e.to)
		}
	}
}
