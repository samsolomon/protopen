-- Break circular FK first (current_deploy_id -> deploys)
ALTER TABLE projects DROP CONSTRAINT projects_current_deploy_id_fkey;
ALTER TABLE projects ADD CONSTRAINT projects_current_deploy_id_fkey
  FOREIGN KEY (current_deploy_id) REFERENCES deploys(id) ON DELETE SET NULL;

-- Cascade deploys when project is deleted
ALTER TABLE deploys DROP CONSTRAINT deploys_project_id_fkey;
ALTER TABLE deploys ADD CONSTRAINT deploys_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Cascade projects when user is deleted
ALTER TABLE projects DROP CONSTRAINT projects_user_id_fkey;
ALTER TABLE projects ADD CONSTRAINT projects_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;