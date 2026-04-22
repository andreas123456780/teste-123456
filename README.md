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

### Melhor Envio (frete)

As rotas `/api/shipping/*` integram com a API oficial da Melhor Envio
(sandbox por padrão). Defina as variáveis abaixo para ativar:

| Variável | Obrigatória | Exemplo |
|---|---|---|
| `MELHOR_ENVIO_ENV` | opcional | `sandbox` (padrão) ou `production` |
| `MELHOR_ENVIO_ACCESS_TOKEN_SANDBOX` | sim (sandbox) | JWT criado em https://sandbox.melhorenvio.com.br/painel/gerenciar/tokens |
| `MELHOR_ENVIO_ACCESS_TOKEN` | sim (produção) | JWT criado em https://melhorenvio.com.br/painel/gerenciar/tokens |
| `MELHOR_ENVIO_ORIGIN_ZIP` | sim | CEP de origem (só dígitos, ex: `01310100`) |
| `MELHOR_ENVIO_USER_AGENT` | recomendado | `NAST Streetwear (contato@seu-dominio.com)` — a ME rejeita chamadas sem UA válido |
| `ADMIN_TOKEN` | sim p/ etiqueta+rastreio | token secreto que o admin envia no header `X-Admin-Token` |

Escopos mínimos do token: `shipping-calculate`, `shipping-cart`,
`shipping-checkout`, `shipping-companies`, `shipping-services`,
`shipping-tracking`, `cart-read`, `orders-read`.

Endpoints:

- `POST /api/shipping/quote` (público, rate-limited, cache 5 min) —
  calcula opções de frete. Body:
  `{"zipCode":"04567-000","items":[{"productId":"p-tee-bw-black","quantity":1}]}`
- `POST /api/shipping/label` (admin, `X-Admin-Token`) — cria o item no
  carrinho ME, faz o checkout (debita saldo da ME), gera e imprime a etiqueta.
- `GET  /api/shipping/track/:orderId` (admin, `X-Admin-Token`) — rastreio.

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
