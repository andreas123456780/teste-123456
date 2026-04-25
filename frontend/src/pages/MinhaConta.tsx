import { useEffect, useState } from "react";
import { Header } from "../components/Header";
import { Footer } from "../components/Footer";
import { accountApi, type MyOrder } from "../api";
import { useAuth } from "../lib/useAuth";
import { formatBRL } from "../utils/format";

type Props = {
  whatsAppNumber: string;
};

// MinhaConta is the authenticated customer's order history page. It
// is a thin client of /api/account/orders — everything else lives on
// the backend (tracking links, coupon codes, per-order tokens). When
// the user lands here unauthenticated we redirect to /login?next=
// so they can sign in and come back to the same URL.
export function MinhaConta({ whatsAppNumber }: Props) {
  const auth = useAuth();
  const [orders, setOrders] = useState<MyOrder[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  // When auth resolves, decide what to do: fetch orders if
  // authenticated, redirect to /login if anonymous, or show a 503
  // message if the backend disabled auth entirely. Each branch is
  // short and guarded so the effect body only ever runs setState
  // through the async path — the anonymous branch does a top-level
  // navigation instead (no setState), and the disabled branch is
  // handled as derived state below so the effect stays pure-ish.
  useEffect(() => {
    if (auth.loading || auth.disabled) return;
    if (!auth.user) {
      const next = encodeURIComponent("/minha-conta");
      window.location.replace(`/login?next=${next}`);
      return;
    }
    let alive = true;
    accountApi
      .myOrders()
      .then((list) => {
        if (alive) setOrders(list);
      })
      .catch((err: unknown) => {
        if (!alive) return;
        setError(err instanceof Error ? err.message : "Erro ao carregar pedidos");
      });
    return () => {
      alive = false;
    };
  }, [auth.loading, auth.disabled, auth.user]);

  // Derived state: if the backend reported auth disabled, surface a
  // friendly message without going through setState-in-effect.
  const disabledMessage = auth.disabled
    ? "Login ainda não está habilitado neste ambiente."
    : null;

  const content = (() => {
    if (disabledMessage) {
      return <MessageState title="Login indisponível" detail={disabledMessage} />;
    }
    if (auth.loading || (!error && !orders)) {
      return <LoadingState />;
    }
    if (error) {
      return <MessageState title="Não foi possível carregar seus pedidos" detail={error} />;
    }
    if (!orders || orders.length === 0) {
      return (
        <MessageState
          title="Nenhum pedido ainda"
          detail="Quando você finalizar uma compra, ela aparece aqui com o código de rastreio."
        />
      );
    }
    return (
      <ul className="flex flex-col gap-4">
        {orders.map((o) => (
          <OrderCard key={o.id} order={o} />
        ))}
      </ul>
    );
  })();

  return (
    <div className="min-h-screen bg-black text-white">
      <Header cartCount={0} onOpenCart={() => {}} />
      <main className="mx-auto max-w-4xl px-6 pb-20 pt-36">
        <div className="mb-8 flex items-end justify-between">
          <div>
            <p className="text-xs font-bold uppercase tracking-[0.4em] text-white/40">
              Minha conta
            </p>
            <h1 className="mt-2 font-display text-4xl font-black tracking-tight">
              Seus pedidos
            </h1>
          </div>
          {auth.user && (
            <div className="text-right text-xs uppercase tracking-[0.2em] text-white/60">
              {auth.user.email}
            </div>
          )}
        </div>
        {content}
      </main>
      <Footer whatsAppNumber={whatsAppNumber} />
    </div>
  );
}

function LoadingState() {
  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] p-8 text-center text-sm text-white/60">
      Carregando seus pedidos…
    </div>
  );
}

function MessageState({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] p-8 text-center">
      <p className="font-display text-lg font-bold">{title}</p>
      <p className="mt-2 text-sm text-white/60">{detail}</p>
    </div>
  );
}

// statusLabel maps the backend status enum to a Portuguese noun the
// customer can actually read. Falls back to the raw string so future
// statuses don't produce a blank cell.
function statusLabel(status: string): { text: string; tone: string } {
  switch (status) {
    case "paid":
      return { text: "Pago — aguardando envio", tone: "text-[var(--color-accent)]" };
    case "shipped":
      return { text: "Enviado", tone: "text-emerald-400" };
    case "cancelled":
      return { text: "Cancelado", tone: "text-white/40" };
    case "failed":
      return { text: "Pagamento recusado", tone: "text-red-400" };
    default:
      return { text: status, tone: "text-white/60" };
  }
}

function OrderCard({ order }: { order: MyOrder }) {
  const created = new Date(order.createdAt);
  const { text: statusText, tone } = statusLabel(order.status);
  return (
    <li className="flex flex-col gap-4 rounded-xl border border-white/10 bg-white/[0.03] p-5">
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 pb-3">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.3em] text-white/40">
            #{order.id}
          </p>
          <p className="mt-1 text-sm text-white/70">
            {created.toLocaleDateString("pt-BR", {
              day: "2-digit",
              month: "long",
              year: "numeric",
            })}
          </p>
        </div>
        <div className="text-right">
          <p className={`text-xs font-semibold uppercase tracking-[0.2em] ${tone}`}>
            {statusText}
          </p>
          <p className="mt-1 font-display text-xl font-black">
            {formatBRL(order.amountCents)}
          </p>
        </div>
      </header>

      <ul className="flex flex-col gap-1 text-sm text-white/80">
        {order.items.map((it, i) => (
          <li key={i} className="flex items-baseline justify-between gap-4">
            <span className="truncate">
              {it.quantity}× {it.productName}
              {it.size ? ` · ${it.size}` : ""}
              {it.color ? ` · ${it.color}` : ""}
            </span>
            <span className="shrink-0 font-mono text-xs text-white/50">
              {formatBRL(it.unitPriceCents * it.quantity)}
            </span>
          </li>
        ))}
      </ul>

      {(order.trackingCode || order.trackingUrl || order.orderToken) && (
        <footer className="flex flex-wrap items-center justify-between gap-2 border-t border-white/10 pt-3 text-xs uppercase tracking-[0.2em]">
          {order.trackingCode && (
            <div className="text-white/60">
              Rastreio:{" "}
              <span className="font-mono text-white">{order.trackingCode}</span>
            </div>
          )}
          <div className="flex gap-3">
            {order.trackingUrl && (
              <a
                href={order.trackingUrl}
                target="_blank"
                rel="noreferrer"
                className="border border-white/20 px-3 py-1.5 text-[11px] font-bold text-white/70 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                Rastrear
              </a>
            )}
            {order.orderToken && (
              <a
                href={`/pedido/${encodeURIComponent(order.orderToken)}`}
                className="bg-white px-3 py-1.5 text-[11px] font-bold text-black transition hover:bg-[var(--color-accent)]"
              >
                Detalhes
              </a>
            )}
          </div>
        </footer>
      )}
    </li>
  );
}
