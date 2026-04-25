# Deploying NAST to Vercel

This repo ships as two Vercel projects backed by the same GitHub
repository. Frontend is a plain Vite SPA; backend is a Go HTTP server
deployed with Vercel's Go framework preset. Persistence lives in a
managed Postgres database provisioned from the Vercel dashboard.

Both services run on the Vercel free/Hobby plan.

## Prerequisites

- A Vercel account (https://vercel.com) with access to the GitHub repo.
- A Stripe account — ideally Brazilian so you can accept Pix.
- A SuperFrete account with a production or sandbox API token.
- (Optional) A Resend account for confirmation emails.

No other dependencies are required.

## 1. Create the backend project

1. https://vercel.com/new → import the GitHub repo.
2. **Root Directory**: `backend`.
3. **Framework Preset**: Vercel detects `go` automatically because of
   `backend/vercel.json`. Leave it.
4. **Environment Variables** (Production + Preview):

   | Name | Required? | Value |
   | --- | --- | --- |
   | `STRIPE_SECRET_KEY` | yes | `sk_live_...` (or `sk_test_...`) |
   | `STRIPE_WEBHOOK_SECRET` | yes for live | `whsec_...` (set after step 4) |
   | `SUPERFRETE_TOKEN` | yes | your SuperFrete API token |
   | `SUPERFRETE_ENV` | yes | `production` or `sandbox` |
   | `SUPERFRETE_ORIGIN_ZIP` | yes | origin CEP for shipping quotes |
   | `ADMIN_TOKEN` | yes | 32+ random bytes; used to log into `/admin` |
   | `ALLOWED_ORIGINS` | yes | `https://<frontend-project>.vercel.app,https://*.vercel.app` |
   | `APP_URL` | yes | frontend URL (e.g. `https://nast.vercel.app`) |
   | `RESEND_API_KEY` | optional | `re_...` if confirmation emails are enabled |
   | `EMAIL_FROM` | optional | `NAST <contato@nast.com.br>` |
   | `ORDER_TOKEN_SECRET` | optional | defaults to hash of `STRIPE_SECRET_KEY` |
   | `CRON_SECRET` | yes (Vercel cron) | 32+ random bytes; shared secret the SuperFrete retry cron uses to authenticate. `openssl rand -hex 32`. |
   | `INTERNAL_JOB_TOKEN` | optional | alias for `CRON_SECRET`. Set one *or* the other. |
   | `LABEL_PROCESSING_TIMEOUT` | optional | Go duration for a single SuperFrete pipeline call (default `25s`). Keep below the function `maxDuration` in `backend/vercel.json`. |

5. **Don't deploy yet** — provision the database first so `DATABASE_URL`
   is injected automatically on the first build.

### Why `CRON_SECRET` matters on Vercel

Vercel functions are killed the instant the HTTP response is flushed,
which means background goroutines never run. The Stripe webhook now
generates the SuperFrete label synchronously (primary path — catches
~every order). For the rare cases where SuperFrete is slow and the
function times out before saving a `tracking_code`, `backend/vercel.json`
schedules a daily cron against `/api/internal/jobs/process-labels` at
03:05 UTC. That endpoint scans `status='paid' AND tracking_code IS NULL`
and retries each pending order with exponential backoff.

Vercel Cron authenticates itself by sending `Authorization: Bearer
$CRON_SECRET`. Set that env var and the cron auto-authenticates; leave
it empty and the endpoint returns 503 so nothing runs accidentally.

**Hobby plan caveat:** Vercel Hobby restricts crons to **once per
day**. If you need faster retry (e.g. every 5 minutes), either:

- Upgrade to Pro and change `schedule` in `backend/vercel.json` to
  `*/5 * * * *`; **or**
- Keep the daily Vercel cron as a safety net and add an external
  trigger — [cron-job.org](https://cron-job.org) and GitHub Actions
  both work. The endpoint just needs:
  ```
  POST https://<backend>.vercel.app/api/internal/jobs/process-labels
  Authorization: Bearer <CRON_SECRET>
  ```
  (Recommended: every 5 minutes.) Both the webhook sync path and the
  admin manual-retry endpoint cover most cases, so the daily cron is
  usually enough in practice.

### Inspecting stuck orders

`GET /api/admin/orders/pending-labels` (auth: `X-Admin-Token`) returns
the list of paid orders that don't yet have a tracking code, along
with the latest error and attempt count. You can also re-trigger a
single order manually with `POST /api/admin/orders/{orderId}/retry-label`.

### Provisioning Postgres

In the backend project: **Storage → Add Database → Postgres** (Neon-
backed, part of Vercel Marketplace). Vercel creates a branch per
environment (prod/preview) and injects `DATABASE_URL`,
`POSTGRES_URL_NON_POOLING`, etc. as environment variables. The backend
reads `DATABASE_URL` and runs migrations on every cold start.

Free tier (at time of writing) gives you 256 MB storage and enough
compute hours to operate a small storefront. Upgrade when needed.

After the DB is attached, trigger a deploy. The first boot runs the
two migrations (`migrations/postgres/0001_init.sql`,
`migrations/postgres/0002_products.sql`) and seeds the product catalog
from the in-memory defaults in `backend/main.go`.

### Stripe webhook

Once the backend is live at `https://<backend>.vercel.app`:

1. https://dashboard.stripe.com/webhooks → **Add endpoint**.
2. URL: `https://<backend>.vercel.app/api/payments/webhook`
3. Events: `payment_intent.succeeded`, `payment_intent.payment_failed`.
4. Save → **Reveal signing secret** → copy `whsec_...`.
5. Paste it into the backend project's `STRIPE_WEBHOOK_SECRET` env var
   and redeploy.

Until this is set, the backend rejects all webhook payloads as a
safety measure.

## 2. Create the frontend project

1. https://vercel.com/new → import the same GitHub repo.
2. **Root Directory**: `frontend`.
3. Framework is auto-detected as **Vite** via `frontend/vercel.json`.
4. **Environment Variables** (Production + Preview):

   | Name | Required? | Value |
   | --- | --- | --- |
   | `VITE_API_URL` | yes | `https://<backend>.vercel.app` |
   | `VITE_STRIPE_PUBLISHABLE_KEY` | yes | `pk_live_...` or `pk_test_...` |
   | `VITE_PLAUSIBLE_DOMAIN` | optional | `nast.com.br` to enable analytics |
   | `VITE_PLAUSIBLE_SRC` | optional | custom Plausible script URL |

5. Deploy.

## 3. Custom domains

In the frontend project: **Settings → Domains → Add `nast.com.br`**.
Vercel provisions TLS automatically. Repeat for `api.nast.com.br` on
the backend project (then update `VITE_API_URL` and `ALLOWED_ORIGINS`).

## 4. Verifying the deploy

- `GET https://<backend>.vercel.app/api/health` → `{"status":"ok"}`
- `GET https://<backend>.vercel.app/api/products` → JSON array of the
  seeded catalog.
- Load the frontend; add a product to cart; get a shipping quote from
  SuperFrete; reach Stripe Elements at checkout.
- Log into `/admin` with `ADMIN_TOKEN`; edit a product; confirm the
  edit is visible on the storefront without a redeploy.

## Local development

Nothing changes — `docker run postgres:16-alpine` works, but so does
the default SQLite path (zero config). Run `DATABASE_URL=...` to
switch drivers. Migrations apply to whichever database is selected.
