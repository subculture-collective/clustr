# Bundle Size Tracking Example

This document shows an example of what the bundle size report looks like on PRs.

## Example Bundle Size Report

### 📦 Bundle Size Report

| File | Current | Base | Diff |
|------|---------|------|------|
| Total JS bundle | 432.90 KB | 430.50 KB | 🔴 +2.40 KB (+0.56%) |
| Total CSS bundle | 4.74 KB | 4.70 KB | 🔴 +0.04 KB (+0.85%) |

📊 [View detailed bundle analysis](https://github.com/subculture-collective/clustr/actions/runs/...)

---

## When CI Fails

If bundle size exceeds the configured limits, the report will show:

### ⚠️ Bundle Size Limit Exceeded

- ❌ JS bundle (550.00 KB) exceeds limit of 500.00 KB

And CI will fail with the error: `Bundle size exceeds configured limits`

---

## When Bundle Size Increases Significantly

If bundle size increases by more than 5% or 50KB:

### ⚠️ Significant Bundle Size Increase

- Total JS bundle increased by 60.00 KB (13.86%), exceeding the threshold of 5% or 50KB

---

## Current Bundle Limits

- **JS Bundle**: 500 KB (gzipped)
- **CSS Bundle**: 50 KB (gzipped)

These limits can be adjusted in `frontend/.size-limit.json`.
