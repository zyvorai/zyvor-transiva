# Offline artifact verification

Three read-only checks for exported artifacts. They answer "is what I received
what was exported?" without a hypervisor, a guest agent or any execution.

**What a pass means.** The files on disk match what the manifest (or OVA
manifest) recorded, and file headers agree with the declared formats. It does
not mean the guest boots, is free of malware, or that the recorded checksums
were taken from the original source disk. Transiva never executes, mounts,
boots or converts the artifacts; `.cert` signatures inside OVAs are reported
but not verified.

## CLI (`transivaexport` / `hyperexport`)

Handled before any provider connection. Exit codes: 0 pass, 1 verification
failed, 2 usage or I/O error. Add `-json` for machine-readable output.

```bash
# Verify disks listed in a manifest (sizes, SHA-256, header vs declared format,
# backing files / extents, duplicate paths). Every disk is checked.
hyperexport -verify-bundle /exports/vm1/artifact-manifest.json
hyperexport -verify-bundle m.json -bundle-root /exports -require-checksums

# Inspect an OVA without extracting it
hyperexport -inspect-ova /exports/vm1.ova

# Sign / verify the manifest (Ed25519, PKCS#8 / PKIX PEM)
hyperexport -gen-signing-key /etc/transiva/export-signing     # writes .key (0600) and .pub
hyperexport -sign-manifest m.json -sign-key export-signing.key  # writes m.json.sig
hyperexport -verify-signature m.json -pub-key export-signing.pub
hyperexport -verify-bundle m.json -pub-key export-signing.pub   # signature + disks
```

### Build, compare and export checksums

For exports made by other tools (ovftool, govc, a copied bundle) and for
comparing two exports:

```bash
# Create a manifest for disk files in a directory (formats from headers, not
# extensions; size = header virtual size when known; SHA-256 computed;
# relative paths; refuses to overwrite). -provider / -manifest-vm-name are optional.
hyperexport -build-manifest /exports/vm1 -manifest-out /exports/vm1/artifact-manifest.json

# Compare two manifests by disk ID (format, bytes, checksum, boot order, type).
# Exit 0 equivalent, 1 different. -diff-paths also compares local_path.
hyperexport -diff-manifests a.json -diff-against b.json

# Emit checksums in `sha256sum -c` format so any tool can re-verify
cd /exports/vm1 && hyperexport -write-sha256sums artifact-manifest.json | sha256sum -c
```

`-build-manifest` skips files whose extension has no matching header (for
example `-flat.vmdk` extents) and says so; it does not know which disk boots,
so `boot_order_hint` follows file-name order. These three are CLI-only.

Other flags: `-sig-file`, `-skip-checksums`, `-allow-backing-files`,
`-max-disk-bytes`.

### Pre-flight, space estimate, strays and checksum fill

```bash
# Readiness hints from manifest metadata only (no disk is read). Exit 0 for
# ready or conditionally_ready, 1 for not_ready; -assess-fail-on warning also
# fails on conditionally_ready.
hyperexport -assess-manifest m.json [-assess-fail-on warning] [-json]

# Bound the destination space a conversion needs (bounds, not a measurement;
# qemu-img measure on the receiving side is authoritative). Exit 1 = does not fit.
hyperexport -estimate-space m.json -estimate-target qcow2 -estimate-free-dir /var/lib/libvirt/images
hyperexport -estimate-space m.json -estimate-target raw -estimate-free-bytes 500000000000

# Also list files next to the manifest that no disk references
hyperexport -verify-bundle m.json -report-strays

# Add SHA-256 to disks that have none (never replaces an existing checksum;
# atomic in-place rewrite, or -manifest-out for a new file). This records the
# files as they are now and invalidates any <manifest>.sig, so sign again.
hyperexport -fill-checksums m.json [-manifest-out m-filled.json]
```

`-assess-manifest` rules: UEFI and Secure Boot, Windows guests (VirtIO
drivers), unknown firmware or OS, multiple disks and an unmarked boot disk,
disks needing conversion, disks over 2 TiB (warning) or 16 TiB (error), missing
checksums, NIC MAC problems (duplicate is an error), non-DNS-1123 VM names, and
warnings recorded during export.

## REST

Disabled unless `artifact_root` is configured (environment `ARTIFACT_ROOT`).
Every path in a request, after symlink resolution, must lie inside that
directory; otherwise the call is rejected with 400. At most two verifications
run at once (429 otherwise). Request bodies are capped at 64 KiB and unknown
JSON fields are rejected. Signing is CLI-only: the daemon never accepts a
private key.

| Endpoint | Purpose |
|---|---|
| `POST /artifacts/verify-bundle` | Manifest plus disks; optional `public_key_path` + `signature_path` |
| `POST /artifacts/inspect-ova` | OVA entries, OVF references, `.mf` digests |
| `POST /artifacts/verify-signature` | Detached manifest signature only |
| `POST /artifacts/assess` | Metadata-only readiness assessment of a manifest |
| `POST /artifacts/estimate-space` | Destination space bounds; optional `free_bytes` for a fit verdict |

`verify-bundle` also accepts `report_strays`. `-fill-checksums` writes files and is CLI-only.

Schemas: [openapi.yaml](../openapi.yaml). Failed checks are `findings` in a 200
response (`ok: false`); HTTP errors are for bad requests only.

## Hostile-input behaviour

| Input | Result |
|---|---|
| Manifest over 8 MiB, FIFO, directory, NUL in path | refused before parsing |
| Disk path outside `artifact_root` (`..`, absolute, symlink) | `path_outside_root`, file not opened |
| Disk over `-max-disk-bytes` (default 16 TiB) | `file_too_large`, not hashed |
| Header says qcow2, manifest says raw (or any mismatch) | `format_mismatch` |
| qcow2 backing file / VMDK parent hint | `backing_file` (error unless `-allow-backing-files`) |
| VMDK descriptor extent outside the bundle | `extent_path` / `extent_not_covered` |
| OVA entry with `..`, absolute path, subdirectory, symlink, hardlink, device | `entry_name` / `entry_type`, never extracted |
| OVA duplicate (case-insensitive) names, too many entries, oversized entry | `duplicate_entry`, `too_many_entries`, `entry_too_large` |
| OVA gzip-compressed, truncated, not a tar | `compressed` (not decompressed), `tar_corrupt` |
| OVF with entity-expansion DOCTYPE | not expanded; descriptor over cap refused |
| `.mf` naming `../x`, short digests, files not in the archive | `mf_name`, `mf_digest_length`, `mf_file_missing` |
| Modified manifest after signing | `signature_invalid` |
| Unlisted files next to the manifest (with `-report-strays`) | `stray_disk_file` (warning), `stray_file` / `stray_symlink` (info); symlinks are never followed; scan stops at 10000 entries |

Package: [`artifact/`](../artifact). Background and alternatives considered:
[RESEARCH.md](RESEARCH.md).
