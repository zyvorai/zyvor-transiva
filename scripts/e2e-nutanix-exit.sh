#!/usr/bin/env bash
# Automated Nutanix exit test, run on a Linux host (KVM + libvirt, optional KubeVirt).
#
#   download Linux disk -> serve it as a Nutanix VM (Chimera Prism v4)
#   -> hyperexport -> h2kvmctl -> boot on libvirt -> (optional) boot on KubeVirt
#
# Usage:  ./scripts/e2e-nutanix-exit.sh [user@host]     # no host = run locally
# Env:    DISK_URL (default: newest Alpine tiny cloud image), CHIMERA_PORT (8990), WORK (~/migration-e2e)
# Needs:  chimera-nutanix.service (see chimera repo), hyperexport, h2kvmctl, virsh, qemu-img, sudo.
set -euo pipefail

if [[ $# -ge 1 ]]; then
  exec ssh -o BatchMode=yes "$1" "bash -s" < "$0"
fi

WORK=${WORK:-$HOME/migration-e2e}
PORT=${CHIMERA_PORT:-8990}
VM=${VM_NAME:-chimera-vm-01}
DOM=nutanix-e2e-$VM
BASE=https://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/cloud
ENVF=/etc/chimera/chimera-nutanix.env
mkdir -p "$WORK"

PASS=0; FAIL=0; T0=$(date +%s)
stage() { STAGE=$1; S0=$(date +%s); echo "== $1"; }
ok()    { echo "PASS $STAGE ($(( $(date +%s) - S0 ))s)"; PASS=$((PASS+1)); }
bad()   { echo "FAIL $STAGE: $*"; FAIL=$((FAIL+1)); summary; exit 1; }
summary() { echo "== $PASS passed, $FAIL failed, $(( $(date +%s) - T0 ))s total"; }
need()  { command -v "$1" >/dev/null || bad "missing tool: $1"; }

stage "preflight"
for t in curl qemu-img virsh hyperexport h2kvmctl sudo; do need $t; done
[[ -r $ENVF ]] || sudo test -r $ENVF || bad "$ENVF not found (install chimera-nutanix.service)"
ok

stage "download Nutanix-style Linux disk"
if [[ -z ${DISK_URL:-} ]]; then
  NAME=$(curl -fsS -m 30 "$BASE/" | grep -o 'alpine-[0-9.]*-x86_64-tiny-r[0-9]*\.qcow2' | sort -uV | tail -1)
  [[ -n $NAME ]] || bad "could not resolve an Alpine image name"
  DISK_URL=$BASE/$NAME
fi
IMG=$WORK/ahv-disk.qcow2
if [[ ! -s $IMG ]]; then
  curl -fsSL -m 600 -o "$IMG.part" "$DISK_URL" || bad "download failed: $DISK_URL"
  mv -f "$IMG.part" "$IMG"
fi
qemu-img check "$IMG" >/dev/null || bad "downloaded image is not a valid disk"
echo "$(qemu-img info --output=json "$IMG" | grep -m1 virtual-size)  $DISK_URL"
ok

stage "serve it as a Nutanix VM (Chimera Prism v4)"
sudo rm -rf /srv/chimera-nfs/default-container/.acropolis/vmdisk
if sudo grep -q '^CHIMERA_NUTANIX_DISK_IMAGE=' $ENVF; then
  sudo sed -i "s|^CHIMERA_NUTANIX_DISK_IMAGE=.*|CHIMERA_NUTANIX_DISK_IMAGE=$IMG|" $ENVF
else
  echo "CHIMERA_NUTANIX_DISK_IMAGE=$IMG" | sudo tee -a $ENVF >/dev/null
fi
sudo systemctl restart chimera-nutanix
for _ in $(seq 20); do curl -ks -m 2 -o /dev/null "https://127.0.0.1:$PORT/" && break; sleep 1; done
[[ -n $(sudo find /srv/chimera-nfs/default-container/.acropolis/vmdisk -name disk.img -size +1M | head -1) ]] || bad "chimera did not publish a disk"
ok

stage "export with hyperexport (Prism v4 + NFS pickup)"
PW=$(sudo sed -n 's/^CHIMERA_PASSWORD=//p' $ENVF)
OUT=$WORK/exported; rm -rf "$OUT"; mkdir -p "$OUT"
NUTANIX_HOST=127.0.0.1 NUTANIX_PORT=$PORT NUTANIX_USERNAME=admin NUTANIX_PASSWORD=$PW \
NUTANIX_VERIFY_SSL=0 NUTANIX_ENABLED=1 \
  hyperexport --provider nutanix -vm "$VM" -output "$OUT" \
    -resolve-containers \
    -mounts default-container:/srv/chimera-nfs/default-container -quiet >"$WORK/export.log" 2>&1 \
  || { tail -20 "$WORK/export.log"; bad "hyperexport failed"; }
QCOW=$(find "$OUT" -name '*.qcow2' | head -1)
MAN=$(find "$OUT" -name artifact-manifest.json | head -1)
[[ -n $QCOW && -n $MAN ]] || bad "no qcow2 / artifact-manifest.json"
qemu-img check "$QCOW" >/dev/null || bad "exported disk is corrupt"
ok

stage "convert with h2kvm"
CONV=$WORK/converted; sudo rm -rf "$CONV"; mkdir -p "$CONV"
sudo h2kvmctl --cmd local --vmdk "$QCOW" --output-dir "$CONV" --out-format qcow2 --flatten \
  --fstab-mode stabilize-all --regen-initramfs --emit-domain-xml --checksum \
  --report "$CONV/report.json" >"$WORK/h2kvm.log" 2>&1 || { tail -20 "$WORK/h2kvm.log"; bad "h2kvmctl failed"; }
CQ=$(find "$CONV" -name '*.qcow2' | head -1); [[ -n $CQ ]] || bad "h2kvm produced no qcow2"
grep -qi 'no filesystem mounted' "$WORK/h2kvm.log" && echo "WARN guest repair skipped (h2kvm: no filesystem mounted)"
ok

stage "boot on libvirt/KVM"
sudo virsh destroy "$DOM" >/dev/null 2>&1 || true; sudo virsh undefine "$DOM" --nvram >/dev/null 2>&1 || true
sudo cp "$CQ" /var/lib/libvirt/images/$DOM.qcow2
sudo virt-install --name "$DOM" --memory 512 --vcpus 1 --import --noautoconsole --osinfo detect=on,require=off \
  --disk /var/lib/libvirt/images/$DOM.qcow2 --network network=default --graphics vnc >/dev/null 2>&1 \
  || bad "virt-install failed"
IP=
for _ in $(seq 60); do
  IP=$(sudo virsh domifaddr "$DOM" --source lease 2>/dev/null | awk '/ipv4/ {sub("/.*","",$4); print $4}')
  [[ -n $IP ]] && ping -c1 -W1 "$IP" >/dev/null 2>&1 && break; IP=; sleep 2
done
[[ -n $IP ]] || bad "guest never answered ping"
echo "guest up at $IP"
ok

# KubeVirt landing is not automated here yet (needs a containerDisk build); not claimed as tested.
summary
