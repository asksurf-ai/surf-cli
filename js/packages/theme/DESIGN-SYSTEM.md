# Surf Design System — Token Reference

Quick reference for Surf semantic tokens. Full definitions in `index.css`.

## Background Tokens

| Tailwind Class | Purpose |
|---------------|---------|
| `bg-bg-chat` | Page background (dark: `#101010`, light: `#fcfcfc`) |
| `bg-bg-chat-nav` | Navigation/sidebar background |
| `bg-bg-subtle` | Hover states, skeleton loaders, subtle fills |
| `bg-bg-base` | Card/panel background (semi-transparent) |
| `bg-bg-base-opaque` | Card background when opacity isn't desired |
| `bg-bg-menu` | Dropdown/popover background |

## Foreground Tokens

| Tailwind Class | Purpose |
|---------------|---------|
| `text-fg-base` | Primary text (headings, body) |
| `text-fg-subtle` | Secondary text (labels, descriptions) |
| `text-fg-muted` | Tertiary text (placeholders, timestamps) |
| `text-fg-disabled` | Disabled state text |

## Border Tokens

| Tailwind Class | Purpose |
|---------------|---------|
| `border-border-base` | Subtle dividers (barely visible) |
| `border-border-strong` | Card borders, section dividers |
| `border-border-contrast` | Emphasized borders (active states) |
| `border-border-focus` | Focus rings |
| `border-input` | Form input borders (use this, not `border-primary`) |

## Brand Colors

| Tailwind Class | Value |
|---------------|-------|
| `bg-brand-100` / `text-brand-100` | `#ff2882` (primary pink) |
| `bg-brand-60` | 60% opacity brand |
| `bg-brand-30` | 30% opacity brand |
| `bg-brand-10` | 10% opacity brand (subtle highlights) |

## Tag Colors

6 tag color pairs for categorical labels:

```
bg-tag-blue-10 text-tag-blue-100
bg-tag-yellow-10 text-tag-yellow-100
bg-tag-purple-10 text-tag-purple-100
bg-tag-cyan-10 text-tag-cyan-100
bg-tag-pink-10 text-tag-pink-100
bg-tag-orange-10 text-tag-orange-100
```

Usage: `<span className="px-2 py-0.5 rounded-full text-xs bg-tag-blue-10 text-tag-blue-100">DeFi</span>`

## Visualizer (Chart) Colors

8 chart colors for data series:

| Token | Hex | Name |
|-------|-----|------|
| `--visualizer-rose-pop` | `#fd4b96` | Rose Pop |
| `--visualizer-indigo-breeze` | `#6366f1` | Indigo Breeze |
| `--visualizer-emerald-mint` | `#10b981` | Emerald Mint |
| `--visualizer-golden-amber` | `#f59e0b` | Golden Amber |
| `--visualizer-royal-blue` | `#1d4ed8` | Royal Blue |
| `--visualizer-crimson-spark` | `#ef4444` | Crimson Spark |
| `--visualizer-aqua-glow` | `#06b6d4` | Aqua Glow |
| `--visualizer-sunbeam-yellow` | `#facc15` | Sunbeam Yellow |

## Typography

| Token | Font |
|-------|------|
| `font-family-header` | Lato |
| `font-family-body` | Lato |
| `font-family-code` | Roboto Mono |

**Never use**: Inter, Space Grotesk, Poppins, Outfit, or other sans-serif fonts.

## Design Principles

- **Borders over shadows** — use `border-border-strong` not `shadow-md`
- **Flat cards** — `border border-border-strong rounded bg-bg-base-opaque`
- **Use `bg-bg-chat`** for page background, not `bg-background`
- **Skeleton loaders** use `bg-bg-subtle` with `animate-pulse`
- **Brand color sparingly** — only for primary CTA buttons and active states
