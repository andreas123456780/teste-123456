package main

// Transactional email (order confirmation) via Resend.
//
// Integration is optional: if RESEND_API_KEY is unset or empty, the
// worker is a no-op and the rest of the pipeline runs unchanged. This
// lets us ship without a Resend account and plug it in later.
//
// We keep the integration small (stdlib net/http, JSON POST) so there's
// no new dep beyond what modernc.org/sqlite already added.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	resendAPIBase   = "https://api.resend.com"
	resendHTTPTimeout = 10 * time.Second
)

// resendAPIBaseOverride lets tests redirect outbound calls.
var resendAPIBaseOverride = ""

func resendBaseURL() string {
	if resendAPIBaseOverride != "" {
		return resendAPIBaseOverride
	}
	return resendAPIBase
}

// emailConfig is hydrated from environment at startup.
type emailConfig struct {
	APIKey   string // RESEND_API_KEY
	From     string // EMAIL_FROM (e.g. "NAST <contato@nast.example.com>")
	AppURL   string // APP_URL — used to build order lookup URLs
	TokenKey []byte // HMAC key for order tokens
}

func loadEmailConfig(tokenKey []byte, appURL string) emailConfig {
	return emailConfig{
		APIKey:   strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		From:     strings.TrimSpace(os.Getenv("EMAIL_FROM")),
		AppURL:   appURL,
		TokenKey: tokenKey,
	}
}

// emailWorker consumes order ids and sends the confirmation email.
// Implements emailEnqueuer.
type emailWorker struct {
	cfg    emailConfig
	orders *orderStore
	http   *http.Client
	queue  chan string
}

func newEmailWorker(cfg emailConfig, orders *orderStore) *emailWorker {
	w := &emailWorker{
		cfg:    cfg,
		orders: orders,
		http:   &http.Client{Timeout: resendHTTPTimeout},
		queue:  make(chan string, 256),
	}
	go w.run()
	return w
}

func (w *emailWorker) Enqueue(orderID string) {
	if w == nil || orderID == "" {
		return
	}
	if w.cfg.APIKey == "" || w.cfg.From == "" {
		return
	}
	select {
	case w.queue <- orderID:
	default:
		log.Printf("email_job: queue full, dropping order=%s", orderID)
	}
}

func (w *emailWorker) run() {
	for id := range w.queue {
		if err := w.process(id); err != nil {
			log.Printf("email_job: send(%s): %v", id, err)
		}
	}
}

func (w *emailWorker) process(orderID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	o, ok, err := w.orders.get(ctx, orderID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("order not found")
	}
	link := fmt.Sprintf("%s/pedido/%s",
		strings.TrimRight(w.cfg.AppURL, "/"),
		makeOrderToken(w.cfg.TokenKey, o.ID, time.Now()),
	)
	subject := "Pedido confirmado · NAST"
	html := renderConfirmationHTML(o, link)
	text := renderConfirmationText(o, link)

	payload := map[string]any{
		"from":    w.cfg.From,
		"to":      []string{o.Email},
		"subject": subject,
		"html":    html,
		"text":    text,
		"tags": []map[string]string{
			{"name": "type", "value": "order_confirmation"},
			{"name": "orderId", "value": o.ID},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		resendBaseURL()+"/emails", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)

	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend %d: %s", resp.StatusCode, truncate(string(body), 256))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func renderConfirmationText(o *pendingOrder, link string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Olá, %s!\n\n", o.Name)
	fmt.Fprintf(&b, "Recebemos seu pedido NAST (%s) com sucesso.\n", o.ID)
	fmt.Fprintf(&b, "Total: R$ %s (frete R$ %s, %s)\n\n",
		moneyBRL(o.AmountCents), moneyBRL(o.ShippingCents), o.ShippingSvcName)
	b.WriteString("Itens:\n")
	for _, it := range o.Items {
		fmt.Fprintf(&b, "  • %s", it.ProductName)
		if it.Size != "" {
			fmt.Fprintf(&b, " · tam %s", it.Size)
		}
		if it.Color != "" {
			fmt.Fprintf(&b, " · %s", it.Color)
		}
		fmt.Fprintf(&b, " · %dx R$ %s\n", it.Quantity, moneyBRL(it.UnitPriceCents))
	}
	fmt.Fprintf(&b, "\nAcompanhe seu pedido: %s\n\n— NAST\n", link)
	return b.String()
}

