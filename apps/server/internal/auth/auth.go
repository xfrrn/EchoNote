package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"
)

const (
	SessionCookieName = "echonote_session"
	SessionTokenBytes = 32
	MinPasswordBytes  = 12
	MaxPasswordBytes  = 72
)

func NormalizeUsername(value string) (display, normalized string, err error) {
	display = norm.NFKC.String(strings.TrimSpace(value))
	count := utf8.RuneCountInString(display)
	if count < 3 || count > 64 {
		return "", "", errors.New("username must contain 3-64 characters")
	}
	for _, character := range display {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '-' && character != '_' && character != '.' {
			return "", "", errors.New("username may only contain letters, numbers, dot, dash, and underscore")
		}
	}
	return display, strings.ToLower(display), nil
}

func HashPassword(password string, cost int) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	return validatePassword(password) == nil && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func NewSessionToken() (string, [sha256.Size]byte, error) {
	raw := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", [sha256.Size]byte{}, err
	}
	return base64.RawURLEncoding.EncodeToString(raw), sha256.Sum256(raw), nil
}

func HashSessionToken(token string) ([sha256.Size]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != SessionTokenBytes || base64.RawURLEncoding.EncodeToString(raw) != token {
		return [sha256.Size]byte{}, errors.New("invalid session token")
	}
	return sha256.Sum256(raw), nil
}

func validatePassword(password string) error {
	if !utf8.ValidString(password) || len(password) < MinPasswordBytes || len(password) > MaxPasswordBytes {
		return errors.New("password must contain 12-72 UTF-8 bytes")
	}
	for _, character := range password {
		if unicode.IsControl(character) {
			return errors.New("password must not contain control characters")
		}
	}
	return nil
}
