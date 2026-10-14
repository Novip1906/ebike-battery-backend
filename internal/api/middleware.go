package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"ebike-battery-backend/internal/auth"

	"github.com/gin-gonic/gin"
)

const (
	bearerPrefix    = "Bearer "
	riderSessionKey = "rider_session"
)

var (
	errNoToken      = errors.New("нет заголовка Authorization с JWT")
	errInvalidToken = errors.New("JWT недействителен или просрочен")
	errRevokedToken = errors.New("JWT отозван при выходе")
)

type TokenBlacklist interface {
	Add(ctx context.Context, token, riderLogin string, ttl time.Duration) error
	Contains(ctx context.Context, token string) (bool, error)
}

type riderSession struct {
	token  string
	claims *auth.Claims
}

type AuthMiddleware struct {
	tokens    *auth.TokenManager
	blacklist TokenBlacklist
}

func NewAuthMiddleware(tokens *auth.TokenManager, blacklist TokenBlacklist) *AuthMiddleware {
	return &AuthMiddleware{tokens: tokens, blacklist: blacklist}
}

func (m *AuthMiddleware) session(c *gin.Context) (*riderSession, error) {
	header := c.GetHeader("Authorization")
	if len(header) < len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return nil, errNoToken
	}
	token := strings.TrimSpace(header[len(bearerPrefix):])
	claims, err := m.tokens.Parse(token)
	if err != nil {
		return nil, errInvalidToken
	}
	revoked, err := m.blacklist.Contains(c.Request.Context(), token)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, errRevokedToken
	}
	return &riderSession{token: token, claims: claims}, nil
}

func (m *AuthMiddleware) RequireRider(c *gin.Context) {
	session, err := m.session(c)
	switch {
	case errors.Is(err, errNoToken):
		fail(c, http.StatusUnauthorized, "Метод доступен только авторизованному велосипедисту: нужен заголовок Authorization со значением Bearer и JWT.")
	case errors.Is(err, errInvalidToken):
		fail(c, http.StatusUnauthorized, "JWT недействителен или просрочен: выполните вход заново.")
	case errors.Is(err, errRevokedToken):
		fail(c, http.StatusUnauthorized, "JWT отозван при выходе: выполните вход заново.")
	case err != nil:
		serverError(c, err)
	default:
		c.Set(riderSessionKey, session)
		c.Next()
	}
}

func (m *AuthMiddleware) OptionalRider(c *gin.Context) {
	session, err := m.session(c)
	switch {
	case err == nil:
		c.Set(riderSessionKey, session)
	case !errors.Is(err, errNoToken) && !errors.Is(err, errInvalidToken) && !errors.Is(err, errRevokedToken):
		log.Printf("чёрный список JWT недоступен, запрос %s %s обработан как гостевой: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.Next()
}

func currentSession(c *gin.Context) *riderSession {
	if value, ok := c.Get(riderSessionKey); ok {
		if session, ok := value.(*riderSession); ok {
			return session
		}
	}
	return nil
}

func currentRiderID(c *gin.Context) uint {
	if session := currentSession(c); session != nil {
		return session.claims.RiderID
	}
	return 0
}
