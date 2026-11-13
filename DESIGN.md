---
name: Andvari
description: Lightweight, multi-tenant WebDAV storage with simple OIDC integration
colors:
  primary: "#0ea5e9"
  primary-hover: "#0284c7"
  primary-light: "#38bdf8"
  primary-subtle: "#0ea5e91a"
  canvas: "#020617"
  surface: "#0f172a"
  surface-hover: "#1e293b"
  surface-alt: "#020617"
  border: "#1e293b"
  border-subtle: "#334155"
  text-primary: "#f8fafc"
  text-secondary: "#94a3b8"
  text-muted: "#64748b"
  warning: "#f59e0b"
  warning-bg: "#f59e0b1a"
  danger: "#f43f5e"
  danger-bg: "#f43f5e1a"
  success: "#10b981"
  success-bg: "#10b9811a"
typography:
  display:
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: "1.25"
    letterSpacing: "-0.025em"
  headline:
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
    fontSize: "1.25rem"
    fontWeight: 600
    lineHeight: "1.35"
    letterSpacing: "-0.015em"
  title:
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: "1.5"
    letterSpacing: "0em"
  body:
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: "1.5"
    letterSpacing: "0em"
  label:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: "1.4"
    letterSpacing: "0.05em"
rounded:
  sm: "6px"
  md: "8px"
  lg: "12px"
  xl: "16px"
  full: "9999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "12px"
  lg: "16px"
  xl: "24px"
  "2xl": "32px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
  button-danger:
    backgroundColor: "{colors.danger-bg}"
    textColor: "{colors.danger}"
    rounded: "{rounded.md}"
    padding: "6px 10px"
---

# Design System: Andvari

## Overview

**Creative North Star: "The Deep Subterranean Vault"**

Andvari embodies the quiet, unwavering durability of an impenetrable stone vault. Named after the mythical Norse dwarf who guarded precious subterranean treasures, the interface is uncompromisingly dark, disciplined, and utilitarian. Every screen presents dense, legible information against an abyssal slate backdrop, punctuated by crystalline glacial-sky signals. It does not entertain or cajole; it serves operators who need high-assurance, multi-tenant file storage and effortless WebDAV connectivity.

The aesthetic rejects modern consumer SaaS vanity—no pastel gradients, floating card shadows, whimsical marketing copy, or decorative cartoon spot illustrations. Instead, surfaces are flat at rest, with crisp 1px borders delineating functional areas. Density is compact yet breathable, designed for rapid visual scanning across tables, tokens, paths, and connection recipes.

