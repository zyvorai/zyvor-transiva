// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

//go:build !(linux && cgo && pam)

package auth

func platformVerifier() PasswordVerifier { return unavailableVerifier{} }
