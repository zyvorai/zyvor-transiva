// Copyright 2026 Zyvor · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import type {ReactNode} from 'react';
import useBaseUrl from '@docusaurus/useBaseUrl';
import Layout from '@theme/Layout';
import {
  Actions,
  Band,
  Bento,
  Btn,
  CtaBand,
  Frame,
  Grad,
  Headline,
  List,
  Steps,
  SubNav,
  Terminal,
  TextLink,
} from '@site/src/components/zv';

const SUB_LINKS = [
  {to: '#why', label: 'Why'},
  {to: '#capabilities', label: 'Capabilities'},
  {to: '#flow', label: 'How it flows'},
  {to: '#nutanix', label: 'Exit Nutanix'},
  {to: '#open', label: 'Open'},
  {to: '#estate', label: 'Estate'},
];

function Hero() {
  return (
    <Band tone="black" glow center>
      <p>
        <TextLink to="/nutanix-exit">New: leaving Nutanix, end to end, with diagrams and a measured run</TextLink>
      </p>
      <Headline
        size="display"
        eyebrow="Transiva · workload mobility"
        title={
          <>
            Escape the
            <br />
            renewal <Grad>trap.</Grad>
          </>
        }
        lede="A Go control plane that discovers, inventories and orchestrates workload exports from VMware vSphere and Nutanix AHV, hands the artifacts to h2kvm for conversion, and runs fleet jobs over REST. Apache-2.0, Community Edition."
      />
      <Actions>
        <Btn to="https://github.com/zyvorai/transiva#60-second-quick-start">Get started</Btn>
        <Btn to="#capabilities" variant="ghost">
          See what it does
        </Btn>
      </Actions>
      <Terminal
        title="hyperexport · Nutanix"
        lines={[
          {kind: 'cmd', text: 'hyperexport --provider nutanix -vm web-prod-01 \\'},
          {kind: 'out', text: '  -resolve-containers \\'},
          {kind: 'out', text: '  -mounts default-container:/mnt/nutanix/default'},
          {kind: 'dim', text: '[*] negotiating the Prism v4 API version'},
          {kind: 'dim', text: '[*] reading the disk from the storage container (NFS pickup)'},
          {kind: 'info', text: '[*] converting to qcow2 and writing artifact-manifest.json'},
          {kind: 'ok', text: '[+] export complete, source VM untouched'},
        ]}
      />
      <p className="zv-stat__note" style={{marginTop: '0.9rem'}}>
        Abridged and illustrative; the real run prints live logs and a JSON result.
      </p>
    </Band>
  );
}

function Why() {
  return (
    <Band tone="soft" id="why" center>
      <Headline
        eyebrow="Why Transiva"
        title={
          <>
            Getting out should be <Grad>a job, not a project.</Grad>
          </>
        }
        lede="Every hypervisor renewal buries your VMs deeper in someone else's proprietary API. Getting out usually means a spreadsheet, a maintenance window, and a pile of one-off ovftool invocations that fail mid-transfer."
      />
      <div className="zv-two" style={{textAlign: 'left'}}>
        <div>
          <h3>What changes</h3>
          <List
            items={[
              'One CLI and job model for both vSphere and Nutanix AHV.',
              'Resumable exports through a REST daemon.',
              'Offline artifacts: the source VM stays untouched until cutover.',
            ]}
          />
        </div>
        <div>
          <h3>The stack around it</h3>
          <p className="zv-note">
            Export with Transiva, convert with h2kvm, assure with GuestKit, operate on Zeus OS.
          </p>
        </div>
      </div>
    </Band>
  );
}

function Capabilities() {
  return (
    <Band tone="light" id="capabilities" center>
      <Headline
        eyebrow="Capabilities"
        title={
          <>
            Two sources. <Grad>One workflow.</Grad>
          </>
        }
      />
      <Bento
        tiles={[
          {title: 'vSphere and Nutanix AHV', body: 'Discover, inventory and export from both through the same CLI, jobs and output layout.', to: 'https://github.com/zyvorai/transiva', variant: 'dark', more: 'Source on GitHub ›'},
          {title: 'CLI, REST and dashboard', body: 'hyperexport for interactive runs, a daemon with a REST API for fleet jobs, and a web dashboard.', to: 'https://github.com/zyvorai/transiva#60-second-quick-start', variant: 'blue', more: 'Quick start ›'},
          {title: 'Offline artifacts', body: 'A qcow2 plus artifact-manifest.json. The source VM is never modified.', to: '/nutanix-exit', more: 'See an export ›'},
          {title: 'Hand-off to h2kvm', body: 'h2kvm repairs the guest offline, converts it and lands it on KVM, KubeVirt, Kairon or Machina.', to: 'https://github.com/zyvorai/h2kvm', more: 'About h2kvm ›'},
        ]}
      />
    </Band>
  );
}

