// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";

interface ChartContainerProps {
  title: string;
  type: "line" | "bar" | "pie";
  data: any[];
  dataKeys?: string[];
  colors?: string[];
  xAxisKey?: string;
  height?: number;
}

const DEFAULT_COLORS = [
  "#3b82f6",
  "#10b981",
  "#f59e0b",
  "#ef4444",
  "#8b5cf6",
  "#ec4899",
];

export const ChartContainer: React.FC<ChartContainerProps> = ({
  title,
  type,
  data,
  dataKeys = [],
  colors = DEFAULT_COLORS,
  xAxisKey = "timestamp",
  height = 300,
}) => {
  const renderChart = () => {
    switch (type) {
      case "line":
        return (
          <ResponsiveContainer width="100%" height={height}>
            <LineChart data={data}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
              <XAxis
                dataKey={xAxisKey}
                stroke="var(--muted)"
                fontSize={12}
                tickFormatter={(value) => {
                  if (xAxisKey === "timestamp") {
                    const date = new Date(value);
                    return date.toLocaleTimeString();
                  }
                  return value;
                }}
              />
              <YAxis stroke="var(--muted)" fontSize={12} />
              <Tooltip
                contentStyle={{
                  backgroundColor: "var(--surface)",
                  border: "1px solid var(--line)",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
              />
              <Legend />
              {dataKeys.map((key, index) => (
                <Line
                  key={key}
                  type="monotone"
                  dataKey={key}
                  stroke={colors[index % colors.length]}
                  strokeWidth={2}
                  dot={false}
                  isAnimationActive={false}
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        );

      case "bar":
        return (
          <ResponsiveContainer width="100%" height={height}>
            <BarChart data={data}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
              <XAxis dataKey={xAxisKey} stroke="var(--muted)" fontSize={12} />
              <YAxis stroke="var(--muted)" fontSize={12} />
              <Tooltip
                contentStyle={{
                  backgroundColor: "var(--surface)",
                  border: "1px solid var(--line)",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
              />
              <Legend />
              {dataKeys.map((key, index) => (
                <Bar
                  key={key}
                  dataKey={key}
                  fill={colors[index % colors.length]}
                  isAnimationActive={false}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        );

      case "pie":
        return (
          <ResponsiveContainer width="100%" height={height}>
            <PieChart>
              <Pie
                isAnimationActive={false}
                data={data}
                dataKey="value"
                nameKey="name"
                cx="50%"
                cy="50%"
                outerRadius={80}
                label
              >
                {data.map((_, index) => (
                  <Cell
                    key={`cell-${index}`}
                    fill={colors[index % colors.length]}
                  />
                ))}
              </Pie>
              <Tooltip
                contentStyle={{
                  backgroundColor: "var(--surface)",
                  border: "1px solid var(--line)",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
              />
              <Legend />
            </PieChart>
          </ResponsiveContainer>
        );

      default:
        return null;
    }
  };

  return (
    <div
      style={{
        backgroundColor: "var(--surface)",
        borderRadius: "8px",
        padding: "20px",
        boxShadow: "0 1px 3px rgba(0, 0, 0, 0.1)",
      }}
    >
      <h3
        style={{
          margin: "0 0 16px 0",
          fontSize: "16px",
          fontWeight: "600",
          color: "var(--ink)",
        }}
      >
        {title}
      </h3>
      {data.length === 0 ? (
        <div
          style={{
            height: height,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: "#9ca3af",
          }}
        >
          No data available
        </div>
      ) : (
        renderChart()
      )}
    </div>
  );
};
