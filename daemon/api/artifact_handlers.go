// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/zyvorai/transiva/artifact"
)

// artifactSem bounds concurrent verification runs: each one can read many GiB.
var artifactSem = make(chan struct{}, 2)

const artifactMaxBody = 64 << 10

// artifactRoot returns the configured confinement directory, or "" when the
// artifact endpoints are disabled. Every path in a request must resolve (after
// symlinks) to somewhere inside it.
func (s *Server) artifactRoot() string {
	if s.appConfig != nil && s.appConfig.ArtifactRoot != "" {
		return s.appConfig.ArtifactRoot
	}
	return os.Getenv("ARTIFACT_ROOT")
}

// artifactPrologue enforces method, root configuration, concurrency and body
// size, and decodes the JSON request into dst. It returns false when it has
// already written an error response.
func (s *Server) artifactPrologue(w http.ResponseWriter, r *http.Request, dst any) (root string, release func(), ok bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return "", nil, false
	}
	root = s.artifactRoot()
	if root == "" {
		s.errorResponse(w, http.StatusServiceUnavailable, "artifact verification is disabled: set artifact_root (ARTIFACT_ROOT) to the directory holding exports")
		return "", nil, false
	}
	select {
	case artifactSem <- struct{}{}:
	default:
		s.errorResponse(w, http.StatusTooManyRequests, "too many artifact verifications in progress; retry later")
		return "", nil, false
	}
	release = func() { <-artifactSem }
	r.Body = http.MaxBytesReader(w, r.Body, artifactMaxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		release()
		s.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return "", nil, false
	}
	return root, release, true
}

// confine resolves a request path inside root; on failure it answers 400
// without revealing resolved locations.
func (s *Server) confine(w http.ResponseWriter, root, field, p string) (string, bool) {
	if p == "" {
		s.errorResponse(w, http.StatusBadRequest, "%s is required", field)
		return "", false
	}
	real, err := artifact.ConfinePath(root, p)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "%s is not an accessible path inside the artifact root", field)
		return "", false
	}
	return real, true
}

type verifyBundleRequest struct {
	ManifestPath      string `json:"manifest_path"`
	SkipChecksums     bool   `json:"skip_checksums,omitempty"`
	RequireChecksums  bool   `json:"require_checksums,omitempty"`
	AllowBackingFiles bool   `json:"allow_backing_files,omitempty"`
	ReportStrays      bool   `json:"report_strays,omitempty"`
	PublicKeyPath     string `json:"public_key_path,omitempty"`
	SignaturePath     string `json:"signature_path,omitempty"`
}

// handleVerifyBundle verifies an exported bundle against its manifest.
func (s *Server) handleVerifyBundle(w http.ResponseWriter, r *http.Request) {
	var req verifyBundleRequest
	root, release, ok := s.artifactPrologue(w, r, &req)
	if !ok {
		return
	}
	defer release()

	mp, ok := s.confine(w, root, "manifest_path", req.ManifestPath)
	if !ok {
		return
	}
	opt := artifact.BundleOptions{
		Root:              root,
		SkipChecksums:     req.SkipChecksums,
		RequireChecksums:  req.RequireChecksums,
		AllowBackingFiles: req.AllowBackingFiles,
		ReportStrays:      req.ReportStrays,
	}
	if req.PublicKeyPath != "" || req.SignaturePath != "" {
		kp, ok := s.confine(w, root, "public_key_path", req.PublicKeyPath)
		if !ok {
			return
		}
		sp, ok := s.confine(w, root, "signature_path", req.SignaturePath)
		if !ok {
			return
		}
		pub, err := artifact.LoadPublicKey(kp)
		if err != nil {
			s.errorResponse(w, http.StatusBadRequest, "public key is not a valid Ed25519 PEM key")
			return
		}
		opt.PublicKey, opt.SignatureFile = pub, sp
	}
	rep, err := artifact.VerifyBundle(r.Context(), mp, opt)
	if err != nil {
		s.errorResponse(w, http.StatusUnprocessableEntity, "manifest could not be verified: %v", err)
		return
	}
	s.jsonResponse(w, http.StatusOK, rep)
}

type inspectOVARequest struct {
	Path string `json:"path"`
}

// handleInspectOVA inspects an OVA without extracting or executing it.
func (s *Server) handleInspectOVA(w http.ResponseWriter, r *http.Request) {
	var req inspectOVARequest
	root, release, ok := s.artifactPrologue(w, r, &req)
	if !ok {
		return
	}
	defer release()

	p, ok := s.confine(w, root, "path", req.Path)
	if !ok {
		return
	}
	rep, err := artifact.InspectOVA(r.Context(), p, artifact.Limits{})
	if err != nil {
		s.errorResponse(w, http.StatusUnprocessableEntity, "OVA could not be inspected: %v", err)
		return
	}
	s.jsonResponse(w, http.StatusOK, rep)
}

