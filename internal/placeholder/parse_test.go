package placeholder

import "testing"

func TestParse_Roundtrip(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		n      int
	}{
		{"STRIPE_KEY_1", "STRIPE_KEY", 1},
		{"STRIPE_KEY_42", "STRIPE_KEY", 42},
		{"AWS_ACCESS_KEY_ID_7", "AWS_ACCESS_KEY_ID", 7},
		{"JWT_0", "JWT", 0},
	}
	for _, tc := range cases {
		gotPrefix, gotN, err := Parse(tc.name)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.name, err)
			continue
		}
		if gotPrefix != tc.prefix || gotN != tc.n {
			t.Errorf("Parse(%q) = (%q, %d); want (%q, %d)", tc.name, gotPrefix, gotN, tc.prefix, tc.n)
		}
	}
}

func TestParse_Rejects(t *testing.T) {
	bad := []string{
		"",
		"stripe_key_1",  // lowercase
		"STRIPE_KEY",    // no counter
		"_KEY_1",        // bad prefix
		"STRIPE KEY 1",  // spaces
		"prefix STRIPE_KEY_1 suffix", // not whole-input match
	}
	for _, s := range bad {
		if _, _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded; want error", s)
		}
	}
}

func TestValidPrefix(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"STRIPE_KEY", true},
		{"JWT", true},
		{"AWS_ACCESS_KEY_ID", true},
		{"", false},
		{"X", false},        // too short (Pattern needs at least 2 chars before _\d+)
		{"stripe_key", false},
		{"1KEY", false},
		{"_KEY", false},
		{"KEY-1", false},
	}
	for _, tc := range cases {
		if got := ValidPrefix(tc.in); got != tc.want {
			t.Errorf("ValidPrefix(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}
