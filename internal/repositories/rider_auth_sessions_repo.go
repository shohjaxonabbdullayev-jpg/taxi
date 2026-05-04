package repositories

import (
	"context"
	"database/sql"
	"errors"
)

// ErrRiderSessionNotFound is returned when a refresh-token row is missing,
// expired, revoked, or already used by a different client.
var ErrRiderSessionNotFound = errors.New("rider_auth_sessions: refresh token not found")

// RiderAuthSession represents a refresh-token row.
type RiderAuthSession struct {
	ID               int64
	UserID           int64
	RefreshHash      string
	RefreshExpiresAt int64
	Revoked          bool
	CreatedAt        int64
}

// RiderAuthSessionsRepo persists hashed refresh tokens for the rider native
// auth flow. Access tokens are stateless HS256 JWTs and do NOT live here.
type RiderAuthSessionsRepo struct {
	db *sql.DB
}

// NewRiderAuthSessionsRepo returns a repo.
func NewRiderAuthSessionsRepo(db *sql.DB) *RiderAuthSessionsRepo {
	return &RiderAuthSessionsRepo{db: db}
}

// Insert stores a new refresh-token row.
func (r *RiderAuthSessionsRepo) Insert(ctx context.Context, userID int64, refreshHash string, refreshExpiresAt int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO rider_auth_sessions (user_id, refresh_hash, refresh_expires_at)
		VALUES (?1, ?2, ?3)`,
		userID, refreshHash, refreshExpiresAt)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// GetByRefreshHash returns the row whose refresh_hash matches and that is
// not revoked / not expired. Otherwise ErrRiderSessionNotFound.
func (r *RiderAuthSessionsRepo) GetByRefreshHash(ctx context.Context, refreshHash string, nowUnix int64) (*RiderAuthSession, error) {
	var s RiderAuthSession
	var rev int
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, refresh_hash, refresh_expires_at, revoked, created_at
		FROM rider_auth_sessions
		WHERE refresh_hash = ?1 AND revoked = 0 AND refresh_expires_at > ?2
		LIMIT 1`,
		refreshHash, nowUnix).Scan(&s.ID, &s.UserID, &s.RefreshHash, &s.RefreshExpiresAt, &rev, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrRiderSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Revoked = rev != 0
	return &s, nil
}

// RevokeByRefreshHash marks a single row revoked. Returns rows affected.
func (r *RiderAuthSessionsRepo) RevokeByRefreshHash(ctx context.Context, refreshHash string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE rider_auth_sessions SET revoked = 1 WHERE refresh_hash = ?1 AND revoked = 0`,
		refreshHash)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// RevokeAllForUser marks every active session for the given user as revoked.
// Used by /v1/rider/auth/logout where we only know the user id (from access JWT).
func (r *RiderAuthSessionsRepo) RevokeAllForUser(ctx context.Context, userID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE rider_auth_sessions SET revoked = 1 WHERE user_id = ?1 AND revoked = 0`,
		userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
