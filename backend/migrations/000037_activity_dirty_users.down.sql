DROP TRIGGER IF EXISTS comments_activity_full_rebuild ON comments;
DROP TRIGGER IF EXISTS comments_activity_dirty_update ON comments;
DROP TRIGGER IF EXISTS comments_activity_dirty_insert_delete ON comments;
DROP TRIGGER IF EXISTS posts_activity_full_rebuild ON posts;
DROP TRIGGER IF EXISTS posts_activity_dirty_update ON posts;
DROP TRIGGER IF EXISTS posts_activity_dirty_insert_delete ON posts;
DROP FUNCTION IF EXISTS log_activity_full_rebuild();
DROP FUNCTION IF EXISTS log_activity_dirty_users();
DROP TABLE IF EXISTS activity_dirty_users;
