// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import React from 'react';
import type { Alert } from '../types/metrics';
import { formatRelativeTime, getSeverityColor } from '../utils/formatters';

interface AlertsListProps {
  alerts: Alert[];
  onDismiss?: (alertId: string) => void;
}

export const AlertsList: React.FC<AlertsListProps> = ({ alerts, onDismiss }) => {
  if (alerts.length === 0) {
    return (
      <div
        style={{
          backgroundColor: 'var(--surface)',
          borderRadius: '8px',
          padding: '20px',
          boxShadow: '0 1px 3px rgba(0, 0, 0, 0.1)',
          textAlign: 'center',
          color: '#9ca3af',
        }}
      >
        No active alerts
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
      {alerts.map((alert) => (
        <div
          key={alert.id}
          style={{
            backgroundColor: 'var(--surface)',
            borderRadius: '8px',
            padding: '16px',
            boxShadow: '0 1px 3px rgba(0, 0, 0, 0.1)',
            borderLeft: `4px solid ${getSeverityColor(alert.severity)}`,
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'flex-start',
          }}
        >
          <div style={{ flex: 1 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
              <span
                style={{
                  display: 'inline-block',
                  padding: '2px 8px',
                  borderRadius: '12px',
                  fontSize: '12px',
                  fontWeight: '600',
                  backgroundColor: getSeverityColor(alert.severity) + '20',
                  color: getSeverityColor(alert.severity),
                }}
              >
                {alert.severity}
              </span>
              <span style={{ fontSize: '12px', color: 'var(--muted)' }}>
                {formatRelativeTime(alert.timestamp)}
              </span>
            </div>
            <div style={{ fontSize: '16px', fontWeight: '600', marginBottom: '4px' }}>
              {alert.title}
            </div>
            <div style={{ fontSize: '14px', color: 'var(--muted)' }}>
              {alert.message}
            </div>
          </div>
          {onDismiss && !alert.acknowledged && (
            <button
              onClick={() => onDismiss(alert.id)}
              style={{
                padding: '4px 12px',
                fontSize: '12px',
                borderRadius: '12px',
                border: '1px solid var(--line)',
                backgroundColor: 'var(--surface)',
                color: 'var(--muted)',
                cursor: 'pointer',
                marginLeft: '16px',
              }}
            >
              Dismiss
            </button>
          )}
        </div>
      ))}
    </div>
  );
};
