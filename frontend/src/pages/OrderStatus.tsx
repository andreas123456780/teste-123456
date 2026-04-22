import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import type { PublicOrder } from "../types";
import { formatBRL } from "../utils/format";
import { Header } from "../components/Header";
import { Footer } from "../components/Footer";

type Props = {
  token: string;
  whatsAppNumber: string;
};

// OrderStatus renders the customer-facing /pedido/:token page. It polls
// the backend every 8s while the order isn't shipped — the label job is
// asynchronous so the tracking code may take up to ~30s to appear after
// payment confirmation.
export function OrderStatus({ token, whatsAppNumber }: Props) {
  const [order, setOrder] = useState<PublicOrder | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const o = await api.getOrderByToken(token);
        if (!cancelled) {
          setOrder(o);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setError(
            err instanceof Error
              ? err.message
              : "Não foi possível carregar este pedido.",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    const id = window.setInterval(load, 8000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [token]);

  const statusLabel = useMemo(() => statusText(order?.status), [order?.status]);

  return (
    <div className="noise relative min-h-full">
      <Header cartCount={0} onOpenCart={() => { /* cart hidden here */ }} />
      <main className="mx-auto max-w-2xl px-6 py-16">
        <div className="eyebrow text-white/40">pedido</div>
        <h1 className="mt-2 text-3xl font-black tracking-tight text-white md:text-4xl">
          {statusLabel.title}
        </h1>
        <p className="mt-3 text-sm text-white/60">{statusLabel.subtitle}</p>

        {loading && !order && (
          <div className="mt-12 animate-pulse text-sm text-white/50">
            Carregando…
          </div>
        )}
        {error && !order && (
          <div className="mt-10 rounded-lg border border-red-500/40 bg-red-500/10 p-5 text-sm text-red-200">
            {error}
          </div>
        )}

        {order && (
          <>
            <section className="mt-10 rounded-xl border border-white/10 bg-white/[0.02] p-6">
              <div className="flex items-center justify-between">
                <div>
                  <div className="eyebrow text-white/40">código</div>
                  <div className="mt-1 font-mono text-sm text-white/80">
                    {order.orderId}
                  </div>
                </div>
                <StatusBadge status={order.status} />
              </div>
              <div className="mt-5 space-y-2 text-sm text-white/80">
                {order.items.map((it) => (
                  <div
                    key={it.productId + (it.size ?? "") + (it.color ?? "")}
                    className="flex justify-between"
                  >
                    <div>
                      <span>{it.productName}</span>
                      {(it.size || it.color) && (
                        <span className="text-white/40">
                          {" "}
                          · {[it.size, it.color].filter(Boolean).join(" · ")}
                        </span>
                      )}
                      <span className="text-white/40"> · {it.quantity}x</span>
                    </div>
                    <div className="text-white/60">
                      {formatBRL(it.unitPriceCents * it.quantity)}
                    </div>
                  </div>
                ))}
              </div>
              <div className="mt-5 space-y-1 border-t border-white/10 pt-4 text-sm">
                <div className="flex justify-between text-white/50">
                  <span>Subtotal</span>
                  <span>{formatBRL(order.totalCents)}</span>
                </div>
                <div className="flex justify-between text-white/50">
                  <span>
                    Frete
                    {order.shippingName ? ` (${order.shippingName})` : ""}
                  </span>
                  <span>{formatBRL(order.shippingCents)}</span>
                </div>
                <div className="flex justify-between pt-1 text-white">
                  <span className="font-black">Total</span>
                  <span className="font-black">
                    {formatBRL(order.amountCents)}
                  </span>
                </div>
              </div>
            </section>

            <section className="mt-6 rounded-xl border border-white/10 bg-white/[0.02] p-6 text-sm">
              <div className="eyebrow text-white/40">envio</div>
              {order.trackingCode ? (
                <>
                  <div className="mt-2 text-white/80">
                    Código de rastreio:{" "}
                    <span className="font-mono text-white">
                      {order.trackingCode}
                    </span>
                  </div>
                  {order.trackingUrl && (
                    <a
                      href={order.trackingUrl}
                      target="_blank"
                      rel="noreferrer noopener"
                      className="mt-3 inline-block border border-white/30 px-4 py-2 text-xs uppercase tracking-widest text-white hover:border-white"
                    >
                      Rastrear no Correios
                    </a>
                  )}
                </>
              ) : order.status === "paid" ? (
                <div className="mt-2 text-white/60">
                  Estamos gerando sua etiqueta. O código de rastreio aparecerá
                  aqui em alguns instantes.
                </div>
              ) : order.status === "pending_payment" ? (
                <div className="mt-2 text-white/60">
                  Aguardando confirmação do pagamento.
                </div>
              ) : (
                <div className="mt-2 text-white/60">
                  Seu pedido foi registrado mas não foi pago. Em caso de
                  dúvida, fale com a gente.
                </div>
              )}
            </section>

            <section className="mt-6 text-xs text-white/40">
              Pedido feito por <span className="text-white/60">{order.customer}</span> em{" "}
              {new Date(order.createdAt).toLocaleString("pt-BR")}.
            </section>
          </>
        )}

        <div className="mt-12">
          <a
            href="/"
            className="text-xs uppercase tracking-widest text-white/60 hover:text-white"
          >
            ← voltar à loja
          </a>
        </div>
      </main>
      <Footer whatsAppNumber={whatsAppNumber} />
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const cls =
    status === "paid" || status === "shipped"
      ? "border-emerald-400/50 text-emerald-300"
      : status === "failed"
        ? "border-red-400/50 text-red-300"
        : "border-white/20 text-white/60";
  return (
    <span
      className={`rounded-full border px-3 py-1 text-[10px] uppercase tracking-widest ${cls}`}
    >
      {statusText(status).badge}
    </span>
  );
}

function statusText(status: string | undefined): {
  title: string;
  subtitle: string;
  badge: string;
} {
  switch (status) {
    case "paid":
      return {
        title: "Pagamento confirmado",
        subtitle: "Seu pedido está sendo preparado.",
        badge: "pago",
      };
    case "shipped":
      return {
        title: "Pedido a caminho",
        subtitle: "Sua etiqueta foi emitida. Acompanhe o rastreio abaixo.",
        badge: "enviado",
      };
    case "failed":
      return {
        title: "Pagamento não aprovado",
        subtitle: "Tente novamente ou use outro método.",
        badge: "falhou",
      };
    case "pending_payment":
      return {
        title: "Aguardando pagamento",
        subtitle:
          "Conclua o pagamento para que possamos preparar seu pedido.",
        badge: "pendente",
      };
    default:
      return {
        title: "Detalhes do pedido",
        subtitle: "",
        badge: status ?? "—",
      };
  }
}
