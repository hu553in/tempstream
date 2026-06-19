-- name: CreateLink :one
INSERT INTO
    watch_links (
        token,
        enabled,
        created_at,
        expires_at,
        disabled_at,
        note
    )
VALUES
    (?, 1, ?, ?, NULL, ?)
RETURNING
    id,
    token,
    enabled,
    created_at,
    expires_at,
    disabled_at,
    note;

-- name: GetLinkByID :one
SELECT
    id,
    token,
    enabled,
    created_at,
    expires_at,
    disabled_at,
    note
FROM
    watch_links
WHERE
    id = ?
LIMIT
    1;

-- name: GetActiveLinkByToken :one
SELECT
    id,
    token,
    enabled,
    created_at,
    expires_at,
    disabled_at,
    note
FROM
    watch_links
WHERE
    token = ?
    AND enabled = 1
    AND (
        expires_at IS NULL
        OR expires_at > ?
    )
LIMIT
    1;

-- name: ListActiveLinks :many
SELECT
    id,
    token,
    enabled,
    created_at,
    expires_at,
    disabled_at,
    note
FROM
    watch_links
WHERE
    enabled = 1
    AND (
        expires_at IS NULL
        OR expires_at > ?
    )
ORDER BY
    created_at DESC;

-- name: GetLastActiveLink :one
SELECT
    id,
    token,
    enabled,
    created_at,
    expires_at,
    disabled_at,
    note
FROM
    watch_links
WHERE
    enabled = 1
    AND (
        expires_at IS NULL
        OR expires_at > ?
    )
ORDER BY
    created_at DESC
LIMIT
    1;

-- name: DisableLinkByID :execrows
UPDATE watch_links
SET
    enabled = 0,
    disabled_at = ?
WHERE
    id = ?
    AND enabled = 1;

-- name: DeleteExpiredDisabledLinks :execrows
DELETE FROM watch_links
WHERE
    enabled = 0
    AND disabled_at IS NOT NULL
    AND disabled_at < ?;
