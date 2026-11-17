-- name: InsertGroup :exec
INSERT INTO workspace_groups (id, workspace_id, name, created_at)
VALUES ($1, $2, $3, $4);

-- name: ListGroups :many
SELECT g.id, g.workspace_id, g.name, g.created_at,
       count(m.user_id)::bigint AS member_count,
       COALESCE(array_agg(m.user_id ORDER BY m.manager_since, m.user_id) FILTER (WHERE m.is_manager), '{}')::uuid[] AS manager_ids
FROM workspace_groups g
LEFT JOIN workspace_group_members m ON m.group_id = g.id
WHERE g.workspace_id = $1
GROUP BY g.id
ORDER BY lower(g.name), g.id;

-- name: GetGroup :one
SELECT g.id, g.workspace_id, g.name, g.created_at,
       count(m.user_id)::bigint AS member_count,
       COALESCE(array_agg(m.user_id ORDER BY m.manager_since, m.user_id) FILTER (WHERE m.is_manager), '{}')::uuid[] AS manager_ids
FROM workspace_groups g
LEFT JOIN workspace_group_members m ON m.group_id = g.id
WHERE g.workspace_id = $1 AND g.id = $2
GROUP BY g.id;

-- name: LockGroup :one
-- Serialises changes to one group's members.
SELECT id FROM workspace_groups WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: RenameGroup :execrows
UPDATE workspace_groups SET name = $3 WHERE workspace_id = $1 AND id = $2;

-- name: DeleteGroup :execrows
DELETE FROM workspace_groups WHERE workspace_id = $1 AND id = $2;

-- name: ListGroupMembers :many
SELECT user_id, is_manager
FROM workspace_group_members
WHERE group_id = $1
ORDER BY added_at, user_id;

-- name: DeleteGroupMembersNotIn :exec
DELETE FROM workspace_group_members
WHERE group_id = @group_id AND user_id <> ALL(@keep::uuid[]);

-- name: UpsertGroupMember :exec
-- A manager who stays a manager keeps the time they became one.
INSERT INTO workspace_group_members (group_id, user_id, is_manager, manager_since, added_at)
VALUES (@group_id, @user_id, @is_manager, CASE WHEN @is_manager::boolean THEN @now::timestamptz END, @now)
ON CONFLICT (group_id, user_id) DO UPDATE
SET is_manager = EXCLUDED.is_manager,
    manager_since = CASE
        WHEN NOT EXCLUDED.is_manager THEN NULL
        ELSE COALESCE(workspace_group_members.manager_since, EXCLUDED.manager_since)
    END;

-- name: FilterActiveMembers :many
SELECT user_id FROM workspace_members
WHERE workspace_id = @workspace_id AND status = 'active' AND user_id = ANY(@ids::uuid[]);

-- name: DeleteGroupMemberships :exec
DELETE FROM workspace_group_members m
USING workspace_groups g
WHERE m.group_id = g.id AND g.workspace_id = $1 AND m.user_id = $2;

-- name: ManagersOf :many
-- Managers of every group the user belongs to, ordered by group name, then
-- by when they became manager. A manager who is not an active member of the
-- workspace anymore is skipped. The caller removes duplicates.
SELECT mgr.user_id
FROM workspace_group_members me
JOIN workspace_groups g ON g.id = me.group_id
JOIN workspace_group_members mgr ON mgr.group_id = g.id AND mgr.is_manager AND mgr.user_id <> me.user_id
JOIN workspace_members wm ON wm.workspace_id = g.workspace_id AND wm.user_id = mgr.user_id AND wm.status = 'active'
WHERE g.workspace_id = $1 AND me.user_id = $2
ORDER BY lower(g.name), g.id, mgr.manager_since, mgr.user_id;
