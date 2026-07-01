# Testing the NAST Frontend

## Prerequisites
- Node.js 18+ and npm
- Chrome browser (for visual testing)

## Running Build & Lint
```bash
cd frontend && npm install && npm run build
npx tsc -b  # typecheck
```

## Vercel Preview Testing
Every PR branch gets a Vercel preview deployment automatically. The URL pattern is:
`https://nast-git-<branch-slug>-andreasfhretjheqs-projects.vercel.app`

To find the exact URL:
1. Check PR comments for the Vercel bot comment
2. Or use `git(action="view_pr")` to find the deployment URL

## Navigating the Site
- The site has an intro "scanner" animation. Click **"ENTRAR SEM SOM"** or **"PULAR"** to skip it.
- Use the **"LOJA"** nav link to jump directly to the product section ("A COLEÇÃO INTEIRA").
- Cookie banner may appear — click "ENTENDI" to dismiss.

## Testing Product Filters
The product section has category filter buttons at the top of the grid.
- Expected categories: **Todas**, **Camisetas**, **Boxy**, **Baby Look**
- Categories are normalized (case-insensitive dedup via `normalizeCategory()`) so admin-created products with different casing won't create duplicate buttons.
- Click each filter button and verify the correct products appear.
- The "Todas" filter should show all products.

## Product Catalog (6 items as of Drop 01)
| Product | Category | Price | Image prefix |
|---------|----------|-------|--------------|
| CAMISETA BLACK & WHITE (preta) | Camisetas | R$ 89,90 | tee-cursive-black |
| CAMISA BLACK & WHITE (branca) | Camisetas | R$ 89,90 | tee-cursive-white |
| CAMISA BOXY NAST PRETA | Boxy | R$ 99,90 | boxy-black |
| CAMISETA BOXY NAST BRANCA | Boxy | R$ 99,90 | boxy-white |
| BABY LOOK NAST TEE (preta) | Baby Look | R$ 79,90 | bb-look-black |
| BABY LOOK NAST TEE (branca) | Baby Look | R$ 79,90 | bb-look-white |

Each product has a front image (`<prefix>.jpeg`) and back image (`<prefix>-back.jpeg`) in `frontend/public/products/`.

## Key Frontend Files
- `frontend/src/components/Products.tsx` — Product grid with category filters
- `frontend/src/data/fallback.ts` — Fallback product data used when API is unavailable
- `frontend/src/components/ProductCard.tsx` — Individual product card
- `frontend/src/components/ProductModal.tsx` — Product detail modal

## Deployment
- Frontend auto-deploys to Vercel when PRs are merged to `bootstrap` branch
- No manual deploy step needed for frontend changes
- Backend changes still require `flyctl deploy`

## Devin Secrets Needed
No secrets needed for frontend testing — Vercel previews are public.
