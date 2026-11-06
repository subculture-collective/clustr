-- Authors whose posts or comments changed since the last activity refresh.
-- Precalculation recomputes user_subreddit_activity and subreddit_relationships
-- for these users only. A NULL user_id requests a full rebuild; it is written
-- by TRUNCATE of a source table and once here so the first refresh after this
-- migration starts from a complete, consistent baseline.
CREATE TABLE activity_dirty_users (
    user_id INTEGER
);

CREATE OR REPLACE FUNCTION log_activity_dirty_users() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.author_id IS NOT NULL THEN
            INSERT INTO activity_dirty_users(user_id) VALUES (NEW.author_id);
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        IF OLD.author_id IS NOT NULL THEN
            INSERT INTO activity_dirty_users(user_id) VALUES (OLD.author_id);
        END IF;
    ELSE
        IF OLD.author_id IS NOT NULL THEN
            INSERT INTO activity_dirty_users(user_id) VALUES (OLD.author_id);
        END IF;
        IF NEW.author_id IS NOT NULL AND NEW.author_id IS DISTINCT FROM OLD.author_id THEN
            INSERT INTO activity_dirty_users(user_id) VALUES (NEW.author_id);
        END IF;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION log_activity_full_rebuild() RETURNS trigger AS $$
BEGIN
    INSERT INTO activity_dirty_users(user_id) VALUES (NULL);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Upserts rewrite author_id and subreddit_id on every crawl; only a real
-- reassignment changes activity, so updates are filtered in WHEN.
CREATE TRIGGER posts_activity_dirty_insert_delete
    AFTER INSERT OR DELETE ON posts
    FOR EACH ROW EXECUTE FUNCTION log_activity_dirty_users();
CREATE TRIGGER posts_activity_dirty_update
    AFTER UPDATE OF author_id, subreddit_id ON posts
    FOR EACH ROW
    WHEN (OLD.author_id IS DISTINCT FROM NEW.author_id OR OLD.subreddit_id IS DISTINCT FROM NEW.subreddit_id)
    EXECUTE FUNCTION log_activity_dirty_users();
CREATE TRIGGER posts_activity_full_rebuild
    AFTER TRUNCATE ON posts
    FOR EACH STATEMENT EXECUTE FUNCTION log_activity_full_rebuild();

CREATE TRIGGER comments_activity_dirty_insert_delete
    AFTER INSERT OR DELETE ON comments
    FOR EACH ROW EXECUTE FUNCTION log_activity_dirty_users();
CREATE TRIGGER comments_activity_dirty_update
    AFTER UPDATE OF author_id, subreddit_id ON comments
    FOR EACH ROW
    WHEN (OLD.author_id IS DISTINCT FROM NEW.author_id OR OLD.subreddit_id IS DISTINCT FROM NEW.subreddit_id)
    EXECUTE FUNCTION log_activity_dirty_users();
CREATE TRIGGER comments_activity_full_rebuild
    AFTER TRUNCATE ON comments
    FOR EACH STATEMENT EXECUTE FUNCTION log_activity_full_rebuild();

INSERT INTO activity_dirty_users(user_id) VALUES (NULL);
