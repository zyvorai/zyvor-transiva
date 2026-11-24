// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/zyvorai/transiva/artifact"
	"github.com/zyvorai/transiva/manifest"
)

// Offline artifact verification commands. They read exported files and compare
// them with what a manifest or OVA manifest recorded; nothing is executed,
// mounted or booted, and no hypervisor is contacted.
var (
	verifyBundleFlag  = flag.String("verify-bundle", "", "Verify an exported bundle offline: path to an Artifact Manifest (checks sizes, formats, SHA-256; never executes artifacts)")
	inspectOVAFlag    = flag.String("inspect-ova", "", "Inspect an OVA offline: entry names, OVF references and .mf digests (no extraction, nothing executed)")
	signManifestFlag  = flag.String("sign-manifest", "", "Sign an Artifact Manifest with an Ed25519 key (needs -sign-key; writes <manifest>.sig unless -sig-file is set)")
	verifySigFlag     = flag.String("verify-signature", "", "Verify a manifest's detached signature (needs -pub-key; -sig-file defaults to <manifest>.sig)")
	genSigningKeyFlag = flag.String("gen-signing-key", "", "Generate an Ed25519 signing key pair at <prefix>.key / <prefix>.pub")
	signKeyFlag       = flag.String("sign-key", "", "Private key file for -sign-manifest (PKCS#8 PEM, mode 0600)")
	pubKeyFlag        = flag.String("pub-key", "", "Public key file for -verify-signature / -verify-bundle")
	sigFileFlag       = flag.String("sig-file", "", "Detached signature file (default <manifest>.sig)")
	bundleRootFlag    = flag.String("bundle-root", "", "Confine disk paths in -verify-bundle to this directory")
	requireChecksums  = flag.Bool("require-checksums", false, "-verify-bundle: treat a disk without a checksum as an error")
	allowBacking      = flag.Bool("allow-backing-files", false, "-verify-bundle: downgrade 'disk has a backing file' to a warning")
	skipChecksumsFlag = flag.Bool("skip-checksums", false, "-verify-bundle: check structure and headers only, do not hash disks")
	maxDiskBytesFlag  = flag.Int64("max-disk-bytes", 0, "Per-disk / per-OVA-entry size cap in bytes (default 16 TiB)")
	buildManifestFlag = flag.String("build-manifest", "", "Create an Artifact Manifest for disk files in this directory (formats from headers, SHA-256 computed; writes to -manifest-out)")
	manifestOutFlag   = flag.String("manifest-out", "", "Output file for -build-manifest (default <dir>/artifact-manifest.json; never overwrites)")
	vmNameFlag        = flag.String("manifest-vm-name", "", "VM name recorded by -build-manifest")
	diffManifestsFlag = flag.String("diff-manifests", "", "Compare this manifest with the one given by -diff-against (exit 1 when they differ)")
	diffAgainstFlag   = flag.String("diff-against", "", "Second manifest for -diff-manifests")
	diffPathsFlag     = flag.Bool("diff-paths", false, "-diff-manifests: also compare local_path")
	writeSumsFlag     = flag.String("write-sha256sums", "", "Print a manifest's disk checksums in sha256sum -c format")
	jsonOutFlag       = flag.Bool("json", false, "Print artifact verification results as JSON")
)

