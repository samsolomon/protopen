-- Break circular FK first (current_deploy_id -> deploys)
ALTER TABLE sites DROP CONSTRAINT sites_current_deploy_id_fkey;
ALTER TABLE sites ADD CONSTRAINT sites_current_deploy_id_fkey
  FOREIGN KEY (current_deploy_id) REFERENCES deploys(id) ON DELETE SET NULL;

-- Cascade deploys when site is deleted
ALTER TABLE deploys DROP CONSTRAINT deploys_site_id_fkey;
ALTER TABLE deploys ADD CONSTRAINT deploys_site_id_fkey
  FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE;

-- Cascade sites when user is deleted
ALTER TABLE sites DROP CONSTRAINT sites_user_id_fkey;
ALTER TABLE sites ADD CONSTRAINT sites_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;