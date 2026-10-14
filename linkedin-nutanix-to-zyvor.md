# How to get your VMs out of Nutanix and onto Zyvor

*LinkedIn article draft. Teaser post at the bottom.*

---

Most Nutanix exit plans start the same way: someone opens a spreadsheet, counts the VMs, multiplies by "a few days each" and lands on an 18-month project.

It doesn't have to be. The hard part of leaving a hypervisor isn't copying disks. It's the VM that boots fine on the old platform and drops to a dracut shell the first time it sees KVM.

Here is the path we use, end to end, with the actual commands.

## The shape of it: four steps

**Export → Repair → Convert → Land.**

Nothing is powered on until the disk is fixed. That one rule removes most of the war-room moments.

## Step 1: Get the disk out of AHV (Transiva)

Transiva is open source: https://github.com/zyvorai/zyvor-transiva

Nutanix export is an offline pickup, not a live pull of disk bytes. You mount the storage containers over NFS on a migration host, and Transiva finds each VM's disks through the Prism Central API and converts them with `qemu-img`.

```
transivactl list --provider nutanix --server prism.example.com --user admin
transivactl export --provider nutanix --vm web-prod-01 \
  --output /var/lib/transiva/nutanix \
  --mounts 'default-container-12345:/mnt/nutanix/default' \
  --resolve-containers
```

You get a qcow2 or raw image plus an `artifact-manifest.json`. No agent inside the guest, and no change to the running VM.

## Step 2 and 3: Fix the guest offline, then convert (h2kvm)

This is where migrations usually go wrong: bootloader and initramfs without virtio drivers, Windows still pointing at the old hypervisor's storage controller, stale network config.

h2kvm takes the exported disk, repairs the guest while it is still a file, injects first-boot, network and user config, and converts it to qcow2. GuestKit does the repair work and covers 35+ OS versions, including the Windows VirtIO and registry path.

The Nutanix export goes in as a local file:

```
h2kvmctl --cmd local --vmdk web-prod-01.qcow2 --deploy-kairon ...
```

## Step 4: Land it

You choose where it runs. Three real options:

- **Kairon, driven through Veyron (the default).** h2kvm asks Veyron to create a Kairon Machine pointing at the converted disk, pinned by sha256. The node boots it straight on KVM. No `virt-launcher` pod per VM, no KubeVirt, no CDI, no PVC upload.
- **Zorvia on KubeVirt, if you already run it.** Zorvia is the Apache-2.0 control plane around KubeVirt: CLI, TUI and web console, 43 OS templates, day-2 hotplug and live migration, RBAC and an exportable audit trail. h2kvm still supports KubeVirt as a legacy target (`--deploy-k8s`).
- **Machina, for plain libvirt hosts.** h2kvm emits the domain XML and the private cloud takes over from there.

One target per run, and the CLI rejects combinations.

## After cutover: day 2

Veyron is the console, API and CLI for the VMs: templates, snapshots, SSO, RBAC and a SOC view, in one Rust binary that keeps no database of its own. Kubernetes stays the source of truth.

If you are landing on KubeVirt, Zorvia gives you the same day-2 layer: drift and plan checks before changes, snapshots, backups, and a console that reaches the guest without handing out cluster credentials.

## What we measured, and what we didn't

On the same node, same Ubuntu 24.04 image and same readiness probe (SSH banner), run back to back against KubeVirt v1.9.0 on 2026-10-04:

- 14x lighter idle control plane (63 MiB vs 905 MiB)
- 7.4x faster to SSH with 5 VMs at once (24.8 s vs 184.7 s, p50)

To be straight about it: per-VM memory is the same, because both run the same QEMU. The 10-VM run was on a shared lab host, so read it as "the pod-per-VM path breaks first under pressure", not as KubeVirt's density ceiling. The win is the control plane and the start path.

Two more honest notes. The Nutanix pickup lives in Transiva, a separate repo from h2kvm (https://github.com/zyvorai/zyvor-transiva). And production use of h2kvm needs a paid licence (pricing is public); evaluation and labs are free.

## Try it on one VM

Pick the least important VM you own and run the four steps above. If it boots first time, you have your answer for the other 200.

30-day proof of concept: https://zyvor.dev/poc?utm_source=linkedin&utm_medium=article&utm_campaign=nutanix_exit
Book a walkthrough: https://zyvor.dev/schedule?utm_source=linkedin&utm_medium=article&utm_campaign=nutanix_exit

---

## Teaser post (for the feed)

Leaving Nutanix isn't an 18-month project. It's four steps: export the disk, fix the guest offline, convert, land it.

We wrote up the exact path from AHV to Kairon/Veyron (or KubeVirt with Zorvia), with the commands and the benchmark caveats.

Article below. Start with one VM.
