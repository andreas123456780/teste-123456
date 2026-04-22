# NAST — Guia de deploy

Deploy profissional do site NAST (backend Go + frontend React num
container único). O alvo recomendado é **Fly.io** (`gru` region, Brasil),
mas o container é 100% genérico — roda em qualquer orquestrador que
aceite Docker (Render, Railway, ECS, Cloud Run, Kubernetes…).

---

## 1. Pré-requisitos

- Conta **Stripe** com Pix habilitado
  (https://dashboard.stripe.com/settings/payments/pix).
- Conta **SuperFrete** (https://web.superfrete.com) com saldo para
  comprar etiquetas.
- Conta **Resend** para e-mails transacionais
  (https://resend.com) — opcional: o fluxo não quebra se estiver
  ausente.
- Domínio (ex: `nast.com.br`). Recomendo registrar no
  https://registro.br.
- **Fly CLI** instalado:
  `curl -L https://fly.io/install.sh | sh`.

## 2. Variáveis de ambiente

Todas são setadas via `flyctl secrets set`. Nenhuma é commitada.

| Variável | Obrigatória | O que é |
|---|---|---|
| `STRIPE_SECRET_KEY` | sim | `sk_live_…` ou `sk_test_…` |
| `STRIPE_WEBHOOK_SECRET` | sim | `whsec_…` gerado ao criar o endpoint |
| `SUPERFRETE_TOKEN` | sim | Bearer da SuperFrete |
| `SUPERFRETE_ORIGIN_ZIP` | sim | CEP de origem, só dígitos |
| `SUPERFRETE_ENV` | sim | `production` ou `sandbox` |
| `SUPERFRETE_USER_AGENT` | recomendado | `NAST (contato@nast.com.br)` |
| `ADMIN_TOKEN` | sim | Segredo do header `X-Admin-Token` |
| `ALLOWED_ORIGINS` | sim | `https://nast.com.br` (em produção) |
| `APP_URL` | sim | URL pública do site (usada nos e-mails) |
| `ORDER_TOKEN_SECRET` | recomendado | `openssl rand -hex 32` |
| `RESEND_API_KEY` | opcional | Sem ela os e-mails são pulados |
| `EMAIL_FROM` | opcional | `NAST <contato@nast.com.br>` |
| `TRUSTED_PROXIES` | opcional | CIDRs dos proxies da Fly/CF |
| `VITE_STRIPE_PUBLISHABLE_KEY` | sim | **Build-arg**, não runtime |

> O `VITE_STRIPE_PUBLISHABLE_KEY` é lido pelo Vite na hora do
> `npm run build`. Passe no `flyctl deploy --build-arg
> VITE_STRIPE_PUBLISHABLE_KEY=pk_live_...` ou ajuste no `fly.toml`.

## 3. Primeiro deploy (Fly.io)

```bash
flyctl auth login

# 1. cria o app (uma única vez)
flyctl apps create nast-backend

# 2. cria o volume que vai manter o SQLite entre deploys
flyctl volumes create nast_data --size 1 --region gru

# 3. seta os secrets
flyctl secrets set \
  STRIPE_SECRET_KEY=sk_live_... \
  STRIPE_WEBHOOK_SECRET=whsec_... \
  SUPERFRETE_TOKEN=... \
  SUPERFRETE_ORIGIN_ZIP=08503000 \
  SUPERFRETE_ENV=production \
  SUPERFRETE_USER_AGENT='NAST (contato@nast.com.br)' \
  ADMIN_TOKEN=$(openssl rand -hex 24) \
  ORDER_TOKEN_SECRET=$(openssl rand -hex 32) \
  ALLOWED_ORIGINS=https://nast.com.br \
  APP_URL=https://nast.com.br \
  RESEND_API_KEY=re_... \
  EMAIL_FROM='NAST <contato@nast.com.br>'

# 4. deploy
flyctl deploy \
  --build-arg VITE_STRIPE_PUBLISHABLE_KEY=pk_live_...
```

Ao final, `flyctl status` deve mostrar 1 machine `started` e
`https://nast-backend.fly.dev/api/health` deve retornar `{"ok":true,…}`.

## 4. Domínio próprio

```bash
# registra o domínio no painel do Fly (certificate emitido automaticamente)
flyctl certs create nast.com.br
flyctl certs create www.nast.com.br
```

O Fly te dará os registros DNS para apontar. No registro.br / Cloudflare /
onde estiver o DNS:

- `A` em `nast.com.br` → IP mostrado pelo `flyctl certs show nast.com.br`
- `AAAA` em `nast.com.br` → IPv6 mostrado
- `CNAME` em `www` → `nast-backend.fly.dev`

Quando a checagem passar, atualize os secrets:

```bash
flyctl secrets set \
  APP_URL=https://nast.com.br \
  ALLOWED_ORIGINS=https://nast.com.br,https://www.nast.com.br
```

## 5. Webhook Stripe

Somente após o domínio estar no ar:

1. Vá em https://dashboard.stripe.com/webhooks → **Add endpoint**.
2. **URL:** `https://nast.com.br/api/payments/webhook`
3. **Eventos:** `payment_intent.succeeded`, `payment_intent.payment_failed`
4. Copie o `whsec_…` e atualize:
   ```bash
   flyctl secrets set STRIPE_WEBHOOK_SECRET=whsec_...
   ```

## 6. Resend (e-mails transacionais)

1. Crie uma API key em https://resend.com/api-keys.
2. Adicione seu domínio em https://resend.com/domains e siga os
   registros DNS (SPF + DKIM + DMARC).
3. Setup final:
   ```bash
   flyctl secrets set \
     RESEND_API_KEY=re_... \
     EMAIL_FROM='NAST <contato@nast.com.br>'
   ```

Sem isso, pedidos continuam funcionando — só o e-mail de confirmação
não é enviado.

## 7. Observabilidade

- **Logs:** `flyctl logs`
- **Métricas:** `https://fly.io/apps/nast-backend/monitoring`
- **Saúde:** `curl https://nast.com.br/api/health`
- O backend emite logs HTTP estruturados (timestamps + IP real
  resolvido via `TRUSTED_PROXIES`) e não loga dados pessoais.

## 8. Backup do banco

O SQLite fica em `/data/nast.db` dentro do volume `nast_data`. Para
backups rotativos:

```bash
flyctl ssh console -C "sqlite3 /data/nast.db .dump" > backups/$(date +%F).sql
```

Recomendado: `litestream` replicando para S3 (não incluído por
simplicidade).

## 9. Rotina de atualização

```bash
git pull
flyctl deploy --build-arg VITE_STRIPE_PUBLISHABLE_KEY=pk_live_...
```

Migrations SQL novas em `backend/migrations/` aplicam automaticamente no
boot. São idempotentes e ordenadas lexicograficamente.
