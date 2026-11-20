// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-1.0

//go:build !(linux && cgo && pam)

package auth

func platformVerifier() PasswordVerifier { return unavailableVerifier{} }
