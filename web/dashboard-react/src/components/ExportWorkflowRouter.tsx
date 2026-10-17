// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import React, { useState } from 'react';
import VSphereExportWorkflow from './VSphereExportWorkflow';
import NutanixExportWorkflow from './NutanixExportWorkflow';

type ProviderTab = 'vsphere' | 'nutanix';

const ExportWorkflowRouter: React.FC = () => {
  const [provider, setProvider] = useState<ProviderTab>('nutanix');

  return (
    <div>
      <div style={styles.tabs}>
        <button
          type="button"
          onClick={() => setProvider('nutanix')}
          style={{ ...styles.tab, ...(provider === 'nutanix' ? styles.tabActive : {}) }}
        >
          Nutanix AHV
        </button>
        <button
          type="button"
          onClick={() => setProvider('vsphere')}
          style={{ ...styles.tab, ...(provider === 'vsphere' ? styles.tabActive : {}) }}
        >
          vSphere
        </button>
      </div>
      {provider === 'nutanix' ? <NutanixExportWorkflow /> : <VSphereExportWorkflow />}
    </div>
  );
};

const styles: { [key: string]: React.CSSProperties } = {
  tabs: {
    display: 'flex',
    gap: '8px',
    marginBottom: '20px',
  },
  tab: {
    padding: '10px 18px',
    border: '1px solid var(--line)',
    borderRadius: '12px',
    backgroundColor: 'var(--surface)',
    cursor: 'pointer',
    fontWeight: 600,
    fontSize: '13px',
  },
  tabActive: {
    backgroundColor: '#0071e3',
    color: '#fff',
    borderColor: 'var(--blue)',
  },
};

export default ExportWorkflowRouter;
