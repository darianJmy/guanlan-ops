package store

import "testing"

func TestStoreIsInMemoryOnly(t *testing.T) {
	s := New()
	if err := s.Update(func(st *State) error {
		st.Grants["guanlan:1"] = []string{"service"}
		st.Resources = append(st.Resources, Resource{ID: "service", Name: "Nacos", FolderID: 42})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Grants["guanlan:1"]; len(got) != 1 || got[0] != "service" {
		t.Fatalf("state was not retained: %#v", got)
	}
	if _, ok := s.Snapshot().Resource("service"); !ok {
		t.Fatal("resource was not retained")
	}
	restarted := New()
	if len(restarted.Snapshot().Resources) != 0 {
		t.Fatal("runtime state unexpectedly persisted")
	}
}

func TestExternalFolderACLOverridesGlobalManagerAndIndividualGrants(t *testing.T) {
	s := State{Resources: []Resource{{ID: "grouped", FolderID: 42}, {ID: "root"}}, Grants: map[string][]string{"guanlan:2": {"grouped"}}}
	u := User{ID: "guanlan:2", Enabled: true, Admin: true, FolderAccess: map[uint]string{42: "none"}}
	if s.Allowed(u, "grouped") {
		t.Fatal("global manager or old grant bypassed folder deny")
	}
	if !s.Allowed(u, "root") {
		t.Fatal("ungrouped manager lost access")
	}
	u.Admin = false
	delete(s.Grants, "guanlan:2")
	u.FolderAccess[42] = "view"
	if !s.Allowed(u, "grouped") {
		t.Fatal("folder view was not honored")
	}
	u.ID, u.Admin = "standalone-user", true
	u.FolderAccess = nil
	if s.Allowed(u, "grouped") || s.Allowed(u, "root") {
		t.Fatal("non-观澜 identity retained resource access")
	}
}

func TestLegacyIdentityNeverGrantsAccess(t *testing.T) {
	state := State{Resources: []Resource{{ID: "service"}}, Grants: map[string][]string{"legacy": {"service"}}}
	if state.Allowed(User{ID: "legacy", Admin: true, Enabled: true}, "service") {
		t.Fatal("legacy identity regained access")
	}
}