// runArtifactCommands handles the offline artifact flags. It returns
// handled=true when one was present, along with the process exit code
// (0 pass, 1 verification failed, 2 usage or I/O error).
func runArtifactCommands(stdout, stderr io.Writer) (handled bool, code int) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	lim := artifact.Limits{MaxDiskBytes: *maxDiskBytesFlag}

	switch {
	case *genSigningKeyFlag != "":
		priv, pub, err := artifact.GenerateKeyPair(*genSigningKeyFlag)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		outf(stdout, "private key: %s (keep secret)\npublic key:  %s\n", priv, pub)
		return true, 0

	case *signManifestFlag != "":
		if *signKeyFlag == "" {
			outln(stderr, "Error: -sign-manifest needs -sign-key")
			return true, 2
		}
		priv, err := artifact.LoadPrivateKey(*signKeyFlag)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		data, err := readManifestBytes(*signManifestFlag, lim)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		sig, err := artifact.SignManifest(data, priv)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		out := sigPath(*signManifestFlag)
		if err := artifact.WriteSignature(out, sig); err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		outf(stdout, "signed %s with key %s\nsignature: %s\n", *signManifestFlag, sig.KeyID, out)
		return true, 0

	case *verifySigFlag != "":
		if *pubKeyFlag == "" {
			outln(stderr, "Error: -verify-signature needs -pub-key")
			return true, 2
		}
		pub, err := artifact.LoadPublicKey(*pubKeyFlag)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		data, err := readManifestBytes(*verifySigFlag, lim)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		sig, err := artifact.LoadSignature(sigPath(*verifySigFlag))
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		if err := artifact.VerifyManifestSignature(data, sig, pub); err != nil {
			outf(stdout, "FAIL: %v\n", err)
			return true, 1
		}
		outf(stdout, "OK: signature valid for key %s. This attests the manifest bytes only; run -verify-bundle to check the disks.\n", artifact.KeyID(pub))
		return true, 0

	case *verifyBundleFlag != "":
		opt := artifact.BundleOptions{
			Root:              *bundleRootFlag,
			SkipChecksums:     *skipChecksumsFlag,
			RequireChecksums:  *requireChecksums,
			AllowBackingFiles: *allowBacking,
			Limits:            lim,
		}
		if *pubKeyFlag != "" {
			pub, err := artifact.LoadPublicKey(*pubKeyFlag)
			if err != nil {
				outf(stderr, "Error: %v\n", err)
				return true, 2
			}
			opt.PublicKey, opt.SignatureFile = pub, sigPath(*verifyBundleFlag)
		}
		rep, err := artifact.VerifyBundle(ctx, *verifyBundleFlag, opt)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		if *jsonOutFlag {
			printJSON(stdout, rep)
		} else {
			outf(stdout, "manifest sha256: %s\n", rep.ManifestSHA256)
			for _, d := range rep.Disks {
				state := "not hashed"
				if d.ChecksumChecked {
					state = map[bool]string{true: "checksum OK", false: "CHECKSUM MISMATCH"}[d.ChecksumMatch]
				}
				outf(stdout, "  disk %-12s %-6s %12d bytes  %s\n", d.ID, d.DeclaredFormat, d.FileBytes, state)
			}
			printFindings(stdout, rep.Findings)
			printVerdict(stdout, rep.OK, artifact.Disclaimer)
		}
		return true, boolCode(rep.OK)

	case *buildManifestFlag != "":
		m, fs, err := artifact.BuildManifest(ctx, *buildManifestFlag, artifact.BuildOptions{
			Provider: *providerType, VMName: *vmNameFlag, ExportMethod: "external", SkipChecksums: *skipChecksumsFlag, Limits: lim,
		})
		if err != nil {
			printFindings(stderr, fs)
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		out := *manifestOutFlag
		if out == "" {
			out = filepath.Join(*buildManifestFlag, "artifact-manifest.json")
		}
		data, err := manifest.ToJSON(m)
		if err == nil {
			err = writeNewFile(out, data)
		}
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		outf(stdout, "wrote %s (%d disks)\n", out, len(m.Disks))
		printFindings(stdout, fs)
		return true, 0

	case *diffManifestsFlag != "":
		if *diffAgainstFlag == "" {
			outln(stderr, "Error: -diff-manifests needs -diff-against")
			return true, 2
		}
		a, err := loadManifestFile(*diffManifestsFlag, lim)
		if err == nil {
			var b *manifest.ArtifactManifest
			if b, err = loadManifestFile(*diffAgainstFlag, lim); err == nil {
				d := artifact.DiffManifests(a, b, *diffPathsFlag)
				if *jsonOutFlag {
					printJSON(stdout, d)
				} else if d.Identical {
					outln(stdout, "manifests are equivalent")
				} else {
					for _, c := range d.Changes {
						outf(stdout, "  %-8s %s %s %s -> %s\n", c.Kind, c.Subject, c.Field, c.Old, c.New)
					}
				}
				return true, boolCode(d.Identical)
			}
		}
		outf(stderr, "Error: %v\n", err)
		return true, 2

	case *writeSumsFlag != "":
		m, err := loadManifestFile(*writeSumsFlag, lim)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		skipped, err := artifact.WriteSHA256Sums(stdout, m, filepath.Dir(*writeSumsFlag))
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		if len(skipped) > 0 {
			outf(stderr, "note: no checksum recorded for disks: %s\n", strings.Join(skipped, ", "))
		}
		return true, 0

	case *inspectOVAFlag != "":
		rep, err := artifact.InspectOVA(ctx, *inspectOVAFlag, lim)
		if err != nil {
			outf(stderr, "Error: %v\n", err)
			return true, 2
		}
		if *jsonOutFlag {
			printJSON(stdout, rep)
		} else {
			if rep.VirtualSystem != "" {
				outf(stdout, "virtual system: %s\n", rep.VirtualSystem)
			}
			for _, e := range rep.Entries {
				outf(stdout, "  %-40s %14d bytes\n", e.Name, e.Bytes)
			}
			outf(stdout, "manifest (.mf) found: %v, all listed digests verified: %v, certificate present (not verified): %v\n",
				rep.ManifestFound, rep.ManifestOK, rep.CertPresent)
			printFindings(stdout, rep.Findings)
			printVerdict(stdout, rep.OK, artifact.Disclaimer)
		}
		return true, boolCode(rep.OK)
	}
	return false, 0
}

