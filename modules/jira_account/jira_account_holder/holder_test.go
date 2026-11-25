package jira_account_holder

import "testing"

func TestCurrent_IsEmptyUntilAnAccountIsSet(t *testing.T) {
	SetOverride(nil)
	t.Cleanup(func() { SetOverride(nil) })

	if email, token, ok := Current(); ok || email != "" || token != "" {
		t.Errorf("Current = %q / %q / %v, want nothing before an account is set", email, token, ok)
	}
	if Configured() {
		t.Error("Configured must be false before an account is set")
	}

	SetOverride(&Credential{Email: "bot@esb.co.id", APIToken: "saved-token"})
	if email, token, ok := Current(); !ok || email != "bot@esb.co.id" || token != "saved-token" {
		t.Errorf("Current = %q / %q / %v, want the saved account", email, token, ok)
	}
	if !Configured() {
		t.Error("Configured must be true once an account is set")
	}

	SetOverride(nil)
	if _, _, ok := Current(); ok {
		t.Error("Current must be empty again after removing the account")
	}
}

// Override hands out a copy so a caller cannot change the credential in force.
func TestOverride_ReturnsACopy(t *testing.T) {
	SetOverride(&Credential{Email: "bot@esb.co.id", APIToken: "saved-token"})
	t.Cleanup(func() { SetOverride(nil) })

	got := Override()
	got.APIToken = "tampered"
	if _, token, _ := Current(); token != "saved-token" {
		t.Errorf("token in force = %q after mutating the copy", token)
	}
	SetOverride(nil)
	if Override() != nil {
		t.Error("Override must be nil once cleared")
	}
}
