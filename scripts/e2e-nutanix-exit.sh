#!/usr/bin/env bash
# Automated Nutanix exit test, run on a Linux host (KVM + libvirt, optional KubeVirt).
#
#   download Linux disk -> serve it as a Nutanix VM (Chimera Prism v4)
#   -> hyperexport -> h2kvmctl -> boot on libvirt -> (optional) boot on KubeVirt
#
# Usage:  ./scripts/e2e-nutanix-exit.sh [user@host]     # no host = run locally
# Env:    H2KVMCTL (path to h2kvmctl), DISK_URL (default: Ubuntu 24.04 cloud image), MEM_MB (2048), CHIMERA_PORT (8990), WORK (~/migration-e2e)
# Needs:  chimera-nutanix.service (see chimera repo), hyperexport, h2kvmctl, virsh, qemu-img, sudo.
set -euo pipefail

if [[ $# -ge 1 ]]; then
  exec ssh -o BatchMode=yes "$1" "bash -s" < "$0"
fi

WORK=${WORK:-$HOME/migration-e2e}
PORT=${CHIMERA_PORT:-8990}
VM=${VM_NAME:-chimera-vm-01}
DOM=nutanix-e2e-$VM
DEFAULT_DISK_URL=https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
MEM_MB=${MEM_MB:-2048}
H2KVMCTL=${H2KVMCTL:-h2kvmctl}
ENVF=/etc/chimera/chimera-nutanix.env
mkdir -p "$WORK"

PASS=0; FAIL=0; T0=$(date +%s)
stage() { STAGE=$1; S0=$(date +%s); echo "== $1"; }
RES=()
ok()    { local d=$(( $(date +%s) - S0 )); echo "PASS $STAGE (${d}s)"; PASS=$((PASS+1)); RES+=("{\"stage\":\"$STAGE\",\"seconds\":$d,\"status\":\"pass\"}"); }
bad()   { echo "FAIL $STAGE: $*"; FAIL=$((FAIL+1)); summary; exit 1; }
summary() {
  local t=$(( $(date +%s) - T0 ))
  echo "== $PASS passed, $FAIL failed, ${t}s total"
  local IFS=,
  printf '{"passed":%d,"failed":%d,"total_seconds":%d,"image":"%s","stages":[%s]}\n' \
    "$PASS" "$FAIL" "$t" "${DISK_URL##*/}" "${RES[*]}" >"$WORK/results.json"
  echo "results: $WORK/results.json"
}
need()  { command -v "$1" >/dev/null || bad "missing tool: $1"; }

stage "preflight"
for t in curl qemu-img virsh hyperexport "$H2KVMCTL" sudo; do need "$t"; done
echo "h2kvmctl: $("$H2KVMCTL" --version 2>&1 | head -1)"
[[ -r $ENVF ]] || sudo test -r $ENVF || bad "$ENVF not found (install chimera-nutanix.service)"
ok

stage "download Nutanix-style Linux disk"
DISK_URL=${DISK_URL:-$DEFAULT_DISK_URL}
IMG=$WORK/disk-${DISK_URL##*/}
if [[ ! -s $IMG ]]; then
  curl -fsSL -m 1800 -o "$IMG.part" "$DISK_URL" || bad "download failed: $DISK_URL"
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
sudo "$H2KVMCTL" --cmd local --vmdk "$QCOW" --output-dir "$CONV" --out-format qcow2 --flatten \
  --fstab-mode stabilize-all --regen-initramfs --emit-domain-xml --checksum \
  --report "$CONV/report.json" >"$WORK/h2kvm.log" 2>&1 || { tail -20 "$WORK/h2kvm.log"; bad "h2kvmctl failed"; }
CQ=$(find "$CONV" -name '*.qcow2' | head -1); [[ -n $CQ ]] || bad "h2kvm produced no qcow2"
grep -qi 'no filesystem mounted' "$WORK/h2kvm.log" && echo "WARN guest repair skipped (h2kvm: no filesystem mounted)"
ok

stage "boot on libvirt/KVM"
sudo virsh destroy "$DOM" >/dev/null 2>&1 || true; sudo virsh undefine "$DOM" --nvram >/dev/null 2>&1 || true
DEST=/var/lib/libvirt/images/$DOM.qcow2
sudo cp "$CQ" "$DEST"
XML=$(find "$CONV" -name '*.xml' | head -1)
if [[ -n $XML ]]; then
  # Boot with the domain definition h2kvm emitted (firmware, NIC model, graphics), retargeted to our copy.
  sed -e "s|<name>[^<]*</name>|<name>$DOM</name>|" \
      -e "/<uuid>/d" \
      -e "s|<source file='[^']*'|<source file='$DEST'|" \
      -e "s|/var/lib/libvirt/qemu/nvram/[^<]*|/var/lib/libvirt/qemu/nvram/${DOM}_VARS.fd|" "$XML" >"$WORK/domain.xml"
  sudo virsh define "$WORK/domain.xml" >/dev/null && sudo virsh start "$DOM" >/dev/null \
    || bad "libvirt rejected the domain XML h2kvm emitted"
else
  echo "WARN h2kvm emitted no domain XML; falling back to virt-install"
  sudo virt-install --name "$DOM" --memory $MEM_MB --vcpus 1 --import --noautoconsole --osinfo detect=on,require=off \
    --disk "$DEST" --network network=default --graphics vnc >/dev/null 2>&1 || bad "virt-install failed"
fi
IP=
for _ in $(seq 120); do
  IP=$(sudo virsh domifaddr "$DOM" --source lease 2>/dev/null | awk '/ipv4/ {sub("/.*","",$4); print $4}')
  [[ -n $IP ]] && ping -c1 -W1 "$IP" >/dev/null 2>&1 && break; IP=; sleep 2
done
[[ -n $IP ]] || bad "guest never answered ping"
echo "guest up at $IP"
ok

KC=
if [[ ${LAND_KUBEVIRT:-auto} != 0 ]] && command -v podman >/dev/null; then
  for c in "kubectl" "sudo k3s kubectl"; do
    $c get crd virtualmachines.kubevirt.io >/dev/null 2>&1 && { KC=$c; break; }
  done
fi
if [[ -n $KC ]]; then
  stage "boot on KubeVirt"
  KV=nutanix-e2e-kv; CD=$WORK/cdisk; mkdir -p "$CD"; cp "$CQ" "$CD/disk.qcow2"
  printf 'FROM scratch\nADD --chown=107:107 disk.qcow2 /disk/\n' >"$CD/Dockerfile"
  podman build -q -t localhost/$KV:v1 "$CD" >/dev/null || bad "containerDisk build failed"
  podman save localhost/$KV:v1 | sudo k3s ctr images import - >/dev/null || bad "image import failed"
  $KC delete vm $KV --ignore-not-found >/dev/null
  $KC apply -f - >/dev/null <<YAML || bad "$KC apply failed"
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata: {name: $KV, labels: {migrated-from: nutanix}}
spec:
  runStrategy: Always
  template:
    metadata: {labels: {kubevirt.io/domain: $KV}}
    spec:
      domain:
        cpu: {cores: 1}
        resources: {requests: {memory: ${MEM_MB}Mi}}
        devices:
          disks: [{name: root, disk: {bus: virtio}}]
          interfaces: [{name: default, masquerade: {}}]
      networks: [{name: default, pod: {}}]
      volumes:
      - name: root
        containerDisk: {image: localhost/$KV:v1, imagePullPolicy: Never}
YAML
  $KC wait --for=condition=Ready vmi/$KV --timeout=180s >/dev/null || bad "VMI never became Ready"
  KIP=
  for _ in $(seq 90); do
    KIP=$($KC get vmi $KV -o jsonpath='{.status.interfaces[0].ipAddress}' 2>/dev/null)
    [[ -n $KIP ]] && ping -c1 -W1 "$KIP" >/dev/null 2>&1 && break; KIP=; sleep 2
  done
  [[ -n $KIP ]] || bad "KubeVirt guest never answered ping"
  echo "KubeVirt guest up"
  ok
fi
[[ -n $KC ]] || echo "SKIP boot on KubeVirt (no cluster with KubeVirt reachable)"

summary
