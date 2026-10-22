package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestDashboardColumnsAlignWithAndWithoutColor(t *testing.T) {
	originalProfile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(originalProfile) })

	states := []domain.State{
		domain.StateRegistered, domain.StatePendingDelete, domain.StatePossiblyAvailable,
		domain.StateClientHold, domain.StateServerHold, domain.StateRedemptionPeriod,
		domain.StateAvailable, domain.StateUnknown, domain.StateError,
	}
	names := []string{"example.com", "例え.example", strings.Repeat("例", 20) + ".example"}
	expires := time.Date(2027, 10, 22, 12, 0, 0, 0, time.UTC)
	last := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	next := time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)
	items := make([]domain.Domain, len(states))
	for index, state := range states {
		items[index] = domain.Domain{
			UnicodeName: names[index%len(names)], State: state,
			ExpiresAt: &expires, LastCheckAt: &last, NextCheckAt: next,
			ConsecutiveErrors: 42,
		}
	}

	for _, profile := range []struct {
		name  string
		value termenv.Profile
	}{
		{"plain", termenv.Ascii},
		{"color", termenv.ANSI256},
	} {
		t.Run(profile.name, func(t *testing.T) {
			lipgloss.SetColorProfile(profile.value)
			// Move the selection through every color and Unicode name variant.
			for selected := range items {
				model := Model{domains: items, selected: selected, location: time.UTC}
				view := model.listView()
				if got := strings.Contains(view, "\x1b["); got != (profile.value != termenv.Ascii) {
					t.Fatalf("unexpected ANSI styling for %s profile", profile.name)
				}
				lines := strings.Split(ansi.Strip(view), "\n")
				starts := []int{1, 27, 48, 66, 84, 102}
				assertColumns(t, lines[2], []string{"DOMAIN", "STATE", "EXPIRES", "LAST CHECK", "NEXT CHECK", "ERRORS"}, starts)
				for index, state := range states {
					assertColumns(t, lines[index+3], []string{
						"", string(state), "2027-10-22 12:00", "2026-10-22 12:00", "2026-10-22 18:00", "42",
					}, starts)
				}
			}
		})
	}
}

func assertColumns(t *testing.T, line string, values []string, starts []int) {
	t.Helper()
	for index, value := range values {
		if value == "" {
			continue
		}
		position := strings.Index(line, value)
		if position < 0 {
			t.Fatalf("missing %q in row %q", value, line)
		}
		if got := lipgloss.Width(line[:position]); got != starts[index] {
			t.Errorf("%q starts at terminal column %d, want %d; row: %q", value, got, starts[index], line)
		}
	}
}

func TestDomainTruncationUsesTerminalCells(t *testing.T) {
	for _, test := range []struct {
		name, value, want string
	}{
		{"short", "example.com", "example.com"},
		{"ascii", strings.Repeat("a", 30), strings.Repeat("a", 23) + "…"},
		{"wide", strings.Repeat("例", 20), strings.Repeat("例", 11) + "…"},
		{"combining", strings.Repeat("e\u0301", 30), strings.Repeat("e\u0301", 23) + "…"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := truncate(test.value, 24); got != test.want {
				t.Errorf("truncate(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
