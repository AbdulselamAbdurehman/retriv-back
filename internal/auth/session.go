package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SessionManager issues and verifies stateless, HMAC-signed session tokens.
// Format: base64(userID|expiryUnix).base64(hmac). No server-side session store.
type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

func NewSessionManager(secret string, ttl time.Duration) *SessionManager {
	return &SessionManager{secret: []byte(secret), ttl: ttl}
}

var ErrInvalidSession = errors.New("auth: invalid session")

func (m *SessionManager) Issue(userID uuid.UUID) string {
	payload := fmt.Sprintf("%s|%d", userID, time.Now().Add(m.ttl).Unix())
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return enc + "." + m.sign(enc)
}

// Verify checks the signature and expiry, returning the user ID.
func (m *SessionManager) Verify(token string) (uuid.UUID, error) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(m.sign(enc))) {
		return uuid.Nil, ErrInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return uuid.Nil, ErrInvalidSession
	}
	idStr, expStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return uuid.Nil, ErrInvalidSession
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return uuid.Nil, ErrInvalidSession
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, ErrInvalidSession
	}
	return id, nil
}

func (m *SessionManager) sign(msg string) string {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
