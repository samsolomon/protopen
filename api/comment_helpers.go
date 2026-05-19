// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	notificationTypeCommentReply   = "comment_reply"
	notificationTypeCommentMention = "comment_mention"
	maxGuestNameLen                = 60
)

type siteMeta struct {
	ID        string
	OrgID     string
	IsPublic  bool
	CreatedBy *string
	Name      string
	Slug      string
	OrgSlug   string
}

// loadSiteMeta fetches the minimal site metadata needed by comment handlers
// (auth gating, owner notification, slug rendering). Consolidates the
// scattered SELECTs that used to live inline in comments.go, serve.go, and
// storage.go.
func (app *application) loadSiteMeta(ctx context.Context, siteID string) (siteMeta, error) {
	var m siteMeta
	err := app.db.QueryRow(ctx, `
		select p.id, p.org_id, p.is_public, p.created_by, p.name, p.slug, o.slug
		from sites p
		join organizations o on o.id = p.org_id
		where p.id = $1 and p.deleted_at is null
	`, siteID).Scan(&m.ID, &m.OrgID, &m.IsPublic, &m.CreatedBy, &m.Name, &m.Slug, &m.OrgSlug)
	if err != nil {
		return siteMeta{}, err
	}
	return m, nil
}

// loadSiteMetaBySlug resolves an org+site slug pair to siteMeta plus the
// current deploy id. Used by the comment-context bootstrap endpoint that
// the injected runtime hits.
func (app *application) loadSiteMetaBySlug(ctx context.Context, orgSlug, siteSlug string) (siteMeta, string, error) {
	var (
		m       siteMeta
		deploy  *string
	)
	err := app.db.QueryRow(ctx, `
		select p.id, p.org_id, p.is_public, p.created_by, p.name, p.slug, o.slug, p.current_deploy_id
		from sites p
		join organizations o on o.id = p.org_id
		where o.slug = $1 and p.slug = $2 and p.deleted_at is null
	`, orgSlug, siteSlug).Scan(&m.ID, &m.OrgID, &m.IsPublic, &m.CreatedBy, &m.Name, &m.Slug, &m.OrgSlug, &deploy)
	if err != nil {
		return siteMeta{}, "", err
	}
	deployID := ""
	if deploy != nil {
		deployID = *deploy
	}
	return m, deployID, nil
}

var mentionRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])@([a-zA-Z0-9_-]{2,32})`)

// parseMentions extracts unique @username tokens from a comment body. The
// regex requires a non-identifier character (or start of string) before the
// @ so we don't match email addresses like foo@bar.com.
func parseMentions(body string) []string {
	matches := mentionRegex.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := strings.ToLower(m[1])
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// resolveMentionUserIDs looks up user IDs for the given usernames, scoped to
// members of the given org. Mentions that don't resolve to an org member are
// silently dropped — the @text remains in the comment body but no
// notification fires.
func resolveMentionUserIDs(ctx context.Context, tx pgx.Tx, orgID string, usernames []string) ([]string, error) {
	if len(usernames) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		select distinct u.id
		from users u
		join org_members m on m.user_id = u.id
		where m.org_id = $1 and lower(u.username) = any($2)
	`, orgID, usernames)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// sanitizeGuestName trims, collapses whitespace, and drops control characters
// from a guest's display name. Enforces the length cap.
func sanitizeGuestName(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, raw)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if len(cleaned) > maxGuestNameLen {
		cleaned = cleaned[:maxGuestNameLen]
	}
	return cleaned
}

// guestFingerprint hashes IP+UA+siteID into a 16-char opaque token. Stored
// per-comment so owners can correlate spam without learning the IP.
func guestFingerprint(ip, ua, siteID string) string {
	sum := sha256.Sum256([]byte(ip + "\x00" + ua + "\x00" + siteID))
	return hex.EncodeToString(sum[:])[:16]
}

// upsertSiteSubscription auto-subscribes a user to all comments on a site.
// No-op if the row already exists. Takes pgx.Tx to match the rest of the
// transactional helpers (upsertCommentSubscription).
func upsertSiteSubscription(ctx context.Context, tx pgx.Tx, siteID, userID string) error {
	_, err := tx.Exec(ctx, `
		insert into site_subscriptions (id, site_id, user_id, created_at)
		values ($1, $2, $3, now())
		on conflict (site_id, user_id) do nothing
	`, generateID("ssub"), siteID, userID)
	return err
}
