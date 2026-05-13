CREATE TABLE comments (
    id          text PRIMARY KEY,
    site_id     text NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    deploy_id   text NOT NULL REFERENCES deploys(id) ON DELETE CASCADE,
    user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    page_path   text NOT NULL DEFAULT '',
    pin_x       numeric(7,4),
    pin_y       numeric(7,4),
    body        text NOT NULL,
    parent_id   text REFERENCES comments(id) ON DELETE CASCADE,
    resolved_at timestamptz,
    resolved_by text REFERENCES users(id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX comments_deploy_page_idx ON comments (deploy_id, page_path) WHERE resolved_at IS NULL;
CREATE INDEX comments_site_idx ON comments (site_id, created_at DESC);