**Key Characteristics:**
- **Abyssal Dark Canvas**: Deep obsidian and slate tones create a zero-glare, terminal-grade environment for technical operators.
- **Glacial Sky Signals**: The single primary accent (#0ea5e9) is reserved strictly for interactive focus, active tabs, and primary commands.
- **Tonal Layering Over Shadows**: Depth is communicated via stepped shades of slate rather than ambient blur or drop shadows.
- **Precision Monospace Utility**: Paths, hashes, tokens, and CLI snippets use dedicated monospace formatting with instant 1-click copy affordances.

## Colors

The palette is anchored in cold slate tones, using pure color sparingly for operational signaling.

### Primary
- **Glacial Sky** (#0ea5e9): The primary brand and action color. Reserved strictly for primary call-to-action buttons, active navigation tabs, brand accent badges, and focused borders.
- **Deep Sky Hover** (#0284c7): Darker shade for primary button hover and active states.
- **Glacial Ice** (#38bdf8): High-contrast variant for icon glyphs, links, and highlighted code symbols against dark backgrounds.
- **Glacial Glow Tint** (#0ea5e91a / rgba(14, 165, 233, 0.1)): Background tint for selected items, badges, and focus rings.

### Neutral
- **Abyssal Slate Canvas** (#020617): Root page background. The darkest floor of the vault.
- **Vault Panel Slate** (#0f172a): Surface color for cards, navigation headers, tables, and modal dialogs.
- **Vault Border Slate** (#1e293b): 1px structural border separating panels, headers, and container divisions.
- **Vault Divider Subtle** (#334155): Lighter border used for secondary card dividers and subtle inner wells.
- **Crystalline White** (#f8fafc): Primary headings and active interactive text.
- **Muted Steel** (#94a3b8): Secondary body text, table row secondary metadata, and icon defaults.
- **Subdued Slate** (#64748b): Disabled states, table headers, and timestamp labels.

### Functional Accents
- **Molten Amber** (#f59e0b / bg: #f59e0b1a): Administrative impersonation warning banner and high-visibility tenant alerts.
- **Signal Rose** (#f43f5e / bg: #f43f5e1a): Destructive actions, token revocation, file deletion, and connection error alerts.
- **Vault Emerald** (#10b981 / bg: #10b9811a): Successful copy confirmations, newly revealed secret tokens, and positive health badges.

### Named Rules
**The Rarity of Sky Rule.** Glacial Sky (#0ea5e9) must occupy less than 5% of any viewport. If everything glows, nothing is urgent.
**The No-Whiteout Rule.** The canvas is permanently dark (#020617). No light mode, no bright white modal bodies, and no blinding flash transitions are permitted.

## Typography

**Display Font:** System Sans Stack (`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`)
**Body Font:** System Sans Stack (`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`)
**Label/Mono Font:** UI Monospace (`ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace`)

**Character:** Utilitarian, crisp, and neutral. Clean sans-serif handles hierarchy and instructions, while tabular monospace carries technical payloads (hashes, tokens, paths, and terminal commands).

### Hierarchy
- **Display** (Bold 700, 1.5rem / 24px, line-height 1.25, tracking -0.025em): Page headers, brand titles, and modal hero headings.
- **Headline** (Semi-bold 600, 1.25rem / 20px, line-height 1.35, tracking -0.015em): Section titles, panel headers, and dialog headings.
- **Title** (Semi-bold 600, 1rem / 16px, line-height 1.5): Card headings, table column headers, and token card labels.
- **Body** (Regular 400, 0.875rem / 14px, line-height 1.5): Standard prose, instructions, explanations, and file table row data.
- **Body Small** (Regular 400, 0.75rem / 12px, line-height 1.4): Secondary metadata, breadcrumbs, and timestamp indicators.
- **Label / Mono** (Medium 500, 0.75rem / 12px, line-height 1.4, tracking 0.05em): Uppercase WebDAV badges, secret tokens, code snippets, and paths.

### Named Rules
**The Monospace Discipline Rule.** All credentials, WebDAV URLs, file sizes, and terminal commands must be rendered in monospace. Never display a token in proportional text.

## Layout

The layout uses a constrained top-down structure optimized for single-glance administrative awareness:
- **Global Navbar**: Fixed-height top bar (56px) with brand identifier, admin impersonation switcher, "How to Connect" modal trigger, and user session badge.
- **Main Container**: Centered column max-w-5xl (1024px) with horizontal padding (px-4 sm:px-6 lg:px-8).
- **Navigation Tabs**: Compact pill switcher (`bg-slate-900/80`) toggling between Tokens and Files views.
- **Spacing Rhythm**: Standard 4px baseline grid, using 8px (sm), 12px (md), 16px (lg), and 24px (xl) steps.
- **Responsive Behavior**: Breakpoints at 640px (sm) and 768px (md) collapse multi-column cards into single-column vertical stacks.

## Elevation & Depth

Andvari employs **Layered Tonal Surfaces** rather than decorative drop shadows:
- **Canvas (Level 0)**: `#020617` (Abyssal Slate)
- **Panels & Tables (Level 1)**: `#0f172a` (Vault Panel Slate) enclosed in 1px `#1e293b` borders.
- **Inner Wells & Code Blocks (Level -1)**: Inset `#020617` wells enclosed in 1px `#1e293b` borders.
- **Modals & Overlays (Level 2)**: `#0f172a` elevated with `box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.7)` and a `rgba(0, 0, 0, 0.8)` backdrop blur.

### Named Rules
**The Flat-at-Rest Rule.** Panels, cards, and tables never cast shadows when resting. Elevation is expressed strictly by border color and background shade. Shadows are reserved exclusively for floating modal dialogs.

## Shapes

- **Base Radius**: 8px (`rounded-lg`) is the canonical curvature for buttons, inputs, dropdowns, and interactive items.
- **Container Radius**: 12px (`rounded-xl`) or 16px (`rounded-2xl`) for outer cards and modal dialogs.
- **Small Elements**: 6px (`rounded-md`) for badge tags and inline copy buttons.
- **Pills**: `rounded-full` reserved solely for user avatar circles and status dots.
- **Borders**: Uniform 1px solid border (`#1e293b`) across all cards, modals, and divided rows.

## Components

### Buttons
- **Primary**: Solid Glacial Sky (`background: #0ea5e9; color: #ffffff; border-radius: 8px; padding: 8px 14px; font-weight: 600; font-size: 12px`). Hover shifts to `#0284c7`.
- **Secondary / Action**: Vault Panel (`background: #0f172a; color: #94a3b8; border: 1px solid #1e293b; border-radius: 8px; padding: 8px 14px`). Hover shifts to `background: #1e293b; color: #f8fafc`.
- **Destructive**: Rose Tint (`background: rgba(244, 63, 94, 0.1); color: #f43f5e; border: 1px solid rgba(244, 63, 94, 0.2); border-radius: 8px; padding: 6px 10px`). Hover shifts to `background: rgba(244, 63, 94, 0.2)`.

### Cards & Panels
- **Container**: `background: #0f172a; border: 1px solid #1e293b; border-radius: 12px; padding: 20px`.
- **Header**: Flex row with icon badge, title (`#f8fafc`), and trailing action slot.

### Inputs & Selects
- **Field**: `background: #020617; border: 1px solid #1e293b; border-radius: 8px; color: #ffffff; padding: 8px 12px; font-size: 14px`.
- **Focus**: `outline: none; border-color: #0ea5e9; box-shadow: 0 0 0 1px #0ea5e9`.
- **Placeholder**: Text muted (`#64748b`).

### Modals
- **Backdrop**: `background: rgba(0, 0, 0, 0.8); backdrop-filter: blur(4px)`.
- **Body**: Max width 640px, `background: #0f172a; border: 1px solid #1e293b; border-radius: 16px; padding: 24px`.
- **Dismiss**: Supports keyboard `Escape` listener and backdrop tap.

### Tables
- **Header**: `background: rgba(15, 23, 42, 0.8); border-bottom: 1px solid #1e293b; color: #64748b; font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em`.
- **Row**: `divide-y divide-slate-800/60; hover: background: rgba(30, 41, 59, 0.4)`.

## Do's and Don'ts

### Do:
- **Do** wrap every code snippet, token, and path with an immediate 1-click copy button.
- **Do** preserve 1px solid `#1e293b` borders on all container edges to maintain crisp structural boundaries.
- **Do** dismiss modals on `Escape` keypress and backdrop click.
- **Do** use `font-mono` with muted colors for file sizes, timestamps, and directory breadcrumbs.
- **Do** show explicit loading spinners on buttons executing async network operations (revoke, delete, generate).

### Don't:
- **Don't** use browser-native `alert()`, `confirm()`, or `prompt()` dialogs.
- **Don't** use pastel or multicolor gradient fills on buttons, cards, or hero banners.
- **Don't** invent light mode colors or introduce light backgrounds anywhere in the UI.
- **Don't** add decorative illustrations or marketing cartoons; maintain utilitarian technical rigor.
- **Don't** hide active tokens without warning; prompt users explicitly before closing uncopied credentials.
