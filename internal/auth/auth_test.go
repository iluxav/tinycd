package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSetAndVerifyPassword(t *testing.T) {
	f := &File{}
	if err := f.SetPassword("admin", "correct-horse-battery-staple"); err != nil {
		t.Fatal(err)
	}
	if f.SessionKey == "" {
		t.Error("SessionKey should be auto-generated on first SetPassword")
	}
	if err := f.Verify("admin", "correct-horse-battery-staple"); err != nil {
		t.Errorf("correct password should verify: %v", err)
	}
	if err := f.Verify("admin", "wrong"); !errors.Is(err, ErrBadCreds) {
		t.Errorf("wrong password should ErrBadCreds, got %v", err)
	}
	if err := f.Verify("nobody", "correct-horse-battery-staple"); !errors.Is(err, ErrBadCreds) {
		t.Errorf("unknown user should ErrBadCreds, got %v", err)
	}
}

func TestSetPasswordValidation(t *testing.T) {
	f := &File{}
	if err := f.SetPassword("admin", "short"); err == nil {
		t.Error("expected error for short password")
	}
	if err := f.SetPassword("", "longenough"); err == nil {
		t.Error("expected error for empty user")
	}
}

func TestCookieRoundTrip(t *testing.T) {
	f := &File{}
	_ = f.SetPassword("admin", "longenoughpw")

	cookie, err := f.MakeCookie("admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cookie, "|") {
		t.Errorf("cookie should have | separators, got %q", cookie)
	}
	name, err := f.ParseCookie(cookie)
	if err != nil {
		t.Fatal(err)
	}
	if name != "admin" {
		t.Errorf("cookie user = %q, want admin", name)
	}
}

func TestCookieTampered(t *testing.T) {
	f := &File{}
	_ = f.SetPassword("admin", "longenoughpw")

	cookie, _ := f.MakeCookie("admin", time.Hour)
	// flip a char in the signature part
	bad := cookie[:len(cookie)-3] + "xyz"
	if _, err := f.ParseCookie(bad); !errors.Is(err, ErrBadCookie) {
		t.Errorf("tampered cookie should ErrBadCookie, got %v", err)
	}
}

func TestCookieExpired(t *testing.T) {
	f := &File{}
	_ = f.SetPassword("admin", "longenoughpw")

	cookie, _ := f.MakeCookie("admin", -time.Hour)
	if _, err := f.ParseCookie(cookie); !errors.Is(err, ErrExpired) {
		t.Errorf("expired cookie should ErrExpired, got %v", err)
	}
}

func TestCookieWrongKey(t *testing.T) {
	f1 := &File{}
	_ = f1.SetPassword("admin", "longenoughpw")
	cookie, _ := f1.MakeCookie("admin", time.Hour)

	f2 := &File{}
	_ = f2.SetPassword("admin", "differentpw")
	// f2 has a different SessionKey, so f1's cookie shouldn't verify.
	if _, err := f2.ParseCookie(cookie); !errors.Is(err, ErrBadCookie) {
		t.Errorf("cookie from different key should ErrBadCookie, got %v", err)
	}
}
