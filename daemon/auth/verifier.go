// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"sync"
)

// ErrInvalidCredentials is returned for a wrong username or password. It deliberately does not say which.
var ErrInvalidCredentials = errors.New("invalid username or password")

// ErrVerifierUnavailable means this build or host cannot check passwords, so nobody can sign in.
var ErrVerifierUnavailable = errors.New("password verification is unavailable: this build has no PAM support " +
	"(set TRANSIVA_ALLOW_INSECURE_AUTH=1 for development only)")

// PasswordVerifier checks a username and password against the system's accounts.
type PasswordVerifier interface {
	Verify(username, password string) error
}

// insecureVerifier is the old behaviour: it only checks that the account exists and ignores the
// password. It is used only when TRANSIVA_ALLOW_INSECURE_AUTH=1 is set explicitly.
type insecureVerifier struct{}

func (insecureVerifier) Verify(username, _ string) error {
	// #nosec G204 -- username is validated by isValidUsername before it reaches a verifier, and "--"
	// prevents it from being interpreted as a flag by id regardless
	if err := exec.Command("id", "--", username).Run(); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// unavailableVerifier fails closed on builds without PAM.
type unavailableVerifier struct{}

func (unavailableVerifier) Verify(string, string) error { return ErrVerifierUnavailable }

var warnInsecureOnce sync.Once

// defaultVerifier picks the verifier for a new AuthManager: real PAM when built in, the explicit
// insecure opt-in for development, otherwise a verifier that refuses every sign-in.
func defaultVerifier() PasswordVerifier {
	if os.Getenv("TRANSIVA_ALLOW_INSECURE_AUTH") == "1" {
		warnInsecureOnce.Do(func() {
			log.Print("auth: WARNING: TRANSIVA_ALLOW_INSECURE_AUTH=1, passwords are NOT checked")
		})
		return insecureVerifier{}
	}
	return platformVerifier()
}
