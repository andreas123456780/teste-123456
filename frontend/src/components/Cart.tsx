import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useMemo, useRef, useState } from "react";
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

// FormState holds every recipient field SuperFrete / our backend
// require. `address` is the logradouro only; `addressNumber` and
// `addressComplement` are kept separate so we can ship a structured
// address to the label API and save the admin from guessing where
// "Rua X 123 fundos ap 4" ends and starts.
type FormState = {
  name: string;
  email: string;
  document: string; // CPF formatted with mask, digits-only sent to API
  zipCode: string;
  address: string;
  addressNumber: string;
  addressComplement: string;
  district: string;
  city: string;
  state: string;
};

function maskCpf(raw: string): string {
  const d = raw.replace(/\D/g, "").slice(0, 11);
  const parts: string[] = [];
  if (d.length > 0) parts.push(d.slice(0, Math.min(3, d.length)));
  if (d.length > 3) parts.push(d.slice(3, Math.min(6, d.length)));
  if (d.length > 6) parts.push(d.slice(6, Math.min(9, d.length)));
  const head = parts.join(".");
  const tail = d.length > 9 ? "-" + d.slice(9, 11) : "";
  return head + tail;
}

// Client-side mirrors of the Go validators. Keep the logic aligned so
// the form can show errors before a round-trip to /api/checkout.
function isFullName(s: string): boolean {
  const fields = s.trim().split(/\s+/).filter(Boolean);
  if (fields.length < 2) return false;
  return fields.every((f) => f.length >= 2);
}

function isCpfOrCnpjDigits(s: string): boolean {
  const d = s.replace(/\D/g, "");
  if (d.length !== 11 && d.length !== 14) return false;
  return !/^(\d)\1+$/.test(d);
}

