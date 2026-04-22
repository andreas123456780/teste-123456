# NAST — streetwear autoral

Site da marca **NAST**. Peças limitadas, estética minimalista e acabamento
premium. Este repositório contém o frontend animado + o backend em Go.

- **Backend:** Go 1.23, apenas stdlib, hardening embutido (security headers,
  CORS por allowlist, rate limit por IP, validação e limites de body).
- **Frontend:** React 19 + Vite + TypeScript, Tailwind v4, Framer Motion,
  fotos reais dos produtos e tabela de medidas lateral.

## Estrutura

```
backend/   API em Go (stdlib)
frontend/  SPA React animada
```

## Rodando localmente

### Backend

```bash
cd backend
go run .
# http://localhost:8080
```

Variáveis úteis:

- `PORT` (padrão `8080`)
- `ALLOWED_ORIGINS` (padrão `http://localhost:5173,http://127.0.0.1:5173`)
- `TRUSTED_PROXIES` — lista de CIDRs (ou IPs) de reverse proxies em que o
  servidor pode confiar para ler `X-Forwarded-For`. Quando vazio (padrão),
  o header é ignorado e o rate limit é aplicado ao peer direto. Exemplo atrás
  de Cloudflare + um balanceador interno:
  `TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12,173.245.48.0/20`

### SuperFrete (frete)

As rotas `/api/shipping/*` integram com a [API da
SuperFrete](https://superfrete.readme.io) (sandbox por padrão). Defina as
variáveis abaixo para ativar:

| Variável | Obrigatória | Exemplo |
|---|---|---|
| `SUPERFRETE_ENV` | opcional | `sandbox` (padrão) ou `production` |
| `SUPERFRETE_TOKEN` | sim | Bearer gerado em `https://sandbox.superfrete.com/#/integrations` (ou `web.superfrete.com` em produção) |
| `SUPERFRETE_ORIGIN_ZIP` | sim | CEP de origem (só dígitos, ex: `08503000`) |
| `SUPERFRETE_USER_AGENT` | recomendado | `NAST Streetwear (contato@seu-dominio.com)` — SuperFrete exige UA com contato |
| `ADMIN_TOKEN` | sim p/ etiqueta+rastreio | token secreto que o admin envia no header `X-Admin-Token` |

Endpoints:

- `POST /api/shipping/quote` (público, rate-limited, cache 5 min) —
  calcula opções de frete. Body:
  `{"zipCode":"04567-000","items":[{"productId":"p-tee-bw-black","quantity":1}]}`
- `POST /api/shipping/label` (admin, `X-Admin-Token`) — adiciona a
  encomenda ao carrinho SuperFrete, faz o checkout (debita saldo), gera e
  imprime a etiqueta.
- `GET  /api/shipping/track/:orderId` (admin, `X-Admin-Token`) — usa
  `GET /api/v0/order/info/:id` pra devolver status + código de rastreio.

### Stripe (pagamentos — cartão + Pix)

O fluxo `/api/checkout` cria um pedido local com status
`pending_payment` e devolve `orderId`. O browser troca esse id por um
`clientSecret` em `/api/payments/intent` e confirma o pagamento com os
Stripe Elements. O webhook `/api/payments/webhook` marca o pedido como
`paid` (ou `failed`) a partir dos eventos `payment_intent.*`.

| Variável | Obrigatória | Onde |
|---|---|---|
| `STRIPE_SECRET_KEY` | sim | https://dashboard.stripe.com/test/apikeys (`sk_test_…`) |
| `STRIPE_WEBHOOK_SECRET` | sim p/ webhook | https://dashboard.stripe.com/test/webhooks (`whsec_…`) |
| `VITE_STRIPE_PUBLISHABLE_KEY` | sim no build do frontend | `pk_test_…` |

> Pix via Stripe requer conta Stripe BR com o método habilitado. Cartão
> funciona em qualquer conta. O desconto Pix (-5%) permanece aplicado no
> backend antes da criação do PaymentIntent — o cliente paga o valor já
> descontado.

Endpoints:

- `POST /api/checkout` (público) — valida o carrinho, aplica o desconto
  Pix quando aplicável e guarda o pedido em memória. Retorna
  `{orderId, totalCents, shippingCents, amountCents, status,
  paymentMethod}`.
- `POST /api/payments/intent` (público) — cria o PaymentIntent Stripe
  para um pedido existente. Retorna `{clientSecret, amountCents, ...}`.
- `POST /api/payments/webhook` (Stripe → backend) — valida assinatura
  (HMAC SHA-256 sobre `t=<ts>.<body>`) e atualiza o status do pedido.

Dimensões/peso de cada SKU ficam em `backend/shipping.go` (`packagingPresets`).
Pese e ajuste antes de ir pra produção — quando um SKU não está na tabela,
caímos num envelope conservador de 250g / 30×25×3 cm.

Testes:

```bash
cd backend
go test ./...
```

### Frontend

```bash
cd frontend
npm install
npm run dev
# http://localhost:5173
```

Build e lint:

```bash
npm run lint
npm run build
```

A SPA também funciona sem o backend — há um catálogo de fallback equivalente
ao seed em memória do Go.

## Customização rápida

- **Número do WhatsApp flutuante:** `frontend/src/App.tsx` — constante
  `WHATSAPP_NUMBER` no formato E.164 sem o `+` (ex: `5511999999999`).
- **Cor de destaque:** `frontend/src/index.css` — variável
  `--color-accent` (padrão `#39ff14`).
- **Produtos:** `backend/main.go` (seed em memória) + fallback em
  `frontend/src/data/fallback.ts`. Mantenha as duas listas em sincronia.
- **Fotos dos produtos:** `frontend/public/products/` (referenciadas por
  nome de arquivo no campo `image` de cada produto).
- **Tabela de medidas:** `frontend/src/components/SizeChart.tsx` —
  três tabelas (camiseta regular, boxy, baby look) exibidas num painel
  lateral fixo ao lado do grid e num modal acessível pelo link
  "tabela de medidas".

## Seções

- Hero com tipografia gigante animada e parallax suave
- Grid de produtos (4 peças reais) + tabela de medidas lateral
- CTA primária: "entrar em contato" via WhatsApp; secundária: sacola
- Manifesto (scroll reveal palavra a palavra)
- Newsletter, footer com sociais, WhatsApp flutuante

## Segurança

- `Content-Security-Policy`, `Strict-Transport-Security`, `X-Frame-Options:
  DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`,
  `Permissions-Policy`.
- CORS restrito por allowlist vinda de `ALLOWED_ORIGINS`.
- Rate limit em memória (token bucket por IP, GC automático). A IP de origem
  só considera `X-Forwarded-For` quando o peer pertence a `TRUSTED_PROXIES`,
  evitando spoof trivial do header.
- `http.MaxBytesReader` para todos os corpos + timeouts de leitura/escrita.
- `json.Decoder` com `DisallowUnknownFields` no checkout.
- Validação estrita (email, tamanhos, quantidades, caracteres de controle).
- Comparação em tempo constante para lookup por ID (`crypto/subtle`).
- Zero dependências externas no backend — supply-chain mínima.
