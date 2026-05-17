package placeholder

import "testing"

func TestPattern_MatchesAndRejects(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"STRIPE_KEY_1", true},
		{"STRIPE_KEY_10", true},
		{"AWS_ACCESS_KEY_ID_2", true},
		{"GITHUB_TOKEN_42", true},
		{"JWT_1", true},

		{"stripe_key_1", false}, // lowercase prefix
		{"STRIPE_KEY", false},   // no counter
		{"STRIPE_KEY_", false},  // counter missing digits
		{"_KEY_1", false},       // prefix must start with [A-Z]
		{"1_KEY_1", false},      // prefix must start with [A-Z]
		{"", false},
	}
	re := Pattern()
	for _, tc := range cases {
		// Whole-input match: the regex must consume every byte. Using
		// FindString alone would return "" for both "no match" and a
		// successful empty match — distinguish via FindStringIndex.
		loc := re.FindStringIndex(tc.input)
		got := loc != nil && loc[0] == 0 && loc[1] == len(tc.input)
		if got != tc.want {
			t.Errorf("Pattern.match(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestPattern_LongestInLine(t *testing.T) {
	// FindAllString must return the leftmost-longest match — this is
	// the property the rehydrate path relies on so STRIPE_KEY_10 beats
	// STRIPE_KEY_1 when both appear.
	got := Pattern().FindAllString("prefix STRIPE_KEY_10 mid STRIPE_KEY_1 end", -1)
	if len(got) != 2 || got[0] != "STRIPE_KEY_10" || got[1] != "STRIPE_KEY_1" {
		t.Errorf("FindAllString returned %v; want [STRIPE_KEY_10 STRIPE_KEY_1]", got)
	}
}
