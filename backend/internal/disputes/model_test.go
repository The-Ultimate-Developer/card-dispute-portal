package disputes

import "testing"

func TestStateMachine(t *testing.T) {
	all := []Status{StatusSubmitted, StatusUnderReview, StatusResolved, StatusRejected}
	allowed := map[[2]Status]bool{
		{StatusSubmitted, StatusUnderReview}: true,
		{StatusSubmitted, StatusRejected}:    true,
		{StatusUnderReview, StatusResolved}:  true,
		{StatusUnderReview, StatusRejected}:  true,
	}

	for _, from := range all {
		for _, to := range all {
			want := allowed[[2]Status{from, to}]
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s -> %s: got %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestFinalStatesHaveNoExits(t *testing.T) {
	for _, s := range []Status{StatusResolved, StatusRejected} {
		if !s.Final() || len(transitions[s]) != 0 {
			t.Errorf("%s should be final with no transitions", s)
		}
	}
}

func TestValidation(t *testing.T) {
	if Status("PENDING").Valid() || Reason("BORED").Valid() {
		t.Fatal("unknown values must be invalid")
	}
	if !StatusUnderReview.Valid() || !ReasonDuplicate.Valid() {
		t.Fatal("known values must be valid")
	}
}
