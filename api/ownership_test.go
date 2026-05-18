// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// userSession returns a sessionUser shaped like requireSessionUser would build
// it, with the given role inside orgID. The org slug/name aren't read by the
// handlers under test, so placeholders are fine.
func userSession(userID, orgID, role string) sessionUser {
	return sessionUser{
		ID: userID,
		Orgs: []orgInfo{
			{ID: orgID, Slug: "org-" + orgID, Name: "Test Org", Role: role},
		},
	}
}

// seedOwnershipFixture stands up two users and a single team org. user1 is an
// admin, user2 is a member. siteOwned is created by user2, siteOrphan has
// NULL creator. Returns the orgID and the two site ids.
func seedOwnershipFixture(t *testing.T, app *application) (orgID, siteOwned, siteOrphan string) {
	t.Helper()
	user1, user2 := "usr_admin", "usr_member"
	orgID = "org_team"
	seedUser(t, app.db, user1, "admin@example.test", "admin", "Admin")
	seedUser(t, app.db, user2, "member@example.test", "member", "Member")
	seedOrg(t, app.db, orgID, "team", "Team", false)
	seedOrgMember(t, app.db, orgID, user1, roleAdmin)
	seedOrgMember(t, app.db, orgID, user2, roleMember)
	creator := user2
	siteOwned = "site_owned"
	siteOrphan = "site_orphan"
	seedSite(t, app.db, siteOwned, orgID, "owned", "Owned Site", &creator)
	seedSite(t, app.db, siteOrphan, orgID, "orphan", "Orphan Site", nil)
	return orgID, siteOwned, siteOrphan
}

func TestRequireSiteMutate_CreatorAllowed(t *testing.T) {
	app := newTestApp(t)
	orgID, siteOwned, _ := seedOwnershipFixture(t, app)
	creator := userSession("usr_member", orgID, roleMember)

	_, role, err := app.requireSiteMutate(context.Background(), creator, siteOwned)
	if err != nil {
		t.Fatalf("creator should be allowed, got %v", err)
	}
	if role != roleMember {
		t.Fatalf("expected role member, got %q", role)
	}
}

func TestRequireSiteMutate_AdminAllowedOnOthersAndOrphans(t *testing.T) {
	app := newTestApp(t)
	orgID, siteOwned, siteOrphan := seedOwnershipFixture(t, app)
	admin := userSession("usr_admin", orgID, roleAdmin)

	if _, _, err := app.requireSiteMutate(context.Background(), admin, siteOwned); err != nil {
		t.Fatalf("admin on teammate site: %v", err)
	}
	if _, _, err := app.requireSiteMutate(context.Background(), admin, siteOrphan); err != nil {
		t.Fatalf("admin on legacy NULL-creator site: %v", err)
	}
}

func TestRequireSiteMutate_MemberOnOthersForbidden(t *testing.T) {
	app := newTestApp(t)
	orgID, siteOwned, siteOrphan := seedOwnershipFixture(t, app)

	// A second member that isn't the creator. Re-seed minimally.
	seedUser(t, app.db, "usr_other", "other@example.test", "other", "Other")
	seedOrgMember(t, app.db, orgID, "usr_other", roleMember)
	other := userSession("usr_other", orgID, roleMember)

	if _, _, err := app.requireSiteMutate(context.Background(), other, siteOwned); !errors.Is(err, ErrSiteMutateForbidden) {
		t.Fatalf("expected ErrSiteMutateForbidden on teammate site, got %v", err)
	}
	if _, _, err := app.requireSiteMutate(context.Background(), other, siteOrphan); !errors.Is(err, ErrSiteMutateForbidden) {
		t.Fatalf("expected ErrSiteMutateForbidden on legacy site, got %v", err)
	}
}

func TestRequireSiteMutate_NonMemberRejected(t *testing.T) {
	app := newTestApp(t)
	_, siteOwned, _ := seedOwnershipFixture(t, app)
	outsider := userSession("usr_outsider", "org_other", roleAdmin)

	_, _, err := app.requireSiteMutate(context.Background(), outsider, siteOwned)
	if err == nil || !strings.Contains(err.Error(), "not a member") {
		t.Fatalf("expected not-a-member error, got %v", err)
	}
}

func TestRequireSiteMutate_MissingSite(t *testing.T) {
	app := newTestApp(t)
	orgID, _, _ := seedOwnershipFixture(t, app)
	admin := userSession("usr_admin", orgID, roleAdmin)

	_, _, err := app.requireSiteMutate(context.Background(), admin, "site_does_not_exist")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected site not found, got %v", err)
	}
}

func TestStatusForSiteMutateError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusOK},
		{"forbidden", ErrSiteMutateForbidden, http.StatusForbidden},
		{"not found", errors.New("site not found"), http.StatusNotFound},
		{"not a member", errors.New("not a member of this organization"), http.StatusForbidden},
	}
	for _, tc := range cases {
		if got := statusForSiteMutateError(tc.err); got != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, got, tc.want)
		}
	}
}

