package argon2x

import "testing"

// TestParams is the regression guard from docs/04-AUTH.md §6 checklist: a silent
// downgrade of Argon2id parameters is a security incident, so pin them here.
func TestParams(t *testing.T) {
	if Params.Memory != 64*1024 {
		t.Errorf("Memory = %d, want %d", Params.Memory, 64*1024)
	}
	if Params.Iterations != 3 {
		t.Errorf("Iterations = %d, want 3", Params.Iterations)
	}
	if Params.Parallelism != 2 {
		t.Errorf("Parallelism = %d, want 2", Params.Parallelism)
	}
	if Params.SaltLength != 16 {
		t.Errorf("SaltLength = %d, want 16", Params.SaltLength)
	}
	if Params.KeyLength != 32 {
		t.Errorf("KeyLength = %d, want 32", Params.KeyLength)
	}
}

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := Verify("correct horse battery staple", h)
	if err != nil || !ok {
		t.Fatalf("verify good password: ok=%v err=%v", ok, err)
	}
	ok, _ = Verify("wrong", h)
	if ok {
		t.Fatal("verify wrong password returned true")
	}
}

func TestCheckPolicy(t *testing.T) {
	cases := []struct {
		name, pw, email string
		wantErr         bool
	}{
		{"ok", "a-strong-passphrase-123", "user@example.com", false},
		{"too short", "short", "user@example.com", true},
		{"breached", "password123", "user@example.com", true},
		{"equals local part", "user", "user@example.com", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckPolicy(c.pw, c.email)
			if (err != nil) != c.wantErr {
				t.Fatalf("CheckPolicy(%q) err=%v wantErr=%v", c.pw, err, c.wantErr)
			}
			if err != nil && !IsPolicyError(err) {
				t.Fatalf("expected PolicyError, got %T", err)
			}
		})
	}
}
