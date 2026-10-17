# Case study: a Linux VM out of Nutanix in one command

One script takes a stock Ubuntu 24.04 cloud image, serves it as a Nutanix VM, pulls it out through the Prism v4 API, converts it and boots it on plain Linux/KVM and on KubeVirt. No Nutanix tooling is needed on the destination.

`scripts/e2e-nutanix-exit.sh user@host` runs the whole thing unattended on a Linux host.

## What ran

| Stage | What happens | Result |
|---|---|---|
| Download | The Ubuntu 24.04 (noble) cloud image from `cloud-images.ubuntu.com` | passed, 0 s (cached) |
| Serve | Chimera's Nutanix persona publishes it as VM `chimera-vm-01` on Prism v4 (TLS), with the disk in the container's `.acropolis/vmdisk/<uuid>/` | passed, 13 s |
| Export | `hyperexport --provider nutanix -resolve-containers`: version negotiation, VM list, NFS pickup, `qemu-img convert`, `artifact-manifest.json` | passed, 16 s |
| Convert | `h2kvmctl --cmd local ... --emit-domain-xml --checksum`: GuestKit repair, qcow2, checksum, libvirt domain XML (UEFI/OVMF) | passed, 277 s |
| Boot on KVM | The script defines the domain from the XML h2kvm emitted and waits for a DHCP lease and ping | passed, 79 s |
| Boot on KubeVirt | containerDisk build and import, `VirtualMachine`, wait for Ready and ping | passed, 335 s |

7 of 7 stages passed in 747 s (`docs/marketing/results.json`).

## What the run found

The first Ubuntu runs failed, and the failures were real:

- **The guest booted but had no network.** A stock Ubuntu cloud image ships no netplan or networkd config and relies on cloud-init. After a hypervisor exit there is no datasource, no cloud-init stage ran, and `systemd-networkd-wait-online` failed. GuestKit reported "no automated repairs required".
- **The fix is in h2kvm:** when a guest arrives with no network configuration, h2kvm now installs a DHCP-on-ethernet fallback (`/etc/netplan/60-h2kvm-dhcp.yaml`) after GuestKit's repair. The log of the passing run shows it installed.
- **GuestKit could not see Alpine's root filesystem** because its `exists` check followed absolute symlinks on the host (`/bin/sh -> /bin/busybox`). Fixed in GuestKit.

## Why it is easy

- The source VM is never modified. Export reads the disk through the storage container and writes a standard qcow2.
- The artifact is a plain qcow2 plus a manifest, so any hypervisor can use it.
- Nothing proprietary is installed on the destination. libvirt/KVM is enough.
- The same artifact booted on KubeVirt.

## Limits

- **The Prism endpoint is Chimera's simulator, not a real Nutanix cluster.** Real error bodies and NFS-mounted containers have not been exercised.
- **One guest.** Windows, multi-disk and large VMs belong to h2kvm and were not run here.
- **The Ubuntu image needed only the network fallback.** A guest that needs boot repair (bootloader, initramfs, drivers) was not part of this run.
- **The check inside the guest is ping.** There is no deeper application check.
- **Kairon and Machina landings were not run.** Zorvia was checked by hand in an earlier KubeVirt run.

## Run it

```bash
./scripts/e2e-nutanix-exit.sh user@host                 # or run on the host directly
DISK_URL=https://.../your-disk.qcow2 ./scripts/e2e-nutanix-exit.sh user@host
```