func renderConfirmationHTML(o *pendingOrder, link string) string {
	var items strings.Builder
	for _, it := range o.Items {
		variant := ""
		if it.Size != "" || it.Color != "" {
			parts := []string{}
			if it.Size != "" {
				parts = append(parts, "tam "+htmlEscape(it.Size))
			}
			if it.Color != "" {
				parts = append(parts, htmlEscape(it.Color))
			}
			variant = " · " + strings.Join(parts, " · ")
		}
		fmt.Fprintf(&items,
			`<tr><td style="padding:8px 0;border-bottom:1px solid #222;color:#eee;font:14px/1.4 system-ui,sans-serif">%s%s</td>`+
				`<td style="padding:8px 0;border-bottom:1px solid #222;color:#eee;font:14px/1.4 system-ui,sans-serif;text-align:right">%dx R$ %s</td></tr>`,
			htmlEscape(it.ProductName), variant, it.Quantity, moneyBRL(it.UnitPriceCents),
		)
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><title>Pedido NAST</title></head>
<body style="margin:0;background:#0a0a0a;color:#eee;font-family:system-ui,sans-serif">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
    <table role="presentation" width="520" cellpadding="0" cellspacing="0" style="max-width:520px;background:#101010;border:1px solid #1f1f1f;border-radius:14px">
      <tr><td style="padding:28px 28px 12px">
        <h1 style="margin:0;font:600 22px/1.2 system-ui;letter-spacing:.08em;color:#fff">NAST</h1>
        <p style="margin:6px 0 0;color:#8a8a8a;font:12px/1.2 system-ui;letter-spacing:.12em;text-transform:uppercase">pedido confirmado</p>
      </td></tr>
      <tr><td style="padding:16px 28px">
        <p style="margin:0 0 12px;color:#eee;font:15px/1.5 system-ui">Olá, <strong>%s</strong> — recebemos seu pedido.</p>
        <p style="margin:0 0 20px;color:#8a8a8a;font:13px/1.5 system-ui">Código: <code style="color:#eee">%s</code></p>
        <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">%s
          <tr><td style="padding:12px 0;color:#8a8a8a;font:13px/1.4 system-ui">Frete (%s)</td>
              <td style="padding:12px 0;color:#eee;font:13px/1.4 system-ui;text-align:right">R$ %s</td></tr>
          <tr><td style="padding:4px 0 0;color:#eee;font:600 15px/1.4 system-ui">Total</td>
              <td style="padding:4px 0 0;color:#39ff14;font:600 15px/1.4 system-ui;text-align:right">R$ %s</td></tr>
        </table>
      </td></tr>
      <tr><td style="padding:8px 28px 32px">
        <a href="%s" style="display:inline-block;padding:12px 20px;border:1px solid #39ff14;color:#39ff14;text-decoration:none;font:600 13px/1 system-ui;letter-spacing:.12em;text-transform:uppercase;border-radius:8px">Acompanhar pedido</a>
      </td></tr>
      <tr><td style="padding:16px 28px 28px;border-top:1px solid #1f1f1f;color:#565656;font:11px/1.5 system-ui">
        Este email foi enviado porque você finalizou uma compra em NAST. Em caso de dúvida, responda a esta mensagem.
      </td></tr>
    </table>
  </td></tr></table>
</body></html>`,
		htmlEscape(o.Name), htmlEscape(o.ID), items.String(),
		htmlEscape(o.ShippingSvcName), moneyBRL(o.ShippingCents), moneyBRL(o.AmountCents), htmlEscape(link),
	)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

// moneyBRL formats cents as Brazilian currency (no R$ prefix, no
// thousands separator needed at this scale — prices are < 1k BRL).
func moneyBRL(cents int) string {
	if cents < 0 {
		return "-" + moneyBRL(-cents)
	}
	return fmt.Sprintf("%d,%02d", cents/100, cents%100)
}