type verifySignatureRequest struct {
	ManifestPath  string `json:"manifest_path"`
	SignaturePath string `json:"signature_path"`
	PublicKeyPath string `json:"public_key_path"`
}

// handleVerifySignature checks a detached manifest signature. Signing is
// deliberately CLI-only: the daemon never accepts private keys.
func (s *Server) handleVerifySignature(w http.ResponseWriter, r *http.Request) {
	var req verifySignatureRequest
	root, release, ok := s.artifactPrologue(w, r, &req)
	if !ok {
		return
	}
	defer release()

	mp, ok := s.confine(w, root, "manifest_path", req.ManifestPath)
	if !ok {
		return
	}
	sp, ok := s.confine(w, root, "signature_path", req.SignaturePath)
	if !ok {
		return
	}
	kp, ok := s.confine(w, root, "public_key_path", req.PublicKeyPath)
	if !ok {
		return
	}
	data, err := artifact.ReadSmallFile(mp, artifact.DefaultLimits().MaxManifestBytes)
	if err != nil {
		s.errorResponse(w, http.StatusUnprocessableEntity, "manifest could not be read: %v", err)
		return
	}
	pub, err := artifact.LoadPublicKey(kp)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "public key is not a valid Ed25519 PEM key")
		return
	}
	sig, err := artifact.LoadSignature(sp)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "signature file is not a valid transiva signature")
		return
	}
	resp := map[string]any{"valid": true, "key_id": artifact.KeyID(pub),
		"scope": "attests the manifest bytes only; disks are checked by /artifacts/verify-bundle"}
	if err := artifact.VerifyManifestSignature(data, sig, pub); err != nil {
		resp["valid"] = false
		resp["error"] = err.Error()
	}
	s.jsonResponse(w, http.StatusOK, resp)
}

type assessRequest struct {
	ManifestPath string `json:"manifest_path"`
}

// handleAssessManifest runs the metadata-only readiness rules over a manifest.
// No disk is opened.
func (s *Server) handleAssessManifest(w http.ResponseWriter, r *http.Request) {
	var req assessRequest
	root, release, ok := s.artifactPrologue(w, r, &req)
	if !ok {
		return
	}
	defer release()

	mp, ok := s.confine(w, root, "manifest_path", req.ManifestPath)
	if !ok {
		return
	}
	m, err := artifact.LoadManifest(mp, artifact.Limits{})
	if err != nil {
		s.errorResponse(w, http.StatusUnprocessableEntity, "manifest could not be assessed: %v", err)
		return
	}
	s.jsonResponse(w, http.StatusOK, artifact.AssessManifest(m))
}

type estimateSpaceRequest struct {
	ManifestPath string `json:"manifest_path"`
	TargetFormat string `json:"target_format,omitempty"`
	FreeBytes    *int64 `json:"free_bytes,omitempty"`
}

// handleEstimateSpace bounds the destination space a conversion of the
// manifest's disks will need. Files are only stat-ed.
func (s *Server) handleEstimateSpace(w http.ResponseWriter, r *http.Request) {
	var req estimateSpaceRequest
	root, release, ok := s.artifactPrologue(w, r, &req)
	if !ok {
		return
	}
	defer release()

	mp, ok := s.confine(w, root, "manifest_path", req.ManifestPath)
	if !ok {
		return
	}
	if req.TargetFormat != "" && req.TargetFormat != "qcow2" && req.TargetFormat != "raw" {
		s.errorResponse(w, http.StatusBadRequest, "target_format must be qcow2 or raw")
		return
	}
	if req.FreeBytes != nil && *req.FreeBytes < 0 {
		s.errorResponse(w, http.StatusBadRequest, "free_bytes must not be negative")
		return
	}
	m, err := artifact.LoadManifest(mp, artifact.Limits{})
	if err != nil {
		s.errorResponse(w, http.StatusUnprocessableEntity, "manifest could not be read: %v", err)
		return
	}
	opt := artifact.EstimateOptions{TargetFormat: req.TargetFormat, FreeBytes: -1, Root: root, ManifestDir: filepath.Dir(mp)}
	if req.FreeBytes != nil {
		opt.FreeBytes = *req.FreeBytes
	}
	est, err := artifact.EstimateSpace(m, opt)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "%v", err)
		return
	}
	s.jsonResponse(w, http.StatusOK, est)
}
