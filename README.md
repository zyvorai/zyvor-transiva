<div align="center">

# Transiva

[![CI](https://github.com/zyvorai/zyvor-transiva/actions/workflows/ci.yml/badge.svg)](https://github.com/zyvorai/zyvor-transiva/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/tag/zyvorai/zyvor-transiva?label=release&sort=semver&style=flat-square&color=0071e3&labelColor=1d1d1f)](https://github.com/zyvorai/zyvor-transiva/tags)
[![Go 1.27+](https://img.shields.io/badge/go-1.27+-0071e3?style=flat-square&labelColor=1d1d1f)](https://go.dev/)
[![License: Zyvor Production Licence v2.0](https://img.shields.io/badge/license-Zyvor_Production_Licence_v2.0-0071e3?style=flat-square&labelColor=1d1d1f)](LICENSE)

[![Book a demo](https://img.shields.io/badge/Book_a_demo-0071e3?style=for-the-badge)](https://zyvor.dev/schedule?utm_source=github&utm_medium=transiva&utm_campaign=readme_hero)
[![30-day PoC](https://img.shields.io/badge/30--day_PoC-000000?style=for-the-badge)](https://zyvor.dev/poc?utm_source=github&utm_medium=transiva&utm_campaign=readme_hero)
[![Quickstart](https://img.shields.io/badge/Quickstart_in_60_seconds-0a84ff?style=for-the-badge)](#quickstart)

![Transiva - Escape the renewal trap. Export becomes a job.](docs/social/transiva-hero-dark.jpg)

### Escape the renewal trap. Export becomes a job.

**Enterprise workload mobility starts with honest offline export.** Transiva is a Go control plane that discovers, inventories, and orchestrates workload exports from VMware vSphere and Nutanix AHV, handing artifacts to [h2kvm](https://github.com/zyvorai/zyvor-h2kvm) for conversion. Fleet jobs run over REST and CLI — **Zyvor Production Licence v2.0** (30-day trial on request), no guest agent, source VM untouched until cutover.

**2 source hypervisors** · **1 workflow for both** · **0 guest agents** · **Resumable export jobs** · **CLI + REST**

[**Platform docs**](https://zyvor.dev/docs/transiva-platform?utm_source=github&utm_medium=transiva&utm_campaign=readme_hero) · [**Nutanix guide**](docs/nutanix.md) · [**OpenAPI**](openapi.yaml) · [**Paid license**](#what-the-paid-license-includes)

</div>

---

## Why Transiva

| When this happens… | Transiva gives you… |
|---|---|
| Every hypervisor renewal buries your VMs deeper in someone else's proprietary API | Discovery via the provider API and an inventory of workloads before you export anything |
| Getting out means a spreadsheet, a maintenance window and a pile of one-off `ovftool` invocations | Export as a job: interactive CLI for one-offs, `transivad` plus REST for fleets |
| An export fails mid-transfer and starts again from zero | Resumable jobs via `transivad` and REST |
| Part of the estate is on vSphere, part on Nutanix AHV | Two sources, one workflow: same CLI, same jobs, same output layout |
| Nobody will approve an agent in production guests | Offline artifacts, no guest agent, the source VM untouched until cutover |
| The export is only step one of the exit | A clean hand-off: artifacts flow into h2kvm and GuestKit, then KVM, KubeVirt and Zeus OS |

![Capabilities at a glance: Discover, Export, Automate, Hand off](docs/ux/readme-capabilities.jpg)

---

## How it fits together

![Export with Transiva, convert with h2kvm](docs/ux/readme-how-it-works.jpg)

**Export with Transiva → convert with [h2kvm](https://github.com/zyvorai/zyvor-h2kvm) → assure with [GuestKit](https://github.com/zyvorai/zyvor-guestkit) → operate on [Zeus OS](https://zyvor.dev/zeus-os?utm_source=github&utm_medium=transiva&utm_campaign=readme_suite) or [Zorvia](https://github.com/zyvorai/zyvor-zorvia).** Each is a separate tool; Zorvia's own importer is Experimental.

### Where this fits: the Zyvor suite

**transiva** (discover · export) → **[h2kvm](https://github.com/zyvorai/zyvor-h2kvm)** (convert to QCOW2) → **[GuestKit](https://github.com/zyvorai/zyvor-guestkit)** (inspect · repair) → KVM · libvirt · KubeVirt → **[Zeus OS](https://zyvor.dev/zeus-os?utm_source=github&utm_medium=transiva&utm_campaign=readme_suite)** (day-2).

Methodology: **Discover → Assess → Convert → Deploy → Operate → Optimize.** This repo covers discover + export. [Full hypervisor-exit route →](https://zyvor.dev/hypervisor-exit?utm_source=github&utm_medium=transiva&utm_campaign=readme_suite) · The diagram and stage table: [docs/suite-fit.md](docs/suite-fit.md).

---

<a id="60-second-quick-start"></a>

## Quickstart

Requires Go 1.27+ to build ([go.mod](go.mod)), and a vSphere or Nutanix AHV endpoint to export from.

```bash
git clone https://github.com/zyvorai/zyvor-transiva.git
cd zyvor-transiva
./scripts/build.sh

cp config.example.yaml config.yaml
# edit config.yaml for your vSphere endpoint
./bin/hyperexport --config config.yaml
```

Nutanix AHV, the daemon and REST API, and the binaries table are in **[docs/quick-start.md](docs/quick-start.md)**; the Nutanix walkthrough is **[docs/nutanix.md](docs/nutanix.md)**. Legacy `hyper*` binaries remain compatibility aliases for at least one major release.

<a id="binaries"></a>

| Binary | Role |
|--------|------|
| `transivaexport` (compat: `hyperexport`) | Interactive CLI exports (vSphere, Nutanix, …) |
| `transivad` (compat: `hypervisord`) | REST API daemon |
| `transivactl` (compat: `hyperctl`) | Job control — `list`, `export`, `info`, `submit` |
| `nutanix-pickup` | Standalone Nutanix discovery/pickup |

The example config's daemon address is `0.0.0.0:8080` (`daemon_addr` in [config.example.yaml](config.example.yaml)).

---

## The renewal trap — escaped with an API

Every hypervisor renewal buries your VMs deeper in someone else’s proprietary API. Getting out usually means a spreadsheet, a maintenance window, and a pile of one-off `ovftool` invocations. **Transiva turns that into a job.**

<div align="center">

| **2** source hypervisors | **1** workflow for both | **0** guest agents |
|:---:|:---:|:---:|
| Zyvor Production Licence v2.0 (30-day trial on request) | CLI + REST | Offline export — source VM untouched |

</div>

<table>
<tr>
<td valign="top" width="33%">
<b>Discover and inventory</b><br>
Discover via the provider API and inventory workloads before you export anything.<br>
<a href="docs/renewal-trap.md">The renewal trap</a>
</td>
<td valign="top" width="33%">
<b>Resumable exports</b><br>
Export fails mid-transfer? Resumable jobs via <code>transivad</code> and REST, not a restart from zero.<br>
<a href="docs/quick-start.md">Quick start</a>
</td>
<td valign="top" width="33%">
<b>Two sources, one workflow</b><br>
vSphere and Nutanix AHV behave identically: same CLI, same jobs, same output layout.<br>
<a href="docs/nutanix.md">Nutanix guide</a>
</td>
</tr>
<tr>
<td valign="top" width="33%">
<b>Offline, no guest agent</b><br>
Offline artifacts; the source VM is untouched until cutover.<br>
<a href="docs/renewal-trap.md">Why teams start here</a>
</td>
<td valign="top" width="33%">
<b>Scriptable end to end</b><br>
Interactive CLI for one-offs; daemon plus REST for fleets. Source, Docker and RPMs.<br>
<a href="openapi.yaml">OpenAPI</a>
</td>
<td valign="top" width="33%">
<b>Clean handoff</b><br>
Artifacts flow into h2kvm and GuestKit, then KVM, KubeVirt and Zeus OS.<br>
<a href="docs/suite-fit.md">Where it fits</a>
</td>
</tr>
</table>

<a id="why-teams-start-here"></a>

---

## What the paid license includes

**Transiva runs the hypervisor-exit program.** A commercial license is required for production use.

| | **[Transiva Platform](https://zyvor.dev/transiva?utm_source=github&utm_medium=transiva&utm_campaign=readme_edition)** |
|---|---|
| **Who it is for** | Platform / SRE leads · **50–10,000+ VMs** |
| **Sources** | **10–11 providers** — vSphere, Nutanix AHV, Hyper-V, AWS, Azure, GCP, OCI, OpenStack, Proxmox, KubeVirt, … |
| **Incremental** | **CBT** + smart fallback — windows that fit change freeze |
| **Security** | Vault · **SSO/OIDC/SAML** · RBAC · audit · air-gap packs |
| **Support** | **SLA** · workshops · hypervisor-exit PS |

**Bring us your worst estate.** [30-day PoC](https://zyvor.dev/poc?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer) · [Platform demo](https://zyvor.dev/contact?intent=demo&utm_source=github&utm_medium=transiva&utm_campaign=readme_footer) · [Pricing](https://zyvor.dev/pricing?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer)

---

## Development

```bash
make test
make lint
```

PRs welcome. Security reports → [SECURITY.md](SECURITY.md).

## Support the project

Transiva is source-available, maintained by **Susant Sahani** at [Zyvor AI Labs](https://zyvor.dev?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer).

| | |
|---|---|
| **Production / Platform** | [Book a demo](https://zyvor.dev/schedule?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer) · [30-day PoC](https://zyvor.dev/poc?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer) · [sales@zyvor.dev](mailto:sales@zyvor.dev) |
| **Issues** | [GitHub Issues](https://github.com/zyvorai/zyvor-transiva/issues) |
| **General** | [info@zyvor.dev](mailto:info@zyvor.dev) |

<a id="related-repos"></a>Related repositories (h2kvm, guestkit, chimera, netevd), social assets and more: [docs/support-and-related.md](docs/support-and-related.md).

## Documentation

| Topic | Doc |
|---|---|
| Every page, in one list | [docs/documentation-map.md](docs/documentation-map.md) |
| Quick start, binaries, Nutanix | [docs/quick-start.md](docs/quick-start.md) · [docs/nutanix.md](docs/nutanix.md) |
| The renewal trap and why teams start here | [docs/renewal-trap.md](docs/renewal-trap.md) |
| Where Transiva fits in the Zyvor suite | [docs/suite-fit.md](docs/suite-fit.md) |
| Verify exports offline (bundle, OVA, signatures) | [docs/artifact-verification.md](docs/artifact-verification.md) |
| REST API | [openapi.yaml](openapi.yaml) |

---

## Maturity

> **Maturity (honest):** this repository covers **two sources** and **full exports** (no CBT, no multi-provider dashboard). Ten-plus providers, waves, SSO, and cutover-night support are **Transiva Platform**.

Legacy `hyper*` binaries remain compatibility aliases for at least one major release.

---

## Part of the Zyvor stack

| Product | Role next to Transiva |
|---|---|
| **Transiva** | Discover and export VMs from vSphere and Nutanix AHV, as jobs |
| **[Scout](https://github.com/zyvorai/scout)** | Pairs with Transiva before export: read-only readiness scoring and migration-wave planning |
| **[h2kvm](https://github.com/zyvorai/zyvor-h2kvm)** | Converts Transiva's exported artifacts to QCOW2 and deploys them |
| **[GuestKit](https://github.com/zyvorai/zyvor-guestkit)** | Inspects and repairs the converted guests |
| **[Zorvia](https://github.com/zyvorai/zyvor-zorvia)** | Source-available KubeVirt platform to operate the workloads on |

→ [zyvor.dev](https://zyvor.dev)

---

## Donate

A 30-day trial is available on request from sales@zyvor.dev (a paid evaluation Order at no charge); production and any other use needs a paid Order under the Zyvor Production Licence v2.0. If it helps you, you can donate any amount you like: <https://rzp.io/rzp/utwdYh3>. A donation is voluntary and supports development and maintenance. It is not a purchase: it buys no support, SLA, features, priority or license, and it is not tax-deductible (Zyvor AI Labs Private Limited is a company, not a charity). Support contracts are separate: [sales@zyvor.dev](mailto:sales@zyvor.dev).

## License

Transiva is licensed under the [Zyvor Production Licence v2.0](LICENSE) (SPDX: `LicenseRef-Zyvor-Production-2.0`; see [NOTICE](NOTICE)). A 30-day trial is available on request from [sales@zyvor.dev](mailto:sales@zyvor.dev) (a paid evaluation Order at no charge); production and any other use needs a paid Order under the Zyvor Production Licence v2.0 ([zyvor.dev/buy](https://zyvor.dev/buy)).

**Zyvor Enterprise** adds what production teams ask for: supported releases, deployment and upgrade guidance, priority incident triage, a named technical contact and 24x7 critical intake. Plans and terms: [docs/SUBSCRIPTION-MODEL.md](docs/SUBSCRIPTION-MODEL.md) · [Pricing](https://zyvor.dev/pricing?utm_source=github&utm_medium=transiva&utm_campaign=readme_license) · [sales@zyvor.dev](mailto:sales@zyvor.dev). Migration: USD 75 / 60 / 50 per VM (INR 7,434 / 5,947 / 4,956), minimum USD 2,500 (INR 247,800). Prices: INR incl. 18% GST in India; US dollars excl. tax elsewhere. Buy / get a quote: [zyvor.dev/buy](https://zyvor.dev/buy). Orders of USD 5,000 or more and quotes are invoiced in USD through Skydo.

Security reports → [SECURITY.md](SECURITY.md).

---

<div align="center">

### Bring us your worst estate

[![Book a demo](https://img.shields.io/badge/Book_a_demo-0071e3?style=for-the-badge)](https://zyvor.dev/schedule?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer)
[![30-day PoC](https://img.shields.io/badge/Start_a_30--day_PoC-000000?style=for-the-badge)](https://zyvor.dev/poc?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer)
[![Pricing](https://img.shields.io/badge/Pricing-1d1d1f?style=for-the-badge)](https://zyvor.dev/pricing?utm_source=github&utm_medium=transiva&utm_campaign=readme_footer)
[![Contact sales](https://img.shields.io/badge/Contact_sales-0a84ff?style=for-the-badge)](mailto:sales@zyvor.dev?subject=Transiva)
[![Star on GitHub](https://img.shields.io/github/stars/zyvorai/zyvor-transiva?style=for-the-badge&logo=github&label=Star&color=2997ff)](https://github.com/zyvorai/zyvor-transiva)

</div>
