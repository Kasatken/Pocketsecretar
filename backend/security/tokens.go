package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenManager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

type Claims struct {
	UserID int64 `json:"uid"`
	jwt.RegisteredClaims
}

func NewTokenManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

func (m *TokenManager) IssueAccessToken(userID int64) (string, time.Time, error) {
	return m.issue(userID, m.accessTTL, m.accessSecret, "access")
}

func (m *TokenManager) IssueRefreshToken(userID int64) (string, time.Time, error) {
	return m.issue(userID, m.refreshTTL, m.refreshSecret, "refresh")
}

func (m *TokenManager) ParseAccessToken(token string) (*Claims, error) {
	return m.parse(token, m.accessSecret)
}

func (m *TokenManager) ParseRefreshToken(token string) (*Claims, error) {
	return m.parse(token, m.refreshSecret)
}

const subjectGoogleOAuthState = "google_oauth_state"

// IssueGoogleOAuthState короткоживущий JWT для параметра state в OAuth (привязка аккаунта).
func (m *TokenManager) IssueGoogleOAuthState(userID int64) (string, error) {
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			Subject:   subjectGoogleOAuthState,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.refreshSecret)
}

func (m *TokenManager) ParseGoogleOAuthState(token string) (*Claims, error) {
	claims, err := m.parse(token, m.refreshSecret)
	if err != nil {
		return nil, err
	}
	if claims.Subject != subjectGoogleOAuthState {
		return nil, errors.New("invalid oauth state")
	}
	return claims, nil
}

func (m *TokenManager) issue(userID int64, ttl time.Duration, secret []byte, tokenType string) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(ttl)
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			Subject:   tokenType,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (m *TokenManager) parse(token string, secret []byte) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
