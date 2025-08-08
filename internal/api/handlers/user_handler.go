package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/isdelr/ender-deploy-be/internal/auth"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/rs/zerolog/log"
)

// UserHandler handles HTTP requests for user management.
type UserHandler struct {
	service services.UserServiceProvider
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(service services.UserServiceProvider) *UserHandler {
	return &UserHandler{service: service}
}

// AuthPayload defines the structure for login requests.
type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterPayload defines the structure for registration requests.
type RegisterPayload struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register handles new user registration.
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var payload RegisterPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.service.CreateUser(payload.Username, payload.Email, payload.Password)
	if err != nil {
		log.Error().Err(err).Str("email", payload.Email).Msg("Failed to register user")
		http.Error(w, "Failed to register user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Don't send password hash back
	user.PasswordHash = ""

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// Login handles user authentication and JWT generation.
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var payload AuthPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.service.AuthenticateUser(payload.Email, payload.Password)
	if err != nil {
		log.Warn().Err(err).Str("email", payload.Email).Msg("Failed authentication attempt")
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateJWT(user)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to generate JWT")
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Set Secure flag based on environment.
	isProd := os.Getenv("APP_ENV") == "production"

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   isProd,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	// sanitize user for response
	user.PasswordHash = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

// GetMe retrieves the currently authenticated user from the token.
func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok {
		log.Error().Msg("Could not retrieve user claims from context")
		http.Error(w, "Could not retrieve user from token", http.StatusInternalServerError)
		return
	}

	user, err := h.service.GetUserByID(claims.UserID)
	if err != nil {
		log.Error().Err(err).Str("user_id", claims.UserID).Msg("User from token not found in DB")
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// sanitize
	user.PasswordHash = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// Get handles retrieving a user by their ID.
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.service.GetUserByID(id)
	if err != nil {
		log.Warn().Err(err).Str("user_id", id).Msg("Failed to get user by ID")
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// sanitize
	user.PasswordHash = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// Update handles updating a user's profile information.
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.service.UpdateUser(id, payload.Username, payload.Email)
	if err != nil {
		log.Error().Err(err).Str("user_id", id).Msg("Failed to update user")
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	// sanitize
	user.PasswordHash = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// Delete handles the permanent deletion of a user account.
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.service.DeleteUser(id); err != nil {
		log.Error().Err(err).Str("user_id", id).Msg("Failed to delete user")
		http.Error(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ChangePassword handles changing a user's password.
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.UpdatePassword(id, payload.CurrentPassword, payload.NewPassword); err != nil {
		log.Error().Err(err).Str("user_id", id).Msg("Failed to change password")
		http.Error(w, "Failed to change password: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
}
