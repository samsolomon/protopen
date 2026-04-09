-- Sentinel org for anonymous deploys (no real users are members)
INSERT INTO organizations (id, slug, name, is_personal, created_at, updated_at)
VALUES ('org_anonymous', '_anon', 'Anonymous', false, now(), now())
ON CONFLICT (id) DO NOTHING;

-- Anonymous deploys table
CREATE TABLE anonymous_deploys (
  id TEXT PRIMARY KEY,
  slug TEXT UNIQUE NOT NULL,
  deploy_id TEXT NOT NULL REFERENCES deploys(id),
  project_id TEXT NOT NULL REFERENCES projects(id),
  claim_token_hash TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  claimed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_anonymous_deploys_slug ON anonymous_deploys(slug);
CREATE INDEX idx_anonymous_deploys_expires ON anonymous_deploys(expires_at) WHERE claimed_at IS NULL;
