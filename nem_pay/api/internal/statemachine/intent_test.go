package statemachine

import "testing"

func TestCanTransition_LegalEdges(t *testing.T) {
	legal := [][2]string{
		{StatusCreated, StatusRequiresConfirmation},
		{StatusRequiresConfirmation, StatusAuthorized},
		{StatusAuthorized, StatusCaptured},
		{StatusAuthorized, StatusFailed},
		{StatusCaptured, StatusSettled},
		{StatusCaptured, StatusRefunded},
		{StatusCaptured, StatusPartiallyRefunded},
		{StatusSettled, StatusRefunded},
		{StatusSettled, StatusPartiallyRefunded},
		{StatusCreated, StatusFailed},              // expiry of a brand-new intent
		{StatusRequiresConfirmation, StatusFailed}, // declined authorization / expiry
		{StatusAuthorized, StatusFailed},           // expiry of an authorized intent
	}
	for _, e := range legal {
		if !CanTransition(e[0], e[1]) {
			t.Errorf("expected legal edge %s → %s", e[0], e[1])
		}
	}
}

func TestCanTransition_IllegalEdges(t *testing.T) {
	illegal := [][2]string{
		{StatusCreated, StatusCaptured},      // can't skip confirm/authorize
		{StatusCreated, StatusAuthorized},    // must pass through requires_confirmation
		{StatusAuthorized, StatusSettled},    // must capture first
		{StatusFailed, StatusAuthorized},     // failed is terminal
		{StatusRefunded, StatusCaptured},     // refunded is terminal
		{StatusCaptured, StatusAuthorized},   // no going back
		{"nonsense", StatusCaptured},         // unknown source
	}
	for _, e := range illegal {
		if CanTransition(e[0], e[1]) {
			t.Errorf("expected illegal edge %s → %s to be rejected", e[0], e[1])
		}
	}
}
