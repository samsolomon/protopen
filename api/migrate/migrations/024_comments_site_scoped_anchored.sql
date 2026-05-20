-- Comments are anchored to elements (not document coordinates) and survive
-- across deploys.
--
-- 1. deploy_id becomes nullable + SET NULL on delete. It's now "first seen on
--    this deploy" metadata, not identity. Rolling back a deploy preserves the
--    comments instead of cascade-deleting them.
-- 2. Every root comment gets an element anchor. Rows that only had pin_x/pin_y
--    are backfilled to body-anchored with the old normalized coords as the
--    in-element offset (best-effort; user can drag to re-anchor).
-- 3. The deploy-keyed open-comments index is replaced with a site-keyed one to
--    match the new default query (site_id + page_path).
--
-- pin_x / pin_y stay for one release and get dropped in migration 025.

ALTER TABLE comments
    ALTER COLUMN deploy_id DROP NOT NULL,
    DROP CONSTRAINT comments_deploy_id_fkey,
    ADD  CONSTRAINT comments_deploy_id_fkey
         FOREIGN KEY (deploy_id) REFERENCES deploys(id) ON DELETE SET NULL;

-- Backfill: root comments missing an anchor become body-anchored. Replies
-- (parent_id IS NOT NULL) have no pin and stay anchorless.
UPDATE comments
   SET element_selector = 'body',
       element_offset_x = COALESCE(pin_x, 0.5),
       element_offset_y = COALESCE(pin_y, 0.5)
 WHERE parent_id IS NULL
   AND element_selector IS NULL;

DROP INDEX IF EXISTS comments_deploy_page_idx;
CREATE INDEX comments_site_page_open_idx
    ON comments (site_id, page_path)
    WHERE resolved_at IS NULL;
