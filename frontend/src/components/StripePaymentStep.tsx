import { loadStripe, type Stripe } from "@stripe/stripe-js";
import {
  Elements,
  PaymentElement,
  useElements,
  useStripe,
} from "@stripe/react-stripe-js";
import { useMemo, useState } from "react";
import { Lock } from "./icons";
import { formatBRL } from "../utils/format";

const publishableKey = import.meta.env.VITE_STRIPE_PUBLISHABLE_KEY ?? "";

// We lazy-load the Stripe.js instance and memoize it at module scope so
// we don't re-fetch the script on every render. Returning null here causes
// <Elements> to show nothing until the key is configured — callers should
// check MissingKey first so we can render a useful message instead.
let stripePromiseCache: Promise<Stripe | null> | null = null;
function getStripePromise(): Promise<Stripe | null> | null {
  if (!publishableKey) return null;
  if (!stripePromiseCache) {
    stripePromiseCache = loadStripe(publishableKey);
  }
  return stripePromiseCache;
}

type Props = {
  clientSecret: string;
  amountCents: number;
  method: "pix" | "card";
  onPaid: () => void;
  onCancel: () => void;
};

export function StripePaymentStep(props: Props) {
  const stripePromise = useMemo(() => getStripePromise(), []);
  if (!publishableKey || !stripePromise) {
    return (
      <div className="flex flex-col gap-3 border border-[var(--color-accent-warn)]/40 bg-[var(--color-accent-warn)]/10 p-4 text-xs text-white">
        <div className="font-bold uppercase tracking-[0.3em]">
          Pagamento indisponível
        </div>
        <p className="text-white/70">
          Chave Stripe não configurada (VITE_STRIPE_PUBLISHABLE_KEY). Ajuste
          as variáveis de ambiente e recarregue a página.
        </p>
        <button
          onClick={props.onCancel}
          className="self-start border border-white/20 px-3 py-1.5 uppercase tracking-[0.3em]"
        >
          Voltar
        </button>
      </div>
    );
  }

  return (
    <Elements
      stripe={stripePromise}
      options={{
        clientSecret: props.clientSecret,
        appearance: {
          theme: "night",
          variables: {
            colorPrimary: "#39FF14",
            colorBackground: "#0a0a0a",
            colorText: "#ffffff",
            colorDanger: "#ff2a2a",
            fontFamily: "Inter, system-ui, sans-serif",
            spacingUnit: "4px",
            borderRadius: "0px",
          },
        },
      }}
    >
      <StripeConfirmForm {...props} />
    </Elements>
  );
}

function StripeConfirmForm({ amountCents, method, onPaid, onCancel }: Props) {
  const stripe = useStripe();
  const elements = useElements();
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!stripe || !elements) return;
    setSubmitting(true);
    setErr(null);
    const { error, paymentIntent } = await stripe.confirmPayment({
      elements,
      // We stay in the SPA — no return_url redirect. Stripe will still
      // redirect for methods that require it (e.g. 3DS cards).
      redirect: "if_required",
      confirmParams: {
        return_url: window.location.origin,
      },
    });
    setSubmitting(false);
    if (error) {
      setErr(error.message ?? "Pagamento recusado. Tente novamente.");
      return;
    }
    // For PIX, the intent stays `requires_action` until the QR code is
    // scanned. Treat any non-failure status as "user is on the flow" and
    // hand off to the confirmation screen, which will show a success or
    // pending message based on the final webhook outcome.
    if (
      paymentIntent?.status === "succeeded" ||
      paymentIntent?.status === "processing" ||
      paymentIntent?.status === "requires_action"
    ) {
      onPaid();
    } else {
      setErr(`Status inesperado: ${paymentIntent?.status ?? "desconhecido"}`);
    }
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-3">
      <div className="flex items-center justify-between border border-white/10 bg-black/40 p-3 text-[11px] uppercase tracking-[0.3em] text-white/70">
        <span>{method === "pix" ? "Pagar com Pix" : "Pagar com cartão"}</span>
        <span className="font-mono text-white">{formatBRL(amountCents)}</span>
      </div>
      <PaymentElement />
      {err && <div className="text-xs text-[var(--color-accent-warn)]">{err}</div>}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="flex-1 border border-white/15 px-4 py-3 text-xs font-bold uppercase tracking-[0.3em] text-white/70 hover:text-white"
        >
          Voltar
        </button>
        <button
          type="submit"
          disabled={!stripe || submitting}
          className="flex flex-[2] items-center justify-center gap-2 bg-[var(--color-accent)] px-6 py-3 text-xs font-black uppercase tracking-[0.3em] text-black transition hover:bg-white disabled:opacity-60"
        >
          <Lock className="h-4 w-4" />
          {submitting ? "Processando..." : "Pagar agora"}
        </button>
      </div>
    </form>
  );
}
