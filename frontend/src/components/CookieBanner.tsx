import { useState } from "react";

const STORAGE_KEY = "nast:cookie-consent:v1";

// CookieBanner shows a simple LGPD-compliant consent notice until the user
// acknowledges it. We don't currently set any non-essential cookies, so the
// banner is purely informational / legal-cover — acknowledgement is stored
// in localStorage to avoid re-displaying on every visit.

function initialVisibility(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return !window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return false;
  }
}

export function CookieBanner() {
  const [visible, setVisible] = useState<boolean>(initialVisibility);

  if (!visible) return null;

  const accept = () => {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({ acceptedAt: new Date().toISOString(), version: 1 }),
      );
    } catch {
      /* ignore */
    }
    setVisible(false);
  };

  return (
    <div
      role="dialog"
      aria-live="polite"
      aria-label="Aviso de cookies"
      className="fixed inset-x-4 bottom-4 z-40 rounded-xl border border-white/10 bg-[var(--color-bg)]/95 px-5 py-4 shadow-2xl backdrop-blur-xl md:inset-x-auto md:right-6 md:max-w-md"
    >
      <div className="text-xs leading-relaxed text-white/75">
        Usamos apenas cookies necessários para o carrinho e pagamento
        funcionarem. Dados pessoais são tratados conforme a nossa{" "}
        <a className="underline" href="/privacidade">
          Política de Privacidade
        </a>
        .
      </div>
      <button
        type="button"
        onClick={accept}
        className="mt-3 border border-white/30 px-4 py-2 text-[10px] uppercase tracking-widest text-white hover:border-white"
      >
        Entendi
      </button>
    </div>
  );
}