func TestListSites_AuthorFilter(t *testing.T) {
	app := newTestApp(t)
	orgID, _, _ := seedOwnershipFixture(t, app)
	// Add a second site owned by admin so we have two distinct authors.
	adminID := "usr_admin"
	seedSite(t, app.db, "site_admin", orgID, "admin-site", "Admin Site", &adminID)

	all, err := app.listSites(context.Background(), orgID, listSitesOpts{})
	if err != nil {
		t.Fatalf("listSites all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 sites, got %d", len(all))
	}
	var withAuthor int
	for _, s := range all {
		if s.CreatedBy != nil {
			withAuthor++
		}
	}
	if withAuthor != 2 {
		t.Fatalf("expected 2 sites with createdBy, got %d", withAuthor)
	}

	mine, err := app.listSites(context.Background(), orgID, listSitesOpts{AuthorUserID: "usr_member"})
	if err != nil {
		t.Fatalf("listSites filtered: %v", err)
	}
	if len(mine) != 1 || mine[0].Slug != "owned" {
		t.Fatalf("expected only owned site, got %+v", mine)
	}
}

func TestDuplicateSite_CallerBecomesCreator(t *testing.T) {
	app := newTestApp(t)
	orgID, siteOwned, _ := seedOwnershipFixture(t, app)

	result, err := app.duplicateSite(context.Background(), siteOwned, "usr_admin")
	if err != nil {
		t.Fatalf("duplicateSite: %v", err)
	}
	if result.CreatedBy == nil || result.CreatedBy.ID != "usr_admin" {
		t.Fatalf("expected createdBy=usr_admin, got %+v", result.CreatedBy)
	}
	if !strings.HasPrefix(result.Name, "Copy of ") {
		t.Fatalf("expected 'Copy of ...' name, got %q", result.Name)
	}
	if result.Slug == "owned" {
		t.Fatalf("duplicate slug collided with source")
	}

	// Original site stays unchanged.
	sites, err := app.listSites(context.Background(), orgID, listSitesOpts{})
	if err != nil {
		t.Fatalf("listSites: %v", err)
	}
	if len(sites) != 3 {
		t.Fatalf("expected 3 sites after duplicate, got %d", len(sites))
	}
}

func TestDuplicateSite_UniqueSlug(t *testing.T) {
	app := newTestApp(t)
	_, siteOwned, _ := seedOwnershipFixture(t, app)

	first, err := app.duplicateSite(context.Background(), siteOwned, "usr_admin")
	if err != nil {
		t.Fatalf("first duplicate: %v", err)
	}
	second, err := app.duplicateSite(context.Background(), siteOwned, "usr_admin")
	if err != nil {
		t.Fatalf("second duplicate: %v", err)
	}
	if first.Slug == second.Slug {
		t.Fatalf("expected unique slugs, both got %q", first.Slug)
	}
}

func TestUpsertSiteFromUpload_SetsCreatedBy(t *testing.T) {
	app := newTestApp(t)
	orgID, _, _ := seedOwnershipFixture(t, app)

	prepared := preparedUpload{request: uploadRequest{
		Name:  "Fresh Site",
		Files: []fileMeta{{Name: "index.html", Size: 10, Path: "index.html"}},
	}}
	result, err := app.upsertSiteFromUpload(context.Background(), orgID, "team", "usr_member", roleMember, prepared)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if result.CreatedBy != nil {
		t.Fatalf("returned site is the upsert summary, not listSites shape; check raw DB instead")
	}

	// Verify in DB.
	var createdBy *string
	if err := app.db.QueryRow(context.Background(), `select created_by from sites where id = $1`, result.ID).Scan(&createdBy); err != nil {
		t.Fatalf("read created_by: %v", err)
	}
	if createdBy == nil || *createdBy != "usr_member" {
		t.Fatalf("expected created_by=usr_member, got %v", createdBy)
	}
}

func TestUpsertSiteFromUpload_NonCreatorBlocked(t *testing.T) {
	app := newTestApp(t)
	orgID, _, _ := seedOwnershipFixture(t, app)

	// First upload as member creates the site.
	prepared := preparedUpload{request: uploadRequest{
		Name:  "Owned",
		Files: []fileMeta{{Name: "index.html", Size: 10, Path: "index.html"}},
	}}
	if _, err := app.upsertSiteFromUpload(context.Background(), orgID, "team", "usr_member", roleMember, prepared); err != nil {
		t.Fatalf("initial upload: %v", err)
	}

	// Second member tries to re-upload to the same site name.
	seedUser(t, app.db, "usr_other", "other@example.test", "other", "Other")
	seedOrgMember(t, app.db, orgID, "usr_other", roleMember)
	_, err := app.upsertSiteFromUpload(context.Background(), orgID, "team", "usr_other", roleMember, prepared)
	if !errors.Is(err, ErrSiteMutateForbidden) {
		t.Fatalf("expected ErrSiteMutateForbidden, got %v", err)
	}

	// Admin can re-upload regardless.
	if _, err := app.upsertSiteFromUpload(context.Background(), orgID, "team", "usr_admin", roleAdmin, prepared); err != nil {
		t.Fatalf("admin re-upload: %v", err)
	}
}