func readManifestBytes(path string, lim artifact.Limits) ([]byte, error) {
	max := lim.MaxManifestBytes
	if max <= 0 {
		max = artifact.DefaultLimits().MaxManifestBytes
	}
	return artifact.ReadSmallFile(path, max)
}

func loadManifestFile(path string, lim artifact.Limits) (*manifest.ArtifactManifest, error) {
	data, err := readManifestBytes(path, lim)
	if err != nil {
		return nil, err
	}
	var m *manifest.ArtifactManifest
	if l := strings.ToLower(path); strings.HasSuffix(l, ".yaml") || strings.HasSuffix(l, ".yml") {
		m, err = manifest.FromYAML(data)
	} else {
		m, err = manifest.FromJSON(data)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := manifest.ValidateSchema(m); err != nil {
		return nil, fmt.Errorf("invalid manifest %s: %w", path, err)
	}
	return m, nil
}

// writeNewFile writes data to path, refusing to overwrite an existing file.
func writeNewFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302,G304 -- operator-chosen output path
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func sigPath(manifestPath string) string {
	if *sigFileFlag != "" {
		return *sigFileFlag
	}
	return manifestPath + ".sig"
}

func printFindings(w io.Writer, fs []artifact.Finding) {
	for _, f := range fs {
		subj := ""
		if f.Subject != "" {
			subj = " [" + f.Subject + "]"
		}
		outf(w, "  %-7s %s%s: %s\n", strings.ToUpper(f.Severity), f.Code, subj, f.Message)
	}
}

func printVerdict(w io.Writer, ok bool, disclaimer string) {
	v := "FAIL"
	if ok {
		v = "PASS"
	}
	outf(w, "%s\n%s\n", v, disclaimer)
}

func printJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func boolCode(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

// outf and outln write to a CLI stream; a failed write to stdout or stderr has
// no useful recovery, so the error is dropped deliberately.
func outf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
func outln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }
