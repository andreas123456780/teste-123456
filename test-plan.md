# NEST — Test Plan (PR #1)

## What changed (user-visible)
Primeira implementação do site da **NEST** — streetwear autoral com estética preto/branco + acento verde neon `#39FF14`. Tagline do hero: **"FEITO PRA QUEM SABE."**. Catálogo de 4 peças com hover frente/costas, carrinho com toggle Pix (−5%), checkout funcional e botão flutuante do WhatsApp (5511994281802).

## Primary end-to-end flow (record)

**Pré-condição:** backend Go em `:8080` e frontend Vite em `:5173` já rodando. Cart localStorage limpo.

### 1. Hero carregou e branding NEST aplicado
- **Ação:** abrir `http://127.0.0.1:5173/`, aguardar LoadingScreen sair
- **Asserções:**
  - Logo NEST visível no header (texto "NEST" ou marca gráfica)
  - Hero contém as 3 palavras `FEITO`, `PRA QUEM`, `SABE.` visíveis
  - Cor de destaque em verde neon `#39FF14` presente (badge "-5% pix" nos cards, texto "comprar →")
  - **Broken ≠ working**: se tagline fosse a antiga "STREET WEAR AUTORAL.", o hero mostraria 3 palavras diferentes — teste falha imediato

### 2. Backend real servindo produtos (não fallback)
- **Ação:** inspect request `GET /api/products` no Network tab, ou verificar que primeiro card do grid é "Boxy Tee CORE"
- **Asserção:** primeiro produto = `Boxy Tee CORE` com preço `R$ 199,90` (cents 19990)
- **Broken ≠ working**: se backend estivesse offline, o fallback ainda mostra os mesmos produtos, mas o Network tab mostraria `GET /api/products` falhando. Evidência decisiva: comparar com `curl http://127.0.0.1:8080/api/products` em paralelo (já feito — retorna `p-core-tee`)

### 3. Hover frente/costas no Boxy Tee CORE
- **Ação:** hover sobre o card "Boxy Tee CORE"
- **Asserções:**
  - **Frente (sem hover):** SVG mostra um pequeno retângulo acento + texto pequeno "NEST" no peito
  - **Costas (com hover):** SVG muda para letras grandes `NEST` com letter-spacing + texto secundário `STREETWEAR ——` visível. Badge "costas" aparece em branco na parte inferior do card
  - **Broken ≠ working**: se o `backImage` não estivesse sendo swapped, o SVG ficaria idêntico no hover — a diferença entre "pequeno emblema" vs "NEST gigante + STREETWEAR" é visualmente inequívoca

### 4. Modal e adicionar ao carrinho
- **Ação:** clicar no card → modal abre → selecionar size `M`, color `preto` → clicar "Adicionar"
- **Asserções:**
  - Modal abre com nome "Boxy Tee CORE" e preço `R$ 199,90`
  - Após adicionar, carrinho abre automaticamente mostrando `1 item`
  - Linha de produto no carrinho: Boxy Tee CORE · M · preto · qty 1

### 5. Toggle Pix ↔ Cartão
- **Ação:** no carrinho aberto, alternar entre Pix e Cartão
- **Asserções:**
  - Com Pix **ligado** (default): total = `R$ 189,91`
  - Com Pix **desligado**: total = `R$ 199,90`
  - **Broken ≠ working**: valores específicos; se o toggle não atualizasse o total, veríamos `R$ 199,90` fixo

### 6. Checkout completo
- **Ação:** preencher form (nome, email, endereço, CEP), submeter
- **Asserções:**
  - Resposta `POST /api/checkout` → 200
  - Tela "Pedido confirmado!" aparece dentro do carrinho
  - `orderId` exibido começa com `ord_`
  - Total confirmado = `R$ 189,91` (se Pix) ou `R$ 199,90` (se cartão)
  - **Broken ≠ working**: se backend/CORS/validação falhassem, banner de erro apareceria em vez da tela de sucesso

### 7. WhatsApp flutuante abre wa.me correto
- **Ação:** clicar no botão verde neon flutuante no canto inferior direito
- **Asserção:** nova aba abre com URL contendo `wa.me/5511994281802`
- **Broken ≠ working**: se o número estivesse com o placeholder antigo (`5511999999999`), a URL seria diferente

## Regression (rápido, não gravar)
- Security headers no response do backend (CSP, HSTS, X-Frame-Options, nosniff) — via curl

## Evidência
- Vídeo contínuo cobrindo fluxos 1–7
- Screenshot lado-a-lado: frente vs costas do card
- Output de `curl` pra headers de segurança e `/api/products`
