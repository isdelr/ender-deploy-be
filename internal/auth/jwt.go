package auth

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/isdelr/ender-deploy-be/internal/models"
)

var jwtKey = []byte(os.Getenv("JWT_SECRET"))

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
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
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
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// JWTMiddleware creates a middleware for protecting routes.
func JWTMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			// 1. Try to get the token from the Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.Split(authHeader, "Bearer ")
				if len(parts) == 2 {
					tokenStr = parts[1]
				}
			}

			// 2. If not in header, fall back to the cookie
			if tokenStr == "" {
				cookie, err := r.Cookie("token")
				if err != nil {
					http.Error(w, "Missing auth token", http.StatusUnauthorized)
					return
				}
				tokenStr = cookie.Value
			}

			// 3. If we still have no token, fail
			if tokenStr == "" {
				http.Error(w, "Missing auth token", http.StatusUnauthorized)
				return
			}

			// 4. Validate the token
			claims, err := ValidateJWT(tokenStr)
			if err != nil {
				http.Error(w, "Invalid auth token", http.StatusUnauthorized)
				return
			}

			// 5. Pass claims down via context
			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			fmt.Printf("Authenticated user: %s (ID: %s)\n", claims.Username, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