function Flow() {
  return (
    <Band tone="soft" id="flow" center>
      <Headline
        eyebrow="How it flows"
        title={
          <>
            Four steps. <Grad>No agent in the guest.</Grad>
          </>
        }
      />
      <Steps
        steps={[
          {title: '1 · Discover', body: 'Inventory VMs, disks and storage containers from vSphere or Prism.'},
          {title: '2 · Export', body: 'Pick the disk up (vSphere API, or NFS for Nutanix) and write a qcow2 and a manifest.'},
          {title: '3 · Hand off', body: 'h2kvm repairs and converts the artifact offline.'},
          {title: '4 · Land', body: 'Boot it on KVM or KubeVirt, or hand it to Kairon or Machina.'},
        ]}
      />
    </Band>
  );
}

function Nutanix() {
  const map = useBaseUrl('/img/exit-system-map.svg');
  return (
    <Band tone="light" id="nutanix" center>
      <Headline
        eyebrow="New · Exit Nutanix"
        title={
          <>
            Out of Nutanix <Grad>in minutes.</Grad>
          </>
        }
        lede="One script exports a VM through Prism v4, converts it with h2kvm and boots it on plain KVM and KubeVirt."
      />
      <Frame
        src={map}
        alt="Nutanix is exported by Transiva into a qcow2 and manifest, repaired and converted by h2kvm, and landed on KVM, KubeVirt, Kairon or Machina."
        caption="The system map. Solid targets were run; dashed ones were not."
      />
      <Actions>
        <Btn to="/nutanix-exit">See the run</Btn>
        <Btn to="https://zyvorai.github.io/zyvor-h2kvm/nutanix-exit" variant="ghost">
          The h2kvm side
        </Btn>
      </Actions>
    </Band>
  );
}

function Open() {
  return (
    <Band tone="soft" id="open" center>
      <Headline
        eyebrow="Open by default"
        title={
          <>
            Apache-2.0. <Grad>No phone-home.</Grad>
          </>
        }
        lede="No agent in the guest. Source, Docker and RPM packaging. Community Edition is free forever for labs and single-host PoCs: two source hypervisors, CLI and REST, support via GitHub Issues."
      />
      <Actions>
        <Btn to="/docs/ce-vs-enterprise">See CE vs Platform</Btn>
        <Btn to="https://github.com/zyvorai/transiva" variant="ghost">
          View on GitHub
        </Btn>
      </Actions>
    </Band>
  );
}

export default function Home(): ReactNode {
  return (
    <Layout
      title="Transiva — Enterprise Workload Mobility & Migration Control Plane"
      description="Go control plane that discovers, inventories, and orchestrates workload exports from VMware vSphere and Nutanix AHV. Apache-2.0 Community Edition.">
      <SubNav title="Transiva" links={SUB_LINKS} cta={{to: 'https://github.com/zyvorai/transiva#60-second-quick-start', label: 'Get started'}} />
      <main>
        <Hero />
        <Why />
        <Capabilities />
        <Flow />
        <Nutanix />
        <Open />
        <Band tone="light" id="estate" center>
          <Headline
            eyebrow="Moving an estate, not a lab?"
            title={
              <>
                CE proves export. <Grad>Platform moves the estate.</Grad>
              </>
            }
            lede="If you're moving 50 to 10,000+ VMs, HyperSDK Platform adds CBT incremental sync, wave planning, SSO/RBAC and a direct line to Zyvor on cutover night: 10 to 11 source providers instead of 2, and KubeVirt as a deploy target."
          />
        </Band>
        <CtaBand title="Start with one VM." lede="Prove the export in your own lab, then plan the wave.">
          <Btn to="https://zyvor.dev/poc?utm_source=github-pages&utm_medium=transiva">Start a 30-day PoC</Btn>
          <Btn to="https://zyvor.dev/contact?intent=demo&utm_source=github-pages&utm_medium=transiva" variant="ghost">
            Book a Platform demo
          </Btn>
        </CtaBand>
      </main>
    </Layout>
  );
}
