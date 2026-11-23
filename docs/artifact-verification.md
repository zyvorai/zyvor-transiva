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

Other flags: `-sig-file`, `-skip-checksums`, `-allow-backing-files`,
`-max-disk-bytes`.

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

Package: [`artifact/`](../artifact). Background and alternatives considered:
[RESEARCH.md](RESEARCH.md).
