package services

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// UserServiceProvider defines the interface for user services.
type UserServiceProvider interface {
	GetUserByID(id string) (models.User, error)
	CreateUser(username, email, password string) (models.User, error)
	UpdateUser(id, username, email string) (models.User, error)
	UpdatePassword(id, currentPassword, newPassword string) error
	DeleteUser(id string) error
	AuthenticateUser(email, password string) (models.User, error)
}

// UserService provides business logic for user management.
type UserService struct {
	db *sql.DB
}

// NewUserService creates a new UserService.
func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

// GetUserByID retrieves a single user by their ID.
func (s *UserService) GetUserByID(id string) (models.User, error) {
	var user models.User
	row := s.db.QueryRow("SELECT id, username, email, created_at FROM users WHERE id = ?", id)
	err := row.Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.User{}, fmt.Errorf("user with ID %s not found", id)
		}
		return models.User{}, err
	}
	return user, nil
}

// GetUserByEmail retrieves a single user by their email, including the password hash.
func (s *UserService) GetUserByEmail(email string) (models.User, error) {
	var user models.User
	row := s.db.QueryRow("SELECT id, username, email, password_hash, created_at FROM users WHERE email = ?", email)
	err := row.Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.User{}, fmt.Errorf("user with email %s not found", email)
		}
		return models.User{}, err
	}
	return user, nil
}

// CreateUser creates a new user, hashing their password.
func (s *UserService) CreateUser(username, email, password string) (models.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return models.User{}, fmt.Errorf("failed to hash password: %w", err)
	}

	user := models.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
	}

	stmt, err := s.db.Prepare("INSERT INTO users(id, username, email, password_hash) VALUES(?, ?, ?, ?)")
	if err != nil {
		return models.User{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(user.ID, user.Username, user.Email, user.PasswordHash)
	if err != nil {
		return models.User{}, err
	}

	// Return user without password hash
	user.PasswordHash = ""
	return user, nil
}

// UpdateUser updates a user's non-sensitive information.
func (s *UserService) UpdateUser(id, username, email string) (models.User, error) {
	stmt, err := s.db.Prepare("UPDATE users SET username = ?, email = ? WHERE id = ?")
	if err != nil {
		return models.User{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(username, email, id)
	if err != nil {
		return models.User{}, err
	}
	return s.GetUserByID(id)
}

// UpdatePassword verifies the current password, then hashes and sets a new password for a user.
func (s *UserService) UpdatePassword(id, currentPassword, newPassword string) error {
	var user models.User
	row := s.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", id)
	err := row.Scan(&user.PasswordHash)
	if err != nil {
		return fmt.Errorf("could not find user to update password")
	}

	// Check if the current password is correct
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword))
	if err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	_, err = s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", string(hashedPassword), id)
	return err
}

// DeleteUser removes a user from the database.
func (s *UserService) DeleteUser(id string) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// AuthenticateUser verifies a user's credentials.
func (s *UserService) AuthenticateUser(email, password string) (models.User, error) {
	user, err := s.GetUserByEmail(email)
	if err != nil {
		return models.User{}, fmt.Errorf("authentication failed: user not found")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return models.User{}, fmt.Errorf("authentication failed: invalid password")
	}

	// Don't send the password hash to the client
	user.PasswordHash = ""
	return user, nil
}
