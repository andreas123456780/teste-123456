# Testing the NAST Backend

## Prerequisites
- Go 1.25+ (available at `/usr/local/go/bin/go`)
- sqlite3 CLI (`sudo apt-get install -y sqlite3` if missing)

## Running Unit Tests
```bash
export PATH=$PATH:/usr/local/go/bin
cd backend && go test ./...
```
All tests use in-memory SQLite — no external DB or credentials needed.

## Running the Backend Locally
The backend uses SQLite by default (no DATABASE_URL needed):
```bash
cd backend && DATABASE_PATH=/tmp/test.db go run .
```
Missing env vars (Stripe, SuperFrete, Resend, etc.) are handled gracefully — features degrade but the server starts.

## Testing Product Catalog Seed
The `seedProducts` function upserts the in-code catalog (`var catalog` in `main.go`) into the DB on every startup. To verify:
1. Start backend with a fresh DB
2. Check products: `sqlite3 /tmp/test.db "SELECT id, image, back_image FROM products;"`
3. Modify a value in DB to simulate stale data
4. Restart backend — the seed should overwrite stale values

## Key Architecture Notes
- Product catalog is defined in `backend/main.go` (the `catalog` variable)
- Frontend fallback data mirrors the catalog in `frontend/src/data/fallback.ts`
- Product images are static files in `frontend/public/products/`
- The admin panel reads/writes to the DB, but the seed syncs code → DB on every deploy
- When adding/updating products, update BOTH `main.go` AND `fallback.ts`

## Deployment
- Frontend: Auto-deployed via Vercel on push
- Backend: Manual deploy via `flyctl deploy`
- After deploying backend, the seed runs automatically and syncs catalog changes to the DB

## Devin Secrets Needed
No secrets required for local backend testing. For full integration testing:
- Stripe keys (payments)
- SuperFrete token (shipping)
- Resend API key (emails)
- Admin credentials (admin panel)
