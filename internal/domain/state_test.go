package domain

import (
	"reflect"
	"testing"
)

func TestStateFromRDAP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		statuses []string
		found    bool
		want     State
	}{
		{"registered", []string{"active"}, true, StateRegistered},
		{"client hold is registered", []string{"clientHold"}, true, StateClientHold},
		{"server hold", []string{"server hold"}, true, StateServerHold},
		{"redemption", []string{"redemptionPeriod"}, true, StateRedemptionPeriod},
		{"pending delete wins", []string{"clientHold", "pendingDelete"}, true, StatePendingDelete},
		{"404 is only possible", nil, false, StatePossiblyAvailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := StateFromRDAP(test.statuses, test.found); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestNormalizeStatuses(t *testing.T) {
	t.Parallel()
	got := NormalizeStatuses([]string{"clientHold", "active", "client hold", "active"})
	want := []string{"active", "client_hold"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
