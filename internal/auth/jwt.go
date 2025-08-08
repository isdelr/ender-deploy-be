package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/rs/zerolog/log"
)

var jwtKey []byte

// Init sets the JWT signing key at application startup.
func Init(secret string) {
	if secret == "" {
		log.Warn().Msg("JWT secret not provided; using an insecure default. Set JWT_SECRET in production.")
		secret = "change-me"
	}
	jwtKey = []byte(secret)
}

// Claims defines the JWT claims structure.
type Claims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// UserClaimsKey is the context key for user claims.
type contextKey string

const UserClaimsKey = contextKey("userClaims")

// GenerateJWT creates a new JWT for a given user.
func GenerateJWT(user models.User) (string, error) {
	if len(jwtKey) == 0 {
		return "", errors.New("jwt secret is not initialized")
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// ValidateJWT parses and validates a JWT string.
func ValidateJWT(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid auth token")
	}
	return claims, nil
}

// JWTMiddleware creates a middleware for protecting routes.
func JWTMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			// 1) Authorization header (case-insensitive "Bearer " prefix)
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				lower := strings.ToLower(authHeader)
				if strings.HasPrefix(lower, "bearer ") && len(authHeader) > 7 {
					tokenStr = strings.TrimSpace(authHeader[7:])
				}
			}

			// 2) Fallback to cookie
			if tokenStr == "" {
				cookie, err := r.Cookie("token")
				if err != nil {
					http.Error(w, "Missing auth token", http.StatusUnauthorized)
					return
				}
				tokenStr = cookie.Value
			}

			if tokenStr == "" {
				http.Error(w, "Missing auth token", http.StatusUnauthorized)
				return
			}

			claims, err := ValidateJWT(tokenStr)
			if err != nil {
				http.Error(w, "Invalid auth token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			log.Info().Str("username", claims.Username).Str("user_id", claims.UserID).Msg("User authenticated via JWT")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
