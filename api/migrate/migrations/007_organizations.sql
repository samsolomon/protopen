CREATE TABLE organizations (
  id          text PRIMARY KEY,
  slug        text NOT NULL UNIQUE,
  name        text NOT NULL,
  is_personal boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE org_members (
  id         text PRIMARY KEY,
  org_id     text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       text NOT NULL DEFAULT 'member',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, user_id)
);

CREATE TABLE org_invites (
  id         text PRIMARY KEY,
  org_id     text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email      text NOT NULL,
  role       text NOT NULL DEFAULT 'member',
  invited_by text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  UNIQUE (org_id, email)
);

ALTER TABLE sites ADD COLUMN org_id text REFERENCES organizations(id) ON DELETE CASCADE;

INSERT INTO organizations (id, slug, name, is_personal, created_at, updated_at)
SELECT 'org_' || substr(md5(random()::text || clock_timestamp()::text), 1, 16),
       u.username, u.name, true, u.created_at, u.created_at
FROM users u;

INSERT INTO org_members (id, org_id, user_id, role, created_at)
SELECT 'mem_' || substr(md5(random()::text || clock_timestamp()::text), 1, 16),
       o.id, u.id, 'admin', o.created_at
FROM users u
JOIN organizations o ON o.slug = u.username AND o.is_personal = true;

UPDATE sites SET org_id = o.id
FROM users u
JOIN organizations o ON o.slug = u.username AND o.is_personal = true
WHERE sites.user_id = u.id;

ALTER TABLE sites ALTER COLUMN org_id SET NOT NULL;

DROP INDEX IF EXISTS sites_user_slug_active_idx;

CREATE UNIQUE INDEX sites_org_slug_active_idx ON sites (org_id, slug) WHERE deleted_at IS NULL;

ALTER TABLE sites DROP CONSTRAINT sites_user_id_fkey;

ALTER TABLE sites DROP COLUMN user_id
