#!/usr/bin/env python3
# Copyright 2026 Zyvor AI Labs · SPDX-License-Identifier: Apache-2.0
"""Generate the Nutanix-exit diagrams as adaptive SVGs.

    python3 docs/marketing/img/build-diagrams.py            # writes system-map.svg, sequence.svg
    python3 docs/marketing/img/build-diagrams.py --inline docs/marketing/exit-nutanix.html

Colours come from --zx-* CSS variables when a page defines them, otherwise from the
OS light/dark preference. --inline replaces <!--svg:NAME-->...<!--/svg:NAME--> in a page.
"""
import re, sys, pathlib

HERE = pathlib.Path(__file__).parent
STYLE = """<style>
svg{--c-fg:#1d1d1f;--c-mut:#6e6e73;--c-line:#c7c7cc;--c-card:#fff;--c-bg2:#f5f5f7;--c-bgp:#fff;--c-blue:#0071e3;--c-blue2:#2997ff;--c-or:#ff6a2a;--c-ok:#1d8a3a;font-family:"Helvetica Neue",Helvetica,Arial,sans-serif}
@media (prefers-color-scheme:dark){svg{--c-fg:#f5f5f7;--c-mut:#a1a1a6;--c-line:#3a3a3c;--c-card:#111114;--c-bg2:#0b0b0f;--c-bgp:#000;--c-blue:#0a84ff;--c-blue2:#64b5ff;--c-ok:#30d158}}
.fg{fill:var(--zx-fg,var(--c-fg))}.mut{fill:var(--zx-mut,var(--c-mut))}.ok{fill:var(--zx-ok,var(--c-ok))}.or{fill:var(--c-or)}
.card{fill:var(--zx-card,var(--c-card));stroke:var(--zx-line,var(--c-line));stroke-width:1.5}
.hl{fill:var(--zx-card,var(--c-card));stroke:var(--zx-blue,var(--c-blue));stroke-width:2}
.chip{fill:var(--zx-bg2,var(--c-bg2));stroke:none}.pill{fill:var(--zx-bgp,var(--c-bgp))}
.dash{stroke-dasharray:7 6}
.ln{stroke:var(--zx-mut,var(--c-mut));stroke-width:1.6;fill:none}
.lnb{stroke:var(--zx-blue,var(--c-blue));stroke-width:2.4;fill:none}
.lnd{stroke:var(--zx-mut,var(--c-mut));stroke-width:1.6;fill:none;stroke-dasharray:6 6}
.life{stroke:var(--zx-line,var(--c-line));stroke-width:1.5;stroke-dasharray:3 5}
.ah{fill:var(--zx-mut,var(--c-mut))}.ahb{fill:var(--zx-blue,var(--c-blue))}
.t{font-size:16px}.tb{font-size:20px;font-weight:700}.ts{font-size:14px}.tm{font-family:Menlo,Consolas,monospace;font-size:14px}.th{font-size:12px;font-weight:700;letter-spacing:.06em}
</style>
<defs><marker id="a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto"><path d="M0 0L10 5L0 10z" class="ah"/></marker>
<marker id="ab" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto"><path d="M0 0L10 5L0 10z" class="ahb"/></marker></defs>"""

def esc(s): return s.replace("&", "&amp;").replace("<", "&lt;")
def text(x, y, s, cls="fg t", anchor="start"): return f'<text x="{x}" y="{y}" class="{cls}" text-anchor="{anchor}">{esc(s)}</text>'
def box(x, y, w, h, cls="card", dash=False, r=16): return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" class="{cls}{" dash" if dash else ""}"/>'
def chip(x, y, w, s, mono=False): return f'<rect x="{x}" y="{y}" width="{w}" height="30" rx="9" class="chip"/>' + text(x + w/2, y + 20, s, "fg " + ("tm" if mono else "ts"), "middle")
def arrow(d, cls="ln", m="a"): return f'<path d="{d}" class="{cls}" marker-end="url(#{m})"/>'

