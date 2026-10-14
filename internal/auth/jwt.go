package auth

import (
	"errors"
	"os"
	"time"

	"ebike-battery-backend/internal/ds"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenIssuer      = "ebike-battery-backend"
	defaultExpiresIn = time.Hour
)

type Claims struct {
	jwt.RegisteredClaims
	RiderID               uint `json:"rider_id"`
	IsBikeServiceEngineer bool `json:"is_bike_service_engineer"`
}

type TokenManager struct {
	secret    []byte
	expiresIn time.Duration
}

func NewTokenManager(secret string, expiresIn time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), expiresIn: expiresIn}
}

func TokenManagerFromEnv() (*TokenManager, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, errors.New("не задан JWT_SECRET")
	}
	expiresIn := defaultExpiresIn
	if raw := os.Getenv("JWT_EXPIRES_IN"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, errors.New("JWT_EXPIRES_IN задаётся длительностью, например 1h или 30m")
		}
		expiresIn = parsed
	}
	return NewTokenManager(secret, expiresIn), nil
}

func (m *TokenManager) ExpiresIn() time.Duration {
	return m.expiresIn
}

func (m *TokenManager) Issue(rider ds.Rider) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   rider.Login,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiresIn)),
		},
		RiderID:               rider.ID,
		IsBikeServiceEngineer: rider.IsBikeServiceEngineer,
	})
	return token.SignedString(m.secret)
}

func (m *TokenManager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if claims.RiderID == 0 {
		return nil, errors.New("в токене нет велосипедиста")
	}
	return claims, nil
}
