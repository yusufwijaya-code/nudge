package jira_account_holder

import "testing"

func TestCurrent_UsesTheEnvPairUntilAnAccountIsSaved(t *testing.T) {
	SetOverride(nil)
	t.Cleanup(func() { SetOverride(nil) })

	if email, token := Current("env@esb.co.id", "env-token"); email != "env@esb.co.id" || token != "env-token" {
		t.Errorf("Current = %q / %q, want env", email, token)
	}
	SetOverride(&Credential{Email: "bot@esb.co.id", APIToken: "saved-token"})
	if email, token := Current("env@esb.co.id", "env-token"); email != "bot@esb.co.id" || token != "saved-token" {
		t.Errorf("Current = %q / %q, want the saved account", email, token)
	}
	SetOverride(nil)
	if email, _ := Current("env@esb.co.id", "env-token"); email != "env@esb.co.id" {
		t.Errorf("Current = %q, want env after clearing", email)
	}
}

// Override hands out a copy so a caller cannot change the credential in force.
func TestOverride_ReturnsACopy(t *testing.T) {
	SetOverride(&Credential{Email: "bot@esb.co.id", APIToken: "saved-token"})
	t.Cleanup(func() { SetOverride(nil) })

	got := Override()
	got.APIToken = "tampered"
	if _, token := Current("e", "t"); token != "saved-token" {
		t.Errorf("token in force = %q after mutating the copy", token)
	}
	SetOverride(nil)
	if Override() != nil {
		t.Error("Override must be nil once cleared")
	}
}
