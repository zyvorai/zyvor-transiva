// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

//go:build linux && cgo && pam

package auth

import (
	"errors"
	"log"
	"os"

	"github.com/msteinert/pam/v2"
)

// pamService is the PAM service stack used to check passwords: $TRANSIVA_PAM_SERVICE, else
// "transiva" when /etc/pam.d/transiva exists, else the system "login" stack.
func pamService() string {
	if s := os.Getenv("TRANSIVA_PAM_SERVICE"); s != "" {
		return s
	}
	if _, err := os.Stat("/etc/pam.d/transiva"); err == nil {
		return "transiva"
	}
	return "login"
}

type pamVerifier struct{ service string }

func platformVerifier() PasswordVerifier { return pamVerifier{service: pamService()} }

// Verify runs pam_authenticate and pam_acct_mgmt for the user. The daemon must run as root (or be
// otherwise able to read the shadow database) for pam_unix to accept real passwords.
func (v pamVerifier) Verify(username, password string) error {
	tx, err := pam.StartFunc(v.service, username, func(style pam.Style, msg string) (string, error) {
		switch style {
		case pam.PromptEchoOff:
			return password, nil
		case pam.PromptEchoOn:
			return username, nil
		default:
			return "", nil // informational and error messages need no answer
		}
	})
	if err != nil {
		log.Printf("auth: cannot start PAM service %q: %v", v.service, err)
		return ErrVerifierUnavailable
	}
	defer func() { _ = tx.End() }()

	if err := tx.Authenticate(0); err != nil {
		if errors.Is(err, pam.ErrAuthinfoUnavail) || errors.Is(err, pam.ErrService) {
			log.Printf("auth: PAM service %q is not usable: %v", v.service, err)
			return ErrVerifierUnavailable
		}
		return ErrInvalidCredentials
	}
	if err := tx.AcctMgmt(0); err != nil {
		return ErrInvalidCredentials // expired, locked or disabled account
	}
	return nil
}