const UF_LIST = [
  "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA",
  "MT", "MS", "MG", "PA", "PB", "PR", "PE", "PI", "RJ", "RN",
  "RS", "RO", "RR", "SC", "SP", "SE", "TO",
];

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
    document: "",
    zipCode: "",
    address: "",
    addressNumber: "",
    addressComplement: "",
    district: "",
    city: "",
    state: "",
  });
  // Track which fields ViaCEP filled for us so we can mark them
  // read-only and hint to the user why. If ViaCEP returns an empty
  // value for any field we keep that field editable.
  const [cepLoading, setCepLoading] = useState(false);
  const [cepError, setCepError] = useState<string | null>(null);
  const cepRequestRef = useRef(0);
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
  // Coupon state. `draftCode` is what the user typed; `applied` is the
  // server-validated result, only used once the user clicks Aplicar.
  const [draftCode, setDraftCode] = useState("");
  const [couponLoading, setCouponLoading] = useState(false);
  const [couponError, setCouponError] = useState<string | null>(null);
  const [applied, setApplied] = useState<{
    code: string;
    itemDiscountCents: number;
    shippingDiscountCents: number;
  } | null>(null);

  // Auto-fill logradouro/bairro/cidade/uf whenever the user types a
  // full (8-digit) CEP. We debounce 400ms so the backend isn't hit on
  // every keystroke, and race-guard via a monotonic request id so a
  // slow response for an earlier CEP can't overwrite newer state.
  useEffect(() => {
    const digits = form.zipCode.replace(/\D/g, "");
    setCepError(null);
    if (digits.length !== 8) {
      setCepLoading(false);
      return;
    }
    const id = ++cepRequestRef.current;
    setCepLoading(true);
    const handle = setTimeout(() => {
      api
        .lookupCep(digits)
        .then((res) => {
          if (id !== cepRequestRef.current) return;
          setForm((f) => ({
            ...f,
            address: res.logradouro || f.address,
            district: res.bairro || f.district,
            city: res.cidade || f.city,
            state: res.uf || f.state,
          }));
          setCepError(null);
        })
        .catch((err: unknown) => {
          if (id !== cepRequestRef.current) return;
          const msg = err instanceof Error ? err.message : "";
          if (msg.includes("404")) {
            setCepError("CEP não encontrado. Preencha manualmente.");
          } else {
            setCepError("Não foi possível buscar o CEP. Preencha manualmente.");
          }
        })
        .finally(() => {
          if (id === cepRequestRef.current) setCepLoading(false);
        });
    }, 400);
    return () => clearTimeout(handle);
  }, [form.zipCode]);

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
  const rawSubtotal = usePix ? totalPix : totalCard;
  const rawShippingCents = shipping?.priceCents ?? 0;
  const itemDiscount = applied?.itemDiscountCents ?? 0;
  const shippingDiscount = applied?.shippingDiscountCents ?? 0;
  const subtotal = Math.max(0, rawSubtotal - itemDiscount);
  const shippingCents = Math.max(0, rawShippingCents - shippingDiscount);
  const total = subtotal + shippingCents;

  // Dropping the shipping selection or emptying the cart invalidates a
  // previously-applied free-shipping coupon — refresh it so the user
  // isn't silently given 0 shipping on a cart that no longer qualifies.
  const applyCoupon = async () => {
    setCouponError(null);
    const code = draftCode.trim().toUpperCase();
    if (!code) return;
    setCouponLoading(true);
    try {
      const res = await api.validateCoupon({
        code,
        subtotalCents: rawSubtotal,
        shippingCents: rawShippingCents,
      });
      setApplied({
        code: res.coupon.code,
        itemDiscountCents: res.discount.itemDiscountCents,
        shippingDiscountCents: res.discount.shippingDiscountCents,
      });
      setDraftCode(res.coupon.code);
    } catch (err) {
      setApplied(null);
      setCouponError(
        err instanceof Error
          ? err.message.replace(/^API \d+: /, "")
          : "cupom inv\u00e1lido",
      );
    } finally {
      setCouponLoading(false);
    }
  };

  const removeCoupon = () => {
    setApplied(null);
    setCouponError(null);
    setDraftCode("");
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (items.length === 0) return;
    // Run the client-side validators before hitting the API so the
    // user sees an immediate error instead of a 400 from the backend.
    if (!isFullName(form.name)) {
      setError("Digite seu nome completo (mínimo 2 palavras).");
      return;
    }
    if (!isCpfOrCnpjDigits(form.document)) {
      setError("CPF inválido. Digite os 11 dígitos.");
      return;
    }
    if (form.zipCode.replace(/\D/g, "").length !== 8) {
      setError("CEP incompleto.");
      return;
    }
    if (!form.address.trim() || !form.addressNumber.trim()) {
      setError("Endereço e número são obrigatórios.");
      return;
    }
    if (!form.district.trim() || !form.city.trim() || !UF_LIST.includes(form.state)) {
      setError("Endereço incompleto — confira CEP, bairro, cidade e UF.");
      return;
    }
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
        document: form.document.replace(/\D/g, ""),
        address: form.address,
        addressNumber: form.addressNumber,
        addressComplement: form.addressComplement || undefined,
        district: form.district,
        city: form.city,
        state: form.state,
        zipCode: form.zipCode,
        paymentMethod: method,
        shipping: shipping
          ? {
              serviceId: shipping.serviceId,
              serviceName: shipping.serviceName,
              priceCents: shipping.priceCents,
            }
          : undefined,
        couponCode: applied?.code,
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

            <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
              <div className="px-6 py-4">
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

                <div className="grid grid-cols-6 gap-2">
                  <input
                    required
                    placeholder="Nome completo"
                    autoComplete="name"
                    className="col-span-6 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.name}
                    onChange={(e) =>
                      setForm({ ...form, name: e.target.value })
                    }
                  />
                  <input
                    required
                    type="email"
                    placeholder="Email"
                    autoComplete="email"
                    className="col-span-6 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.email}
                    onChange={(e) =>
                      setForm({ ...form, email: e.target.value })
                    }
                  />
                  <input
                    required
                    inputMode="numeric"
                    placeholder="CPF"
                    autoComplete="off"
                    maxLength={14}
                    className="col-span-6 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.document}
                    onChange={(e) =>
                      setForm({ ...form, document: maskCpf(e.target.value) })
                    }
                  />
                  <div className="col-span-6 flex items-center justify-between gap-3 text-[11px] uppercase tracking-[0.25em] text-white/40">
                    <span>endereço de entrega</span>
                    {cepLoading ? (
                      <span className="text-white/60">buscando CEP…</span>
                    ) : null}
                  </div>
                  <input
                    required
                    placeholder="Logradouro (rua, avenida)"
                    autoComplete="address-line1"
                    className="col-span-4 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.address}
                    onChange={(e) =>
                      setForm({ ...form, address: e.target.value })
                    }
                  />
                  <input
                    required
                    placeholder="Número"
                    inputMode="numeric"
                    autoComplete="off"
                    maxLength={24}
                    className="col-span-2 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.addressNumber}
                    onChange={(e) =>
                      setForm({ ...form, addressNumber: e.target.value })
                    }
                  />
                  <input
                    placeholder="Complemento (opcional)"
                    autoComplete="address-line2"
                    maxLength={120}
                    className="col-span-6 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.addressComplement}
                    onChange={(e) =>
                      setForm({ ...form, addressComplement: e.target.value })
                    }
                  />
                  <input
                    required
                    placeholder="Bairro"
                    autoComplete="address-level3"
                    maxLength={120}
                    className="col-span-3 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.district}
                    onChange={(e) =>
                      setForm({ ...form, district: e.target.value })
                    }
                  />
                  <input
                    required
                    placeholder="Cidade"
                    autoComplete="address-level2"
                    maxLength={120}
                    className="col-span-2 border border-white/15 bg-transparent px-3 py-2.5 text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                    value={form.city}
                    onChange={(e) =>
                      setForm({ ...form, city: e.target.value })
                    }
                  />
                  <select
                    required
                    aria-label="UF"
                    className="col-span-1 border border-white/15 bg-transparent px-2 py-2.5 text-sm text-white outline-none focus:border-[var(--color-accent)]"
                    value={form.state}
                    onChange={(e) =>
                      setForm({ ...form, state: e.target.value })
                    }
                  >
                    <option value="" className="bg-black">UF</option>
                    {UF_LIST.map((uf) => (
                      <option key={uf} value={uf} className="bg-black">
                        {uf}
                      </option>
                    ))}
                  </select>
                  {cepError ? (
                    <div className="col-span-6 text-[11px] text-amber-300">
                      {cepError}
                    </div>
                  ) : null}
                </div>

                <div className="flex flex-col gap-2 border-t border-white/10 pt-3">
                  <label className="eyebrow text-white/50">Cupom</label>
                  {applied ? (
                    <div className="flex items-center justify-between border border-[var(--color-accent)]/40 bg-[var(--color-accent)]/5 px-3 py-2">
                      <div>
                        <div className="text-xs font-black uppercase tracking-[0.3em] text-[var(--color-accent)]">
                          {applied.code}
                        </div>
                        <div className="text-[10px] uppercase tracking-[0.25em] text-white/50">
                          -{formatBRL(applied.itemDiscountCents + applied.shippingDiscountCents)}
                        </div>
                      </div>
                      <button
                        type="button"
                        onClick={removeCoupon}
                        className="text-[11px] uppercase tracking-[0.25em] text-white/60 hover:text-white"
                      >
                        remover
                      </button>
                    </div>
                  ) : (
                    <div className="flex gap-2">
                      <input
                        type="text"
                        inputMode="text"
                        autoCapitalize="characters"
                        placeholder="código"
                        value={draftCode}
                        onChange={(e) =>
                          setDraftCode(e.target.value.toUpperCase())
                        }
                        className="flex-1 border border-white/15 bg-transparent px-3 py-2 text-sm font-mono uppercase tracking-widest text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
                      />
                      <button
                        type="button"
                        onClick={applyCoupon}
                        disabled={couponLoading || !draftCode.trim()}
                        className="border border-white/20 px-4 text-[11px] font-bold uppercase tracking-[0.25em] text-white/80 hover:border-white hover:text-white disabled:opacity-50"
                      >
                        {couponLoading ? "..." : "aplicar"}
                      </button>
                    </div>
                  )}
                  {couponError && (
                    <div className="text-[11px] text-[var(--color-accent-warn)]">
                      {couponError}
                    </div>
                  )}
                </div>

                <div className="flex flex-col gap-1 border-t border-white/10 pt-3 text-white">
                  <div className="flex items-center justify-between text-[11px] uppercase tracking-[0.3em] text-white/50">
                    <span>Subtotal</span>
                    <span className="font-mono text-white/80">
                      {formatBRL(rawSubtotal)}
                    </span>
                  </div>
                  {applied && itemDiscount > 0 && (
                    <div className="flex items-center justify-between text-[11px] uppercase tracking-[0.3em] text-[var(--color-accent)]">
                      <span>Cupom</span>
                      <span className="font-mono">
                        -{formatBRL(itemDiscount)}
                      </span>
                    </div>
                  )}
                  <div className="flex items-center justify-between text-[11px] uppercase tracking-[0.3em] text-white/50">
                    <span>Frete</span>
                    <span className="font-mono text-white/80">
                      {shipping ? formatBRL(shippingCents) : "—"}
                    </span>
                  </div>
                  {applied && shippingDiscount > 0 && (
                    <div className="flex items-center justify-between text-[11px] uppercase tracking-[0.3em] text-[var(--color-accent)]">
                      <span>Frete grátis</span>
                      <span className="font-mono">
                        -{formatBRL(shippingDiscount)}
                      </span>
                    </div>
                  )}
                  <div className="mt-1 flex items-center justify-between">
                    <div className="eyebrow text-white/60">Total</div>
                    <div className="text-right">
                      <div className="text-xl font-black text-white">
                        {formatBRL(total)}
                      </div>
                      {usePix && totalCard > totalPix && (
                        <div className="text-[11px] text-white/40 line-through">
                          {formatBRL(totalCard + rawShippingCents)}
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
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}
