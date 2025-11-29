package uuid

import (
	"github.com/google/uuid"
)

// New generates a new UUID v4
func New() string {
	return uuid.New().String()
}

// Parse parses a UUID string
func Parse(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// IsValid checks if a string is a valid UUID
func IsValid(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// MustParse parses a UUID string and panics on error
func MustParse(s string) uuid.UUID {
	return uuid.MustParse(s)
}

// NewUUID returns a new UUID (as uuid.UUID type)
func NewUUID() uuid.UUID {
	return uuid.New()
}

// Nil returns the nil UUID
func Nil() uuid.UUID {
	return uuid.Nil
}

// IsNil checks if a UUID is nil
func IsNil(u uuid.UUID) bool {
	return u == uuid.Nil
}
