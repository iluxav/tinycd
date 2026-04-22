// Package auth handles password storage and signed session cookies for the
// tcd web UI.
//
// Credentials live in ~/.config/tcd/auth.yml with mode 0600. Passwords are
// stored as bcrypt hashes. Session cookies are stateless: the cookie value is
// "<base64 username>|<expiry unix>|<hmac>" signed with a random session key
// that's generated once and persisted alongside the users. Server restarts
// don't invalidate sessions; rotating the session_key in auth.yml does.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// DefaultSessionTTL is how long a login lasts.
const DefaultSessionTTL = 30 * 24 * time.Hour

var (
	ErrNotConfigured = errors.New("auth not configured — run `tcd admin set-password admin`")
	ErrBadCreds      = errors.New("invalid username or password")
	ErrBadCookie     = errors.New("invalid session cookie")
	ErrExpired       = errors.New("session expired")
)

// User is a credential record.
type User struct {
	Name       string `yaml:"name"`
	BcryptHash string `yaml:"bcrypt_hash"`
}

// File is the on-disk auth state.
type File struct {
	Users      []User `yaml:"users"`
	SessionKey string `yaml:"session_key"` // base64; 32 bytes decoded

	path string // not marshalled
}

// Path returns ~/.config/tcd/auth.yml.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tcd", "auth.yml"), nil
}

// Load reads auth.yml. Returns ErrNotConfigured if missing or empty.
func Load() (*File, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotConfigured
		}
		return nil, err
	}
	f := &File{path: p}
	if err := yaml.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if len(f.Users) == 0 || f.SessionKey == "" {
		return nil, ErrNotConfigured
	}
	return f, nil
}

// Save writes auth.yml with 0600 perms.
func (f *File) Save() error {
	if f.path == "" {
		p, err := Path()
		if err != nil {
			return err
		}
		f.path = p
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, data, 0o600)
}

// SetPassword creates or updates a user with the given cleartext password.
// Generates a SessionKey on first use.
func (f *File) SetPassword(name, password string) error {
	if name == "" {
		return errors.New("username is required")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	for i, u := range f.Users {
		if u.Name == name {
			f.Users[i].BcryptHash = string(hash)
			return nil
		}
	}
	f.Users = append(f.Users, User{Name: name, BcryptHash: string(hash)})
	if f.SessionKey == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		f.SessionKey = base64.StdEncoding.EncodeToString(key)
	}
	return nil
}

// Verify checks credentials. Returns ErrBadCreds on any mismatch.
func (f *File) Verify(name, password string) error {
	for _, u := range f.Users {
		if u.Name != name {
			continue
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.BcryptHash), []byte(password)); err != nil {
			return ErrBadCreds
		}
		return nil
	}
	// Constant-time: always do a bcrypt compare so timing doesn't leak existence.
	_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$CwTycUXWue0Thq9StjUM0uJ8jAzFPpgQSpgp3kM8.V5WyqBOeOw9e"), []byte(password))
	return ErrBadCreds
}

// key returns the raw 32-byte session key.
func (f *File) key() ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(f.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("session_key: %w", err)
	}
	if len(b) < 16 {
		return nil, errors.New("session_key too short")
	}
	return b, nil
}

// MakeCookie returns a signed session cookie value for name valid for ttl.
func (f *File) MakeCookie(name string, ttl time.Duration) (string, error) {
	k, err := f.key()
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(ttl).Unix()
	nameB64 := base64.RawURLEncoding.EncodeToString([]byte(name))
	payload := nameB64 + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "|" + sig, nil
}

// ParseCookie verifies a cookie value and returns the username if valid.
func (f *File) ParseCookie(value string) (string, error) {
	parts := strings.Split(value, "|")
	if len(parts) != 3 {
		return "", ErrBadCookie
	}
	nameB64, expStr, sig := parts[0], parts[1], parts[2]
	k, err := f.key()
	if err != nil {
		return "", err
	}
	payload := nameB64 + "|" + expStr
	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(sig)) != 1 {
		return "", ErrBadCookie
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", ErrBadCookie
	}
	if time.Now().Unix() > exp {
		return "", ErrExpired
	}
	name, err := base64.RawURLEncoding.DecodeString(nameB64)
	if err != nil {
		return "", ErrBadCookie
	}
	return string(name), nil
}
