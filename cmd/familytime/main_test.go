package main

import "testing"

func TestUnquoteEnvValue(t *testing.T) {
	cases := map[string]string{
		`plainvalue`:     `plainvalue`,
		`"quoted value"`: `quoted value`,
		`'quoted value'`: `quoted value`,
		`"`:              `"`, // lone quote is left alone
		``:               ``,
		`"mismatched'`:   `"mismatched'`,
		`""`:             ``,
	}
	for in, want := range cases {
		if got := unquoteEnvValue(in); got != want {
			t.Errorf("unquoteEnvValue(%q) = %q, want %q", in, got, want)
		}
	}
}
