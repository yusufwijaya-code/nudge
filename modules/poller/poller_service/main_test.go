package poller_service

import (
	"os"
	"testing"

	"nudge/modules/jira_account/jira_account_holder"
)

// These tests drive the loop with a fake Jira: an account is in force, as it is
// once an admin has set one up. The no-account case has its own test.
func TestMain(m *testing.M) {
	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: "bot@esb.co.id", APIToken: "test-token"})
	os.Exit(m.Run())
}
