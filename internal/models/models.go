package models

import "time"

// User is an authenticated account.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Mark is a stored logo or icon.
type Mark struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	OriginalPath string    `json:"-"`
	OriginalMIME string    `json:"original_mime"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	HasAlpha     bool      `json:"has_alpha"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
