# Research: VM migration and workload mobility tooling

Purpose: find what comparable tools do after an export lands on disk, compare
that with what Transiva does today, and rank ideas that fit the product's
identity: **honest, offline export**. Transiva discovers and exports VMs from
vSphere and Nutanix AHV as jobs, writes an Artifact Manifest v1.0 (disk list,
format, size, optional SHA-256) and hands artifacts to h2kvm. It installs no
guest agent and does not convert, boot or repair guests.

Marking: a claim is sourced when it was read from the fetched URL listed in
its row. **[U]** means it is not from a page fetched during this work
(background knowledge, a page that returned 403/404, or a search snippet) and
should be re-checked before it is quoted externally.

## What the field does

| Tool | What it does (relevant parts) | Source |
|---|---|---|
| virt-v2v | Converts one guest from VMware (vCenter/ESXi, VMX over NFS/SSH, **OVA**), Xen or Hyper-V to KVM; installs virtio drivers for Windows from a `VIRTIO_WIN` location; maps NICs; optional `--fsck` before and after; checks free space; **for OVA input it checks VMDK checksums against the manifest**; outputs to files, libvirt, oVirt, OpenStack, KubeVirt (experimental). | https://libguestfs.org/virt-v2v.1.html |
| Forklift (upstream of Red Hat MTV) | Plan-based migration to KubeVirt: credentials, infrastructure maps, a plan, then run. **Validations flag VMs and plan issues before running.** Cold migration for all providers; warm (CBT or incremental backup) for VMware and oVirt. Providers: VMware, oVirt, OpenStack, Nutanix, Hyper-V, KubeVirt. | https://github.com/kubev2v/forklift |
| Proxmox VE import | ESXi import-source storage and wizard (resulting-config preview, per-disk storage, optional live import); `qm importovf` for `ovftool` exports; `qm disk import` for loose VMDKs. Verification is manual: confirm disks appear, check the resulting config, boot and fix drivers. | https://pve.proxmox.com/wiki/Migrate_to_Proxmox_VE |
| AWS Transform MGN | Agent-based continuous block replication; test instances before cutover; lifecycle states (Not ready, Ready for testing, Ready for cutover, Cutover complete); applications and waves; post-launch actions; "finalize cutover" after the operator confirms the instance works. | https://docs.aws.amazon.com/mgn/latest/ug/what-is-application-migration-service.html |
| qemu-img | `check` (consistency, exit codes), `info` (format, virtual size, **backing file**, snapshots, JSON), `compare` (guest-visible equality across formats), `convert`. Warns that format is usually probed (specify it explicitly) and that backing files are opened and trusted. | https://qemu-project.gitlab.io/qemu/tools/qemu-img.html |
| Veeam Instant Recovery | Runs a VM directly from backup storage, later moved to production storage. The fetched page covered restore-target planning only; **SureBackup-style recovery verification (booting restored VMs and testing them) is [U]**. | https://www.veeam.com/blog/instant-vm-recovery.html |
| Azure Migrate (agentless VMware) | Replication, test migration, cutover. The documentation page returned 404, so everything about it is **[U]**. | (not read) |
| OVF/OVA (DMTF) | Package of descriptor (`.ovf`), optional `.mf` manifest of SHA digests and optional `.cert` signature in a tar; descriptor first. The DMTF page returned 403, so the exact layout rules here are **[U]**; virt-v2v's OVA checksum behaviour above is sourced. | https://www.dmtf.org/standards/ovf (not read) |
| OCI image export of VMs, disk-format conversion pipelines | Not researched beyond the above; **[U]**. | |

## What Transiva has and lacks

Has: provider discovery, export jobs (CLI and REST), resumable jobs, Artifact
Manifest v1.0 with optional per-disk SHA-256, schema validation, a
`VerifyChecksums` helper, h2kvm hand-off.

Lacked before this work (all offline, all within the honesty boundary):

1. A single command or endpoint to **verify a finished export bundle** against
   its manifest the way a receiver (h2kvm, a second site, an auditor) would.
   `VerifyChecksums` existed but stopped at the first mismatch, could panic on
   a short checksum value, and did not look at headers, sizes or paths.
2. Any check that the **disk header agrees with the declared format**, or that a
   qcow2/VMDK does not point at a **backing file or extent outside the bundle**
   (qemu-img documents that these are opened and trusted).
