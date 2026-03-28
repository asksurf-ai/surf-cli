# Surf ECharts Style Reference

All charts MUST follow these rules. ECharts (`echarts` + `echarts-for-react`) is the only chart library.

## Theme-Aware Variables

Define once per chart component:

```javascript
const isDark = resolvedTheme === "dark";
const tooltipBg = isDark ? "#171717" : "#fff";
const tooltipBorder = isDark ? "rgba(255,255,255,0.12)" : "rgba(42,42,42,0.08)";
const fgBase = isDark ? "#e7e7e7" : "#212121";
const fgSubtle = isDark ? "#aaaaaa" : "#7a7a7a";
const axisLabelColor = fgSubtle;
const axisLineColor = isDark ? "#aaaaaa" : "#7a7a7a";
const splitLineColor = isDark ? "rgba(255,255,255,0.12)" : "#f0f0f0";
```

## Chart Color Palette

```javascript
const CHART_COLORS = [
  "#fd4b96", // rose-pop
  "#6366f1", // indigo-breeze
  "#10b981", // emerald-mint
  "#f59e0b", // golden-amber
  "#1d4ed8", // royal-blue
  "#ef4444", // crimson-spark
  "#06b6d4", // aqua-glow
  "#facc15", // sunbeam-yellow
];
```

## Flat Visual Baseline

```javascript
grid: { left: 48, right: 16, top: 16, bottom: 32 },
xAxis: {
  type: "category",
  axisLine: { show: true, lineStyle: { color: axisLineColor, width: 1 } },
  axisLabel: { color: axisLabelColor },
  axisTick: { show: false },
},
yAxis: {
  type: "value",
  axisLine: { show: false },
  axisTick: { show: false },
  axisLabel: { color: axisLabelColor },
  splitLine: { lineStyle: { type: "dashed", color: splitLineColor } },
},
```

## Series Rules

- **Line**: `symbol: "none"`, `smooth: false`, `lineStyle: { width: 1.5 }` (max 1.5)
- **Bar**: `borderRadius: [2, 2, 0, 0]` max, no shadows
- **Pie/Donut**: subtle emphasis scale, no emphasis shadows
- **Treemap**: `itemStyle: { borderWidth: 0, gapWidth: 1 }`

Avoid: heavy shadows, non-transparent chart background, decorative 3D.

## Surf Tooltip

```javascript
tooltip: {
  trigger: "axis", // use "item" for pie/scatter/gauge/treemap
  backgroundColor: tooltipBg,
  borderColor: tooltipBorder,
  borderWidth: 1,
  padding: [8, 12],
  textStyle: { color: fgBase, fontSize: 12 },
  transitionDuration: 0.15,
  extraCssText: "border-radius:8px;box-shadow:0 4px 12px rgba(0,0,0," + (isDark ? "0.4" : "0.08") + ");",
}
```

## Tooltip Formatter (REQUIRED)

MUST use custom formatter for ALL chart types. Never rely on ECharts default tooltip (it uses circle dots instead of Surf dash indicators). Color indicator is a short horizontal dash (12x2.5px).

### Axis Formatter (line/bar/area — `trigger: "axis"`)

```javascript
formatter: (params) => {
  const header = `<div style="font-weight:600;font-size:12px;color:${fgBase};margin-bottom:4px">${params[0].axisValueLabel}</div>`;
  const rows = params.map(p =>
    `<div style="display:flex;justify-content:space-between;align-items:center;gap:16px">` +
      `<div style="display:flex;align-items:center;gap:6px">` +
        `<span style="display:inline-block;width:12px;height:2.5px;border-radius:1px;background:${p.color}"></span>` +
        `<span style="color:${fgSubtle};font-size:12px">${p.seriesName}</span>` +
      `</div>` +
      `<span style="font-weight:600;font-size:12px;color:${fgBase}">${p.value}</span>` +
    `</div>`
  ).join("");
  return header + rows;
},
```

### Item Formatter (pie/scatter/gauge — `trigger: "item"`)

```javascript
formatter: (params) => {
  const header = `<div style="font-weight:600;font-size:12px;color:${fgBase};margin-bottom:4px">${params.seriesName}</div>`;
  const val = params.percent != null ? `${params.value} (${params.percent}%)` : params.value;
  return header +
    `<div style="display:flex;justify-content:space-between;align-items:center;gap:16px">` +
      `<div style="display:flex;align-items:center;gap:6px">` +
        `<span style="display:inline-block;width:12px;height:2.5px;border-radius:1px;background:${params.color}"></span>` +
        `<span style="color:${fgSubtle};font-size:12px">${params.name}</span>` +
      `</div>` +
      `<span style="font-weight:600;font-size:12px;color:${fgBase}">${val}</span>` +
    `</div>`;
},
```

## Legend

```javascript
legend: {
  type: "plain",  // never scroll mode
  bottom: 0,
  icon: "roundRect",
  itemWidth: 12,
  itemHeight: 3,
  textStyle: { color: fgSubtle, fontSize: 11 },
}
```

With bottom legend: set `grid.bottom: 64` to avoid overlap with x-axis labels.

## Time Series

Do NOT use `dataZoom` slider. Use **timeframe tabs** above the chart:

```tsx
const TIMEFRAMES = ["7D", "30D", "90D", "1Y", "All"] as const;
const [range, setRange] = useState<string>("All");

const filteredData = useMemo(() => {
  if (range === "All") return allData;
  const MS = { D: 864e5, Y: 365.25 * 864e5 };
  const m = range.match(/^(\d+)(D|Y)$/);
  if (!m) return allData;
  const cutoff = Date.now() - Number(m[1]) * MS[m[2] as keyof typeof MS];
  return allData.filter(d => new Date(d.date ?? d.timestamp).getTime() >= cutoff);
}, [range, allData]);
```

Tab styling: `px-2 py-0.5 text-[10px] font-semibold rounded`, active `bg-brand-100 text-white`, inactive `text-fg-muted`.

## Heatmap

Single-hue gradient (never rainbow/spectral):

| Data semantics | Base color |
|---------------|-----------|
| Default/brand | `#FD4B96` (rose-pop) |
| Positive/growth | `#10B981` (emerald) |
| Neutral/volume | `#6366F1` (indigo) |

Pattern: `[rgba(BASE, 0.04), rgba(BASE, 0.15), rgba(BASE, 0.4), BASE_HEX]`. Cell: `borderWidth: 2`, `borderColor` matching page bg. `visualMap.show: false`.
