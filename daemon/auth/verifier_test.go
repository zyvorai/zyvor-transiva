// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"os"
	"testing"
)

// fakeVerifier accepts exactly the user/password pairs it was given.
type fakeVerifier struct{ accept map[string]string }

func (f fakeVerifier) Verify(user, pass string) error {
	if want, ok := f.accept[user]; ok && want == pass {
		return nil
	}
	return ErrInvalidCredentials
}

// fakeAnyPassword accepts every password (the old behaviour), for tests that are not about passwords.
type fakeAnyPassword struct{}

func (fakeAnyPassword) Verify(string, string) error { return nil }

func currentTestUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "root"
}

func TestWrongPasswordIsRejected(t *testing.T) {
	am := NewAuthManager()
	am.SetVerifier(fakeVerifier{accept: map[string]string{"alice": "right"}})

	if _, err := am.AuthenticateUser("alice", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: err = %v, want ErrInvalidCredentials", err)
	}
	s, err := am.AuthenticateUser("alice", "right")
	if err != nil || s == nil || s.Token == "" {
		t.Fatalf("right password should sign in, got %v, %v", s, err)
	}
}

func TestUnknownUserIsRejectedLikeAWrongPassword(t *testing.T) {
	am := NewAuthManager()
	am.SetVerifier(fakeVerifier{accept: map[string]string{"alice": "right"}})
	if _, err := am.AuthenticateUser("mallory", "x"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestVerifierUnavailableFailsClosed(t *testing.T) {
	am := NewAuthManager()
	am.SetVerifier(unavailableVerifier{})
	s, err := am.AuthenticateUser("alice", "anything")
	if !errors.Is(err, ErrVerifierUnavailable) || s != nil {
		t.Fatalf("got %v, %v; want ErrVerifierUnavailable and no session", s, err)
	}
}

func TestRepeatedFailuresLockTheAccount(t *testing.T) {
	am := NewAuthManager()
	am.SetVerifier(fakeVerifier{accept: map[string]string{"alice": "right"}})
	for i := 0; i < maxLoginFailures; i++ {
		_, _ = am.AuthenticateUser("alice", "wrong")
	}
	// Even the right password is refused while locked.
	if _, err := am.AuthenticateUser("alice", "right"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("err = %v, want ErrTooManyAttempts", err)
	}
	// Another account is unaffected.
	am.SetVerifier(fakeVerifier{accept: map[string]string{"alice": "right", "bob": "pw"}})
	if _, err := am.AuthenticateUser("bob", "pw"); err != nil {
		t.Fatalf("bob should not be locked: %v", err)
	}
}

func TestSuccessfulLoginClearsFailures(t *testing.T) {
	am := NewAuthManager()
	am.SetVerifier(fakeVerifier{accept: map[string]string{"alice": "right"}})
	for i := 0; i < maxLoginFailures-1; i++ {
		_, _ = am.AuthenticateUser("alice", "wrong")
	}
	if _, err := am.AuthenticateUser("alice", "right"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxLoginFailures-1; i++ {
		_, _ = am.AuthenticateUser("alice", "wrong")
	}
	if _, err := am.AuthenticateUser("alice", "right"); err != nil {
		t.Fatalf("failures before a successful login must not count toward the lockout: %v", err)
	}
}

func TestInsecureModeNeedsAnExplicitOptIn(t *testing.T) {
	t.Setenv("TRANSIVA_ALLOW_INSECURE_AUTH", "")
	if _, ok := defaultVerifier().(insecureVerifier); ok {
		t.Fatal("the user-exists-only verifier must not be the default")
	}
	t.Setenv("TRANSIVA_ALLOW_INSECURE_AUTH", "1")
	if _, ok := defaultVerifier().(insecureVerifier); !ok {
		t.Fatal("TRANSIVA_ALLOW_INSECURE_AUTH=1 should select the development verifier")
	}
}
