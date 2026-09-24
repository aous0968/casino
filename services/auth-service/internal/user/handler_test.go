package user

import "testing"

func TestValidate(t *testing.T) {
	cases := []struct {
		name        string
		email       string
		password    string
		wantInvalid []string // field names expected to fail
	}{
		{"valid", "alice@example.com", "hunter2hunter2", nil},
		{"missing email", "", "hunter2hunter2", []string{"email"}},
		{"bad email", "not-an-email", "hunter2hunter2", []string{"email"}},
		{"missing password", "alice@example.com", "", []string{"password"}},
		{"short password", "alice@example.com", "short", []string{"password"}},
		{"too long password", "alice@example.com", string(make([]byte, 200)), []string{"password"}},
		{"both bad", "bad", "x", []string{"email", "password"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validate(registerRequest{Email: c.email, Password: c.password})
			if len(got) != len(c.wantInvalid) {
				t.Fatalf("got %d invalid fields, want %d (%v)", len(got), len(c.wantInvalid), got)
			}
			for _, f := range c.wantInvalid {
				if _, ok := got[f]; !ok {
					t.Fatalf("expected field %q to be invalid, got %v", f, got)
				}
			}
		})
	}
}