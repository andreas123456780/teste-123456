import { AnimatePresence, motion } from "framer-motion";
import { useMemo, useState } from "react";
import type { CartItem } from "../types";
import { api, type ShippingOption } from "../api";
import { formatBRL } from "../utils/format";
import { Close, Minus, Plus, Lock, WhatsApp } from "./icons";
import { ProductArt } from "./ProductArt";
import { ShippingQuote } from "./ShippingQuote";
import { StripePaymentStep } from "./StripePaymentStep";

type Props = {
  open: boolean;
  items: CartItem[];
  onClose: () => void;
  onUpdateQty: (id: string, size: string, color: string, qty: number) => void;
  onRemove: (id: string, size: string, color: string) => void;
  onClear: () => void;
  whatsAppNumber: string;
};

// Pix via Stripe is not universally enabled (BR-only, needs CNPJ review
// in the Stripe dashboard). Until it is, we route Pix checkouts through
// WhatsApp: the order is still persisted on the backend so we keep the
// audit trail + stock reservation, but the customer finishes payment by
// chat with the operator, who manually sends the Pix QR code and later
// marks the order as paid in /admin.
function buildPixWhatsAppLink(opts: {
  phone: string;
  orderId: string;
  amountCents: number;
  items: CartItem[];
  name: string;
  zipCode: string;
  address: string;
}): string {
  const lines = [
    `Olá! Quero finalizar meu pedido NAST #${opts.orderId} via Pix.`,
    "",
    "*Itens:*",
    ...opts.items.map(
      (it) =>
        `• ${it.quantity}× ${it.product.name} (${it.size} · ${it.color})`,
    ),
    "",
    `*Total:* ${formatBRL(opts.amountCents)}`,
    `*Nome:* ${opts.name}`,
    `*CEP:* ${opts.zipCode}`,
    `*Endereço:* ${opts.address}`,
    "",
    "Por favor, me envie a chave Pix ou o QR Code.",
  ];
  return `https://wa.me/${opts.phone}?text=${encodeURIComponent(lines.join("\n"))}`;
}

type FormState = { name: string; email: string; address: string; zipCode: string };

export function Cart({
  open,
  items,
  onClose,
  onUpdateQty,
  onRemove,
  onClear,
  whatsAppNumber,
}: Props) {
  const [form, setForm] = useState<FormState>({
    name: "",
    email: "",
    address: "",
    zipCode: "",
  });
  const [loading, setLoading] = useState(false);
  const [order, setOrder] = useState<{
    id: string;
    total: number;
    token: string;
    method: "pix" | "card";
    whatsAppHref?: string;
  } | null>(null);
  const [payment, setPayment] = useState<
    | {
        clientSecret: string;
        orderId: string;
        orderToken: string;
        amountCents: number;
        method: "pix" | "card";
      }
    | null
  >(null);
  const [error, setError] = useState<string | null>(null);
  const [usePix, setUsePix] = useState(true);
  const [shipping, setShipping] = useState<ShippingOption | null>(null);

  const handleClose = () => {
    setOrder(null);
    setPayment(null);
    onClose();
  };

  const totalCard = useMemo(
    () => items.reduce((acc, it) => acc + it.product.priceCents * it.quantity, 0),
    [items],
  );
  const totalPix = useMemo(
    () => items.reduce((acc, it) => acc + it.product.pixPriceCents * it.quantity, 0),
    [items],
  );
  const subtotal = usePix ? totalPix : totalCard;
  const shippingCents = shipping?.priceCents ?? 0;
  const total = subtotal + shippingCents;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (items.length === 0) return;
    setLoading(true);
    setError(null);
    try {
      const method: "pix" | "card" = usePix ? "pix" : "card";
      const checkoutRes = await api.checkout({
        items: items.map((it) => ({
          productId: it.product.id,
          quantity: it.quantity,
          size: it.size,
          color: it.color,
        })),
        name: form.name,
        email: form.email,
        address: form.address,
        zipCode: form.zipCode,
        paymentMethod: method,
        shipping: shipping
          ? {
              serviceId: shipping.serviceId,
              serviceName: shipping.serviceName,
              priceCents: shipping.priceCents,
            }
          : undefined,
      });
      if (method === "pix") {
        const waHref = buildPixWhatsAppLink({
          phone: whatsAppNumber,
          orderId: checkoutRes.orderId,
          amountCents: checkoutRes.amountCents,
          items,
          name: form.name,
          zipCode: form.zipCode,
          address: form.address,
        });
        // Open WhatsApp in a new tab first (user gesture is still in
        // scope inside a submit handler), then reveal the confirmation
        // screen in case the popup is blocked or the user returns.
        window.open(waHref, "_blank", "noopener,noreferrer");
        setOrder({
          id: checkoutRes.orderId,
          total: checkoutRes.amountCents,
          token: checkoutRes.orderToken,
          method: "pix",
          whatsAppHref: waHref,
        });
        setShipping(null);
        onClear();
        return;
      }
      const intent = await api.createPaymentIntent(checkoutRes.orderId);
      setPayment({
        clientSecret: intent.clientSecret,
        orderId: checkoutRes.orderId,
        orderToken: checkoutRes.orderToken,
        amountCents: checkoutRes.amountCents,
        method,
      });
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Não foi possível iniciar o pagamento. Tente novamente.",
      );
    } finally {
      setLoading(false);
    }
  };

  const handlePaid = () => {
    if (!payment) return;
    setOrder({
      id: payment.orderId,
      total: payment.amountCents,
      token: payment.orderToken,
      method: payment.method,
    });
    setPayment(null);
    setShipping(null);
    onClear();
  };

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={handleClose}
            className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm"
          />
          <motion.aside
            role="dialog"
            aria-modal="true"
            aria-label="Sacola"
            initial={{ x: "100%" }}
            animate={{ x: 0 }}
            exit={{ x: "100%" }}
            transition={{ type: "spring", stiffness: 260, damping: 32 }}
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-md flex-col border-l border-white/10 bg-[var(--color-bg)]/95 backdrop-blur-xl"
          >
            <div className="flex items-center justify-between border-b border-white/10 px-6 py-5">
              <div>
                <div className="eyebrow text-white/50">sacola</div>
                <div className="mt-1 text-xl font-black text-white">
                  {items.length} {items.length === 1 ? "item" : "itens"}
                </div>
              </div>
              <motion.button
                onClick={handleClose}
                whileHover={{ rotate: 90 }}
                transition={{ type: "spring", stiffness: 260 }}
                className="border border-white/10 p-2 text-white/70 hover:text-white"
                aria-label="Fechar sacola"
              >
                <Close className="h-4 w-4" />
              </motion.button>
            </div>

            <div className="flex-1 overflow-y-auto px-6 py-4">
              {payment ? (
                <StripePaymentStep
                  clientSecret={payment.clientSecret}
                  amountCents={payment.amountCents}
                  method={payment.method}
                  onPaid={handlePaid}
                  onCancel={() => setPayment(null)}
                />
              ) : order ? (
                <motion.div
                  initial={{ opacity: 0, scale: 0.95 }}
                  animate={{ opacity: 1, scale: 1 }}
                  className="flex h-full flex-col items-center justify-center text-center"
                >
                  <motion.div
                    initial={{ scale: 0, rotate: -90 }}
                    animate={{ scale: 1, rotate: 0 }}
                    transition={{ type: "spring", stiffness: 220 }}
                    className="flex h-20 w-20 items-center justify-center bg-[var(--color-accent)]"
                  >
                    <svg
                      width="36"
                      height="36"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="black"
                      strokeWidth="3"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    >
                      <path d="M4 12l5 5L20 6" />
                    </svg>
                  </motion.div>
                  <h3 className="mt-6 text-2xl font-black text-white">
                    {order.method === "pix"
                      ? "Pedido reservado!"
                      : "Pedido confirmado!"}
                  </h3>
                  <p className="mt-2 text-sm text-white/60">
                    Código:{" "}
                    <span className="font-mono text-white">{order.id}</span>
                  </p>
                  <p className="mt-1 text-sm text-white/60">
                    Total:{" "}
                    <span className="font-bold text-[var(--color-accent)]">
                      {formatBRL(order.total)}
                    </span>
                  </p>
                  {order.method === "pix" && (
                    <p className="mt-4 max-w-xs text-xs text-white/70">
                      Abrimos o WhatsApp pra você finalizar o Pix com o
                      atendimento. Se a janela não abrir, clique no botão
                      abaixo.
                    </p>
                  )}
                  {order.method === "pix" && order.whatsAppHref && (
                    <a
                      href={order.whatsAppHref}
                      target="_blank"
                      rel="noreferrer noopener"
                      className="mt-6 inline-flex items-center gap-2 bg-[var(--color-accent)] px-6 py-3 text-xs font-bold uppercase tracking-[0.3em] text-black hover:bg-white"
                    >
                      <WhatsApp className="h-4 w-4" />
                      Pagar no WhatsApp
                    </a>
                  )}
                  {order.token && (
                    <a
                      href={`/pedido/${order.token}`}
                      className="mt-4 border border-[var(--color-accent)] px-6 py-3 text-xs font-bold uppercase tracking-[0.3em] text-[var(--color-accent)] hover:bg-[var(--color-accent)] hover:text-black"
                    >
                      Acompanhar pedido
                    </a>
                  )}
                  <motion.button
                    whileHover={{ y: -1 }}
                    onClick={() => {
                      setOrder(null);
                      onClose();
                    }}
                    className="mt-4 border border-white/20 px-6 py-3 text-xs font-bold uppercase tracking-[0.3em] text-white hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                  >
                    Continuar comprando
                  </motion.button>
                </motion.div>
              ) : items.length === 0 ? (
                <motion.div
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  className="flex h-full flex-col items-center justify-center text-center text-white/50"
                >
                  <div className="text-5xl">∅</div>
                  <p className="mt-4 uppercase tracking-widest">
                    Sacola vazia.
                  </p>
                  <p className="mt-1 text-xs">Escolha uma peça do drop.</p>
                </motion.div>
              ) : (
                <ul className="space-y-3">
                  <AnimatePresence initial={false}>
                    {items.map((it) => (
                      <motion.li
                        key={`${it.product.id}-${it.size}-${it.color}`}
                        layout
                        initial={{ opacity: 0, x: 40 }}
                        animate={{ opacity: 1, x: 0 }}
                        exit={{ opacity: 0, x: 80 }}
                        className="flex gap-3 border border-white/10 bg-white/[0.03] p-3"
                      >
                        <div className="h-20 w-20 shrink-0 overflow-hidden border border-white/10 bg-black/40">
                          <ProductArt
                            image={it.product.image}
                            color={it.color}
                            size={80}
                          />
                        </div>
                        <div className="flex flex-1 flex-col">
                          <div className="flex items-start justify-between gap-2">
                            <div>
                              <div className="text-sm font-bold text-white">
                                {it.product.name}
                              </div>
                              <div className="text-[11px] uppercase tracking-widest text-white/50">
                                {it.size} · {it.color}
                              </div>
                            </div>
                            <button
                              aria-label="Remover"
                              onClick={() =>
                                onRemove(it.product.id, it.size, it.color)
                              }
                              className="text-white/40 hover:text-white"
                            >
                              <Close className="h-4 w-4" />
                            </button>
                          </div>
                          <div className="mt-auto flex items-center justify-between pt-2">
                            <div className="flex items-center gap-1 border border-white/10 bg-black/40 px-1 py-1">
                              <button
                                aria-label="Diminuir"
                                onClick={() =>
                                  onUpdateQty(
                                    it.product.id,
                                    it.size,
                                    it.color,
                                    Math.max(1, it.quantity - 1),
                                  )
                                }
                                className="p-1 text-white hover:bg-white/10"
                              >
                                <Minus className="h-3 w-3" />
                              </button>
                              <span className="w-5 text-center text-sm font-bold text-white">
                                {it.quantity}
                              </span>
                              <button
                                aria-label="Aumentar"
                                onClick={() =>
                                  onUpdateQty(
                                    it.product.id,
                                    it.size,
                                    it.color,
                                    it.quantity + 1,
                                  )
                                }
                                className="p-1 text-white hover:bg-white/10"
                              >
                                <Plus className="h-3 w-3" />
                              </button>
                            </div>
                            <div className="text-sm font-black text-white">
                              {formatBRL(
                                (usePix
                                  ? it.product.pixPriceCents
                                  : it.product.priceCents) * it.quantity,
                              )}
                            </div>
                          </div>
                        </div>
                      </motion.li>
                    ))}
                  </AnimatePresence>
                </ul>
              )}
            </div>

            {!order && !payment && items.length > 0 && (
              <form
                onSubmit={submit}
                className="flex flex-col gap-3 border-t border-white/10 px-6 py-4"
              >
                <div className="flex items-center justify-between border border-white/10 bg-black/40 p-2">
                  <span className="text-[11px] font-bold uppercase tracking-[0.3em] text-white/60">
                    Pagar com
                  </span>
                  <div className="flex gap-1">
                    {(["pix", "card"] as const).map((m) => {
                      const active = usePix === (m === "pix");
                      return (
                        <button
                          type="button"
                          key={m}
                          onClick={() => setUsePix(m === "pix")}
                          className={`relative px-3 py-1 text-[11px] font-bold uppercase tracking-[0.3em] transition ${
                            active ? "text-black" : "text-white/60 hover:text-white"
                          }`}
                        >
                          {active && (
                            <motion.span
                              layoutId="pay-pill"
                              className="absolute inset-0 bg-[var(--color-accent)]"
                            />
                          )}
                          <span className="relative">
                            {m === "pix" ? "pix (zap) -5%" : "cartão"}
                          </span>
                        </button>
                      );
                    })}
                  </div>
                </div>

                <ShippingQuote
                  items={items}
                  zipCode={form.zipCode}
                  onZipChange={(z) => setForm((f) => ({ ...f, zipCode: z }))}
                  selectedServiceId={shipping?.serviceId ?? null}
                  onSelect={(o) => setShipping(o)}
                />

                <div className="grid grid-cols-2 gap-2">
                  <input
                    required
                    placeholder="Nome"
                    className="col-span-2 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.name}
                    onChange={(e) =>
                      setForm({ ...form, name: e.target.value })
                    }
                  />
                  <input
                    required
                    type="email"
                    placeholder="Email"
                    className="col-span-2 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.email}
                    onChange={(e) =>
                      setForm({ ...form, email: e.target.value })
                    }
                  />
                  <input
                    required
                    placeholder="Endereço"
                    className="col-span-2 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.address}
                    onChange={(e) =>
                      setForm({ ...form, address: e.target.value })
                    }
                  />
                </div>

                <div className="flex flex-col gap-1 border-t border-white/10 pt-3 text-white">
                  <div className="flex items-center justify-between text-[11px] uppercase tracking-[0.3em] text-white/50">
                    <span>Subtotal</span>
                    <span className="font-mono text-white/80">
                      {formatBRL(subtotal)}
                    </span>
                  </div>
                  <div className="flex items-center justify-between text-[11px] uppercase tracking-[0.3em] text-white/50">
                    <span>Frete</span>
                    <span className="font-mono text-white/80">
                      {shipping ? formatBRL(shippingCents) : "—"}
                    </span>
                  </div>
                  <div className="mt-1 flex items-center justify-between">
                    <div className="eyebrow text-white/60">Total</div>
                    <div className="text-right">
                      <div className="text-xl font-black">
                        {formatBRL(total)}
                      </div>
                      {usePix && totalCard > totalPix && (
                        <div className="text-[11px] text-white/40 line-through">
                          {formatBRL(totalCard + shippingCents)}
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                {error && (
                  <div className="text-xs text-[var(--color-accent-warn)]">
                    {error}
                  </div>
                )}

                <motion.button
                  type="submit"
                  disabled={loading}
                  whileHover={{ y: -1 }}
                  whileTap={{ scale: 0.97 }}
                  className="inline-flex items-center justify-center gap-2 bg-[var(--color-accent)] px-6 py-4 text-xs font-black uppercase tracking-[0.3em] text-black transition hover:bg-white disabled:opacity-60"
                >
                  <Lock className="h-4 w-4" />
                  {loading ? "Processando..." : "Finalizar compra"}
                </motion.button>
              </form>
            )}
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}
