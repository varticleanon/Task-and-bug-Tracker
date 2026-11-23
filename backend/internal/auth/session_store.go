package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SessionStore struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

func (s *SessionStore) CreateSession(ctx context.Context, userID uuid.UUID, ip, userAgent string) (string, *Session, error) {
	sessionID := uuid.New()
	tokenStr, _, verifierHash, err := GenerateSessionToken(sessionID)
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	idleExpiresAt := now.Add(IdleExpiryDuration)
	absoluteExpiresAt := now.Add(AbsoluteExpiryDuration)

	session := &Session{
		ID:                sessionID,
		UserID:            userID,
		VerifierHash:      verifierHash,
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     idleExpiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		IP:                ip,
		UserAgent:         userAgent,
	}

	query := `
		INSERT INTO sessions (id, user_id, verifier_hash, created_at, last_seen_at, idle_expires_at, absolute_expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = s.pool.Exec(ctx, query,
		session.ID, session.UserID, session.VerifierHash,
		session.CreatedAt, session.LastSeenAt,
		session.IdleExpiresAt, session.AbsoluteExpiresAt,
		session.IP, session.UserAgent,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to insert session into DB: %w", err)
	}

	return tokenStr, session, nil
}

func (s *SessionStore) ValidateSession(ctx context.Context, tokenStr string) (*Session, error) {
	sessionID, verifier, err := ParseSessionToken(tokenStr)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, verifier_hash, created_at, last_seen_at, idle_expires_at, absolute_expires_at, revoked_at, COALESCE(ip, ''), COALESCE(user_agent, '')
		FROM sessions
		WHERE id = $1
	`

	var session Session
	var ip, userAgent string
	err = s.pool.QueryRow(ctx, query, sessionID).Scan(
		&session.ID, &session.UserID, &session.VerifierHash,
		&session.CreatedAt, &session.LastSeenAt,
		&session.IdleExpiresAt, &session.AbsoluteExpiresAt,
		&session.RevokedAt, &ip, &userAgent,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	} else if err != nil {
		return nil, fmt.Errorf("database query error during session validation: %w", err)
	}

	session.IP = ip
	session.UserAgent = userAgent

	if session.RevokedAt != nil {
		return nil, ErrSessionNotFound
	}

	if !VerifyVerifierHash(verifier, session.VerifierHash) {
		return nil, ErrInvalidToken
	}

	now := time.Now().UTC()
	if now.After(session.IdleExpiresAt) || now.After(session.AbsoluteExpiresAt) {
		return nil, ErrSessionExpired
	}

	newIdleExpiresAt := now.Add(IdleExpiryDuration)
	if newIdleExpiresAt.After(session.AbsoluteExpiresAt) {
		newIdleExpiresAt = session.AbsoluteExpiresAt
	}

	updateQuery := `
		UPDATE sessions
		SET last_seen_at = $1, idle_expires_at = $2
		WHERE id = $3 AND revoked_at IS NULL
	`
	_, _ = s.pool.Exec(ctx, updateQuery, now, newIdleExpiresAt, session.ID)

	session.LastSeenAt = now
	session.IdleExpiresAt = newIdleExpiresAt

	return &session, nil
}

func (s *SessionStore) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	query := `UPDATE sessions SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`
	_, err := s.pool.Exec(ctx, query, sessionID)
	return err
}

func (s *SessionStore) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := s.pool.Exec(ctx, query, userID)
	return err
}