3. **OVA/OVF inspection**: vSphere exports are `.ovf`/`.ova` (the CLI's default
   `-format ovf|ova`), but nothing checked an OVA's `.mf` digests, descriptor
   references or hostile tar entries.
4. **Origin integrity for the manifest**: a checksum list proves nothing if the
   manifest itself can be swapped; there was no signature.
5. Not available at all: post-migration validation (boot tests), driver
   injection, conversion. These belong to h2kvm and GuestKit by design.

## Ranked ideas

Ranking weighs fit with "honest offline export", value to a receiver of the
artifacts, and testability without a hypervisor.

| # | Idea | Rationale | Decision |
|---|---|---|---|
| 1 | Bundle verification: manifest vs files (sizes, SHA-256, header-vs-declared format, backing files, extents, path confinement, caps) | Direct analogue of virt-v2v's OVA checksum check and of qemu-img `info` backing-chain awareness; closes the "export finished, is it intact?" question with no agent and no execution. | **Built** (`artifact.VerifyBundle`, `-verify-bundle`, `POST /artifacts/verify-bundle`) |
| 2 | OVA/OVF inspection with `.mf` digest verification | vSphere exports arrive as OVA; virt-v2v verifies OVA checksums, we did not. Hostile-tar handling is the main risk, so it is worth owning. | **Built** (`artifact.InspectOVA`, `-inspect-ova`, `POST /artifacts/inspect-ova`) |
| 3 | Ed25519 detached manifest signatures | Gives the manifest an origin, so checksum lists are tamper-evident across hand-offs; stdlib only, no new dependency, no key handling in the daemon. | **Built** (`-gen-signing-key`, `-sign-manifest`, `-verify-signature`, `POST /artifacts/verify-signature`) |
| 4 | Fix `VerifyChecksums` / manifest loading weaknesses found on the way | Panic on short checksum, first-mismatch stop, unbounded manifest read, unbounded workflow request bodies. | **Fixed** |
| 5 | Pre-flight "plan validation" in the style of Forklift | Valuable, but it needs live provider inventory to be meaningful (vCenter or Prism), which cannot be tested here. | Deferred: needs hypervisors |
| 6 | Chunk-level digests to resume a verification or re-fetch from the first bad chunk | Natural next step for "resumable transfers" (verify can resume instead of re-hashing). Needs a manifest schema extension and agreement with h2kvm. | Deferred: schema change |
| 7 | `qemu-img check/compare` integration | Strong consistency check, but it means executing a parser (`qemu-img`) on untrusted images, which the "never execute exported artifacts" boundary excludes for this layer; h2kvm already owns it. | Rejected for transiva; belongs in h2kvm |
| 8 | Boot-test / SureBackup-style validation, driver injection | Requires a hypervisor and mutates guests. Out of scope for an export tool; h2kvm/GuestKit. | Rejected: outside identity |
| 9 | Agent-based continuous replication (MGN style) | Contradicts "0 guest agents". | Rejected |
| 10 | X.509 `.cert` verification for OVA | Needs trust-store policy; reported as "present, not verified" instead of implying assurance. | Deferred |

## Honesty notes for the built features

* A pass means bytes match what was recorded and headers agree with declared
  formats. It does **not** mean the guest boots, is clean, or that checksums
  were recorded from the original source disk. Every report carries this
  disclaimer.
* A signature attests the manifest bytes to a key holder. Whether that key is
  trustworthy is the verifier's decision.
* Nothing is executed, mounted, booted or converted. Gzip-compressed OVAs are
  refused rather than decompressed. Header sniffing reads at most 64 KiB plus
  the last 512 bytes of each disk.

## Sources read

* https://libguestfs.org/virt-v2v.1.html
* https://github.com/kubev2v/forklift
* https://pve.proxmox.com/wiki/Migrate_to_Proxmox_VE
* https://docs.aws.amazon.com/mgn/latest/ug/what-is-application-migration-service.html
* https://qemu-project.gitlab.io/qemu/tools/qemu-img.html
* https://www.veeam.com/blog/instant-vm-recovery.html (partial relevance only)

Not read (404/403): Azure Migrate tutorial, DMTF OVF page.
