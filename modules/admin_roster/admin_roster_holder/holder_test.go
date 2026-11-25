package admin_roster_holder

import "testing"

func TestRoster_OwnerIsAlwaysAdminAndPromotedUsersAreToo(t *testing.T) {
	t.Cleanup(func() { SetOwner(""); SetAdmins(nil) })
	SetOwner(" Yusuf.Wijaya@esb.co.id ")
	SetAdmins([]string{"Keva.Galih@esb.co.id", "  "})

	if !IsOwner("yusuf.wijaya@esb.co.id") || !IsAdmin("YUSUF.WIJAYA@ESB.CO.ID") {
		t.Error("the owner is an admin, whatever the letter case")
	}
	if !IsAdmin("keva.galih@esb.co.id") || IsOwner("keva.galih@esb.co.id") || !IsPromoted("keva.galih@esb.co.id") {
		t.Error("a promoted user is an admin but not the owner")
	}
	if IsAdmin("eren.dhoheiri@esb.co.id") || IsAdmin("") {
		t.Error("everyone else is not an admin, and nobody with an empty email")
	}
	SetAdmins(nil)
	if IsAdmin("keva.galih@esb.co.id") {
		t.Error("demoting must take effect at once")
	}
}

func TestRoster_NoOwnerSetMeansNoOwner(t *testing.T) {
	SetOwner("")
	t.Cleanup(func() { SetAdmins(nil) })
	if IsOwner("") || IsAdmin("") {
		t.Error("an unset owner must not match an empty email")
	}
}
