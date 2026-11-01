-- name: SessionUser :one
SELECT u.id,u.login,u.name,u.role,u.disabled,u.must_change FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=? AND s.expires_at>?;
-- name: CountUsers :one
SELECT count(*) FROM users;
