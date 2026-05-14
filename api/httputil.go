// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// passwordHashCost is the bcrypt work factor for new password hashes. Existing
// hashes at a lower cost verify fine — bcrypt embeds the cost in the hash.
const passwordHashCost = 12

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// validateEmail parses with net/mail.ParseAddress and returns the canonical
// lowercased address. Rejects empty input, malformed addresses, and any
// control character (CRLF in particular) in the parsed local-part/name.
func validateEmail(raw string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid email address")
	}
	if strings.ContainsAny(addr.Address, "\r\n") || strings.ContainsAny(addr.Name, "\r\n") {
		return "", fmt.Errorf("invalid email address")
	}
	return strings.ToLower(addr.Address), nil
}

// rejectControlChars returns an error if value contains CR/LF. Use for any
// user-supplied string that flows into email subjects/bodies/headers.
func rejectControlChars(value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("input contains invalid characters")
	}
	return nil
}

const (
	roleAdmin  = "admin"
	roleMember = "member"
)

func isDuplicateKeyError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func relativeTime(value time.Time) string {
	delta := time.Since(value)
	switch {
	case delta < time.Minute:
		return "Just now"
	case delta < time.Hour:
		minutes := int(delta.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case delta < 24*time.Hour:
		hours := int(delta.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case delta < 48*time.Hour:
		return "Yesterday"
	default:
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	}
}

func countForMatch(matches []siteRecord) int {
	if len(matches) == 1 {
		return matches[0].Deploys + 1
	}
	return 1
}

func validateSiteMatchCount(matches []siteRecord, name string) error {
	if len(matches) > 1 {
		return fmt.Errorf("multiple existing sites match %q", name)
	}
	return nil
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".zip", "")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")

	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		isLetter := char >= 'a' && char <= 'z'
		isNumber := char >= '0' && char <= '9'

		switch {
		case isLetter || isNumber:
			builder.WriteRune(char)
			lastDash = false
		case !lastDash:
			builder.WriteRune('-')
			lastDash = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "prototype"
	}

	return slug
}

func generateID(prefix string) string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(buffer)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func generateToken(byteLength int) string {
	buffer := make([]byte, byteLength)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func withCORS(frontendOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin == frontendOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

const maxJSONBodyBytes = 1 << 20 // 1 MiB — applied to /api/* except multipart uploads

func limitJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodDelete && r.Method != http.MethodOptions {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "multipart/form-data") {
				r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func appSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func contentSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func requireSession(user sessionUser, w http.ResponseWriter) bool {
	if user.isBearerToken {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "this action requires signing in"})
		return false
	}
	return true
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func getenv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
