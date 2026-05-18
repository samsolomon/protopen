// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samsolomon/protopen/api/migrate"
)

// Database-backed tests opt in via TEST_DATABASE_URL. If unset, callers skip.
// We require an explicit env so a stray `go test ./...` can never truncate a
// dev DB by accident.
const testDBEnv = "TEST_DATABASE_URL"

var (
	testDBOnce sync.Once
	testDBPool *pgxpool.Pool
	testDBErr  error
)

func sharedTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(testDBEnv)
	if url == "" {
		t.Skipf("set %s to run database-backed tests", testDBEnv)
		return nil
	}
	testDBOnce.Do(func() {
		ctx := context.Background()
		testDBPool, testDBErr = pgxpool.New(ctx, url)
		if testDBErr != nil {
			return
		}
		if testDBErr = migrate.Run(ctx, testDBPool); testDBErr != nil {
			testDBPool.Close()
			testDBPool = nil
		}
	})
	if testDBErr != nil {
		t.Fatalf("connect %s: %v", testDBEnv, testDBErr)
	}
	return testDBPool
}

// newTestApp returns an application bound to the shared test pool with all
// data tables truncated. Each call starts from an empty database state.
func newTestApp(t *testing.T) *application {
	t.Helper()
	pool := sharedTestPool(t)
	truncateAll(t, pool)
	return &application{
		db:             pool,
		ingestRoot:     t.TempDir(),
		contentBaseURL: "http://content.test",
	}
}

func truncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	// users + organizations + instance_settings cover every data table via
	// CASCADE because everything else (sites, deploys, org_members, sessions,
	// notifications, audit_log, comments, api_tokens, etc.) ultimately
	// references one of them.
	if _, err := pool.Exec(ctx, `truncate users, organizations, instance_settings restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// seedUser inserts a minimal users row. auth_ref is required (NOT NULL) but
// the value isn't used outside the auth path, so we set a placeholder.
func seedUser(t *testing.T, pool *pgxpool.Pool, id, email, username, name string) {
	t.Helper()
	mustExec(t, pool, `
		insert into users (id, email, auth_ref, username, name)
		values ($1, $2, $3, $4, $5)
	`, id, email, "test", username, name)
}

func seedOrg(t *testing.T, pool *pgxpool.Pool, id, slug, name string, isPersonal bool) {
	t.Helper()
	mustExec(t, pool, `
		insert into organizations (id, slug, name, is_personal)
		values ($1, $2, $3, $4)
	`, id, slug, name, isPersonal)
}

func seedOrgMember(t *testing.T, pool *pgxpool.Pool, orgID, userID, role string) {
	t.Helper()
	mustExec(t, pool, `
		insert into org_members (id, org_id, user_id, role)
		values ($1, $2, $3, $4)
	`, "mem_"+orgID+"_"+userID, orgID, userID, role)
}

func seedSite(t *testing.T, pool *pgxpool.Pool, id, orgID, slug, name string, createdBy *string) {
	t.Helper()
	mustExec(t, pool, `
		insert into sites (id, org_id, slug, name, created_by)
		values ($1, $2, $3, $4, $5)
	`, id, orgID, slug, name, createdBy)
}
