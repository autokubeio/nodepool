/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package security provides security utilities for validating and managing credentials.
package security

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

var (
	// ErrInvalidTokenFormat indicates the token format is invalid
	ErrInvalidTokenFormat = errors.New("invalid token format: must be 64 alphanumeric characters")
	// ErrTokenValidationFailed indicates the token failed API validation
	ErrTokenValidationFailed = errors.New("token validation failed: unable to authenticate with Hetzner Cloud API")
	// ErrEmptyToken indicates an empty token was provided
	ErrEmptyToken = errors.New("token cannot be empty")
)

// tokenFormatRegex matches valid Hetzner Cloud token format (64 alphanumeric characters)
var tokenFormatRegex = regexp.MustCompile(`^[a-zA-Z0-9]{64}$`)

// TokenValidator validates Hetzner Cloud API tokens
type TokenValidator struct{}

// NewTokenValidator creates a new token validator
func NewTokenValidator() *TokenValidator {
	return &TokenValidator{}
}

// ValidateFormat checks if the token has the correct format
func (v *TokenValidator) ValidateFormat(token string) error {
	// Check for empty token
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrEmptyToken
	}

	// Check token format (64 alphanumeric characters)
	if !tokenFormatRegex.MatchString(token) {
		return fmt.Errorf("%w: got %d characters", ErrInvalidTokenFormat, len(token))
	}

	return nil
}

// ValidateWithAPI validates the token by making a test API call
func (v *TokenValidator) ValidateWithAPI(ctx context.Context, token string) error {
	// First validate format
	if err := v.ValidateFormat(token); err != nil {
		return err
	}

	// Create a temporary client to test the token
	client := hcloud.NewClient(hcloud.WithToken(token))

	// Make a lightweight API call to validate the token
	// Using ListServers with limit 1 to minimize API usage
	opts := hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{
			PerPage: 1,
		},
	}

	_, _, err := client.Server.List(ctx, opts)
	if err != nil {
		// Check if it's an authentication error
		if strings.Contains(err.Error(), "unauthorized") ||
			strings.Contains(err.Error(), "invalid token") ||
			strings.Contains(err.Error(), "authentication") {
			return fmt.Errorf("%w: %w", ErrTokenValidationFailed, err)
		}
		// For other errors, still return them but with context
		return fmt.Errorf("failed to validate token: %w", err)
	}

	return nil
}

// Validate performs both format and API validation
func (v *TokenValidator) Validate(ctx context.Context, token string) error {
	return v.ValidateWithAPI(ctx, token)
}

// SanitizeToken returns a sanitized version of the token for logging
// It shows only the first 8 and last 4 characters
func (v *TokenValidator) SanitizeToken(token string) string {
	if len(token) < 16 {
		return "***"
	}
	return token[:8] + "..." + token[len(token)-4:]
}
