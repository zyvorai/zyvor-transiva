# Case study: a Linux VM out of Nutanix in one command

One script takes a Linux disk in Nutanix's format, pulls it out through the Prism v4 API, converts it and boots it on plain Linux/KVM. No Nutanix tooling is needed on the destination.

`scripts/e2e-nutanix-exit.sh user@host` runs the whole thing unattended on a Linux host.

## What ran

| Stage | What happens | Result |
|---|---|---|
| Download | Newest Alpine Linux cloud qcow2 (the format AHV image services use), resolved from the mirror | passed, 0 s (cached) |
| Serve | Chimera's Nutanix persona publishes it as VM `chimera-vm-01` on Prism v4 (`:8990`, TLS), with the disk in the container's `.acropolis/vmdisk/<uuid>/` | passed, 1 s |
| Export | `hyperexport --provider nutanix -resolve-containers`: version negotiation, VM list, NFS pickup, `qemu-img convert`, `artifact-manifest.json` | passed, 1 s |
| Convert | `h2kvmctl --cmd local ... --emit-domain-xml --checksum` | passed, 198 s (the host was under heavy load; 47 s in an earlier run) |
| Boot | `virt-install --import` on libvirt/KVM; the script waits for a DHCP lease, then ping | passed, 35 s; 3/3 ping replies, 0% loss |

The script finished with 6 stages passed, 0 failed, in 235 s.

## Why it is easy

- The source VM is never modified. Export reads the disk through the storage container and writes a standard qcow2.
- The artifact is a plain qcow2 plus a manifest, so any hypervisor can use it.
- Nothing proprietary is installed on the destination. libvirt/KVM is enough.
- The same artifact also booted on KubeVirt in an earlier manual run.

## Limits

- **The Prism endpoint is Chimera's simulator, not a real Nutanix cluster.** Real error bodies and NFS-mounted containers have not been exercised.
- **The guest is a small Alpine image.** Windows, UEFI, multi-disk and large VMs belong to h2kvm and were not run here.
- **h2kvm's repair step was skipped** (`No filesystem mounted`), so the boot above does not show that repair works. The script prints a WARN when this happens.
- **The check inside the guest is ping.** There is no deeper application check.
- **KubeVirt landing is not part of the script.** It was tested only by hand.

## Run it

```bash
./scripts/e2e-nutanix-exit.sh sus@host        # or run on the host directly
DISK_URL=https://.../your-disk.qcow2 ./scripts/e2e-nutanix-exit.sh sus@host
```
