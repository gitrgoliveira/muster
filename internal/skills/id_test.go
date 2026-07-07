package skills

import "testing"

func TestValidateID(t *testing.T) {
	valid := []string{"repo-grep", "run-tests", "speckit", "a", "a.b", "a_b-1", "beads-memory"}
	for _, id := range valid {
		if err := ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) = %v, want nil", id, err)
		}
	}

	invalid := []string{
		"",            // empty
		".",           // dot
		"..",          // parent
		"../etc",      // traversal
		"../../etc/x", // deep traversal
		"a/b",         // separator
		`a\b`,         // windows separator
		"a..b",        // embedded ..
		"/abs",        // leading separator
		".hidden",     // leading dot
		"UPPER",       // uppercase
		"has space",   // space
		"skill:x",     // colon (the prefix must already be stripped)
	}
	for _, id := range invalid {
		if err := ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) = nil, want error", id)
		}
	}
}

func TestValidateID_TooLong(t *testing.T) {
	long := make([]byte, maxIDLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateID(string(long)); err == nil {
		t.Fatal("expected error for over-length id")
	}
}