def system_map():
    o = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 600" role="img" aria-labelledby="t1 d1"><title id="t1">How Transiva, h2kvm and the landing targets fit together</title><desc id="d1">Nutanix Prism v4 is exported by Transiva into a qcow2 and manifest, h2kvm repairs and converts it, and it lands on KVM, KubeVirt with Zorvia (both run), or Kairon and Machina (not run yet). Chimera is a test stand-in for Nutanix.</desc>', STYLE]
    # source column
    o.append(text(20, 36, "SOURCE", "mut th"))
    o.append(box(20, 60, 190, 100, "card", True)); o.append(text(115, 94, "Chimera", "fg tb", "middle")); o.append(text(115, 116, "test double (simulator)", "mut ts", "middle")); o.append(text(115, 136, "Prism v4 + NFS", "mut ts", "middle"))
    o.append(box(20, 250, 190, 150)); o.append(text(115, 290, "Nutanix", "fg tb", "middle")); o.append(text(115, 316, "Prism Central v4", "mut ts", "middle")); o.append(text(115, 338, "storage container", "mut ts", "middle")); o.append(text(115, 360, "(NFS)", "mut ts", "middle"))
    o.append(arrow("M115 160 L115 250", "lnd")); o.append(text(125, 210, "stands in for", "mut ts"))
    # transiva
    o.append(text(260, 36, "EXPORT", "mut th"))
    o.append(box(260, 215, 220, 240, "hl")); o.append(text(370, 250, "Transiva", "fg tb", "middle")); o.append(text(370, 272, "discover and export", "mut ts", "middle"))
    o += [chip(280, 292, 180, "hyperexport", True), chip(280, 332, 180, "daemon + REST API"), chip(280, 372, 180, "web dashboard"), chip(280, 412, 180, "qemu-img convert", True)]
    o.append(arrow("M210 325 L260 325", "lnb", "ab")); o.append(text(235, 312, "v4", "mut ts", "middle"))
    # artifact
    o.append(text(530, 36, "ARTIFACT", "mut th"))
    o.append(box(530, 270, 170, 130)); o.append(text(615, 308, "Artifact", "fg tb", "middle")); o.append(chip(545, 322, 140, "disk.qcow2", True)); o.append(chip(545, 358, 140, "manifest.json", True))
    o.append(arrow("M480 335 L530 335", "lnb", "ab"))
    # h2kvm
    o.append(text(750, 36, "REPAIR AND CONVERT", "mut th"))
    o.append(box(750, 215, 210, 240, "hl")); o.append(text(855, 250, "h2kvm", "fg tb", "middle")); o.append(text(855, 272, "first-boot science", "mut ts", "middle"))
    o += [chip(770, 292, 170, "GuestKit repair"), chip(770, 332, 170, "convert + checksum"), chip(770, 372, 170, "domain XML"), chip(770, 412, 170, "Windows: not run")]
    o.append(arrow("M700 335 L750 335", "lnb", "ab"))
    # destinations
    o.append(text(1030, 36, "LAND", "mut th"))
    dests = [("Linux KVM", "libvirt: boots, answers ping", True), ("KubeVirt + Zorvia", "Ready, Zorvia lists it", True), ("Kairon", "via Veyron: not run yet", False), ("Machina", "libvirt host: not run yet", False)]
    for i, (n, s, ok) in enumerate(dests):
        y = 60 + i * 118
        o.append(box(1030, y, 230, 100, "card", not ok)); o.append(text(1048, y + 38, n, "fg tb"))
        o.append(text(1048, y + 62, s, "mut ts"))
        if ok: o.append(f'<circle cx="1236" cy="{y+24}" r="11" class="ok"/><path d="M1230 {y+24} l4 4 l8 -9" fill="none" stroke="#fff" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/>')
        o.append(arrow(f"M960 335 C995 335 995 {y+50} 1030 {y+50}", "lnb" if ok else "lnd", "ab" if ok else "a"))
    # legend
    o.append('<rect x="20" y="528" width="1240" height="52" rx="12" class="chip"/>')
    o.append('<circle cx="46" cy="554" r="8" class="ok"/>' + text(62, 559, "run end to end", "mut ts"))
    o.append('<rect x="210" y="546" width="30" height="16" rx="5" class="card dash"/>' + text(250, 559, "not run yet, or a test stand-in", "mut ts"))
    o.append(text(560, 559, "Chimera stands in for a real Prism in tests.", "mut ts"))
    o.append("</svg>")
    return "\n".join(o)

