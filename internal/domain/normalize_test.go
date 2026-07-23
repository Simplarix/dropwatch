package domain

import "testing"

func TestNormalizeName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		unicode string
		ascii   string
	}{
		{"https://Example.COM/path?q=1", "example.com", "example.com"},
		{"BÜCHER.de.", "bücher.de", "xn--bcher-kva.de"},
		{"  sub.example.com.  ", "sub.example.com", "sub.example.com"},
	}
	for _, test := range tests {
		result, err := NormalizeName(test.input)
		if err != nil {
			t.Fatalf("NormalizeName(%q): %v", test.input, err)
		}
		if result.Unicode != test.unicode || result.ASCII != test.ascii {
			t.Errorf("NormalizeName(%q) = %#v, want unicode=%q ascii=%q", test.input, result, test.unicode, test.ascii)
		}
	}
}

func TestNormalizeNameRejectsUnsafeValues(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", "localhost", "127.0.0.1", "bad_label.example"} {
		if _, err := NormalizeName(input); err == nil {
			t.Errorf("NormalizeName(%q) unexpectedly succeeded", input)
		}
	}
}
