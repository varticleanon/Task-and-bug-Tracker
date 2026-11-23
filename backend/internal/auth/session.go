package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidToken    = errors.New("invalid session token format")
	ErrSessionNotFound = errors.New("session not found or revoked")
	ErrSessionExpired  = errors.New("session has expired")
)

const (
	IdleExpiryDuration     = 24 * time.Hour
	AbsoluteExpiryDuration = 7 * 24 * time.Hour
)

type Session struct {
	ID                uuid.UUID  `json:"id"`
	UserID            uuid.UUID  `json:"user_id"`
	VerifierHash      []byte     `json:"-"`
	CreatedAt         time.Time  `json:"created_at"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	IdleExpiresAt     time.Time  `json:"idle_expires_at"`
	AbsoluteExpiresAt time.Time  `json:"absolute_expires_at"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	IP                string     `json:"ip,omitempty"`
	UserAgent         string     `json:"user_agent,omitempty"`
}

func GenerateSessionToken(sessionID uuid.UUID) (tokenStr string, verifier []byte, verifierHash []byte, err error) {
	verifier = make([]byte, 32)
	if _, err := rand.Read(verifier); err != nil {
		return "", nil, nil, fmt.Errorf("failed to generate CSPRNG verifier: %w", err)
	}

	hash := sha256.Sum256(verifier)
	verifierHash = hash[:]

	tokenStr = fmt.Sprintf("%s.%s", sessionID.String(), hex.EncodeToString(verifier))
	return tokenStr, verifier, verifierHash, nil
}

func ParseSessionToken(tokenStr string) (sessionID uuid.UUID, verifier []byte, err error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return uuid.Nil, nil, ErrInvalidToken
	}

	sessionID, err = uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, nil, ErrInvalidToken
	}

	verifier, err = hex.DecodeString(parts[1])
	if err != nil || len(verifier) != 32 {
		return uuid.Nil, nil, ErrInvalidToken
	}

	return sessionID, verifier, nil
}

func VerifyVerifierHash(verifier []byte, storedHash []byte) bool {
	hash := sha256.Sum256(verifier)
	return subtle.ConstantTimeCompare(hash[:], storedHash) == 1
}