def sequence():
    parts = [("Script", 90), ("hyperexport", 290), ("Prism v4", 490), ("NFS container", 690), ("h2kvm", 890), ("KVM / KubeVirt", 1090)]
    X = dict(parts)
    o = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 640" role="img" aria-labelledby="t2 d2"><title id="t2">Order of calls in a Nutanix exit</title><desc id="d2">The script runs hyperexport, which negotiates the Prism v4 version, lists the VM, reads the disk from the NFS container and converts it to qcow2 with a manifest. The script then runs h2kvm and boots the result.</desc>', STYLE]
    for n, x in parts:
        o.append(box(x - 75, 14, 150, 40, "hl" if n in ("hyperexport", "h2kvm") else "card", False, 12)); o.append(text(x, 40, n, "fg t", "middle"))
        o.append(f'<line x1="{x}" y1="54" x2="{x}" y2="624" class="life"/>')
    def msg(y, a, b, label, cls="ln", m="a", mono=False):
        x1, x2 = X[a], X[b]; s = 1 if x2 > x1 else -1
        o.append(arrow(f"M{x1+6*s} {y} L{x2-4*s} {y}", cls, m))
        w = len(label) * (8.4 if mono else 7.4) + 16; cx = (x1 + x2) / 2
        o.append(f'<rect x="{cx-w/2}" y="{y-26}" width="{w}" height="22" rx="7" class="pill"/>')
        o.append(text(cx, y - 11, label, "fg " + ("tm" if mono else "ts"), "middle"))
    def note(y, a, label, w=250, orange=False):
        x = X[a]; o.append(f'<rect x="{x-w/2}" y="{y-18}" width="{w}" height="30" rx="9" class="chip"/>'); o.append(text(x, y + 2, label, "fg ts", "middle"))
        if orange: o.append(f'<circle cx="{x-w/2+14}" cy="{y-3}" r="4" class="or"/>')
    step = [(100, "Script", "hyperexport", "run, -provider nutanix -vm <name>", "lnb", "ab", True),
            (150, "hyperexport", "Prism v4", "OPTIONS /unversioned/info (negotiate v4.3)", "ln", "a", False),
            (200, "hyperexport", "Prism v4", "GET vms, then GET vm: disks and container", "ln", "a", False),
            (250, "hyperexport", "NFS container", "read .acropolis/vmdisk/<disk>/", "ln", "a", False)]
    for y, a, b, l, c, m, mono in step: msg(y, a, b, l, c, m, mono)
    note(300, "hyperexport", "qemu-img convert, write manifest", 270)
    msg(360, "Script", "h2kvm", "run, --cmd local --vmdk <disk> --emit-domain-xml", "lnb", "ab", True)
    note(410, "h2kvm", "inspect, repair, convert, checksum", 260)
    note(450, "h2kvm", "no network config: install DHCP fallback", 300, True)
    msg(510, "h2kvm", "KVM / KubeVirt", "boot the converted disk", "lnb", "ab")
    o.append(arrow(f"M{X['KVM / KubeVirt']-6} 560 L{X['Script']+6} 560", "ln", "a")); o.append('<rect x="480" y="534" width="240" height="22" rx="7" class="pill"/>'); o.append(text(600, 549, "guest gets a lease, answers ping", "fg ts", "middle"))
    o.append(text(20, 624, "Real run (Ubuntu 24.04): 7 of 7 stages, 747 s. The Prism endpoint was a simulator.", "mut ts"))
    o.append("</svg>")
    return "\n".join(o)

OUT = {"system-map": system_map(), "sequence": sequence()}
for n, svg in OUT.items(): (HERE / f"{n}.svg").write_text(svg + "\n")
if "--inline" in sys.argv:
    p = pathlib.Path(sys.argv[sys.argv.index("--inline") + 1]); s = p.read_text()
    for n, svg in OUT.items():
        s = re.sub(rf"<!--svg:{n}-->.*?<!--/svg:{n}-->", lambda m: f"<!--svg:{n}-->\n{svg}\n<!--/svg:{n}-->", s, flags=re.S)
    p.write_text(s)
print("ok")
