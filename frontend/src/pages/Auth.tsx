import { useState } from "react";
import { Header } from "../components/Header";
import { Footer } from "../components/Footer";
import { authApi } from "../api";
import { useAuth } from "../lib/useAuth";

type Mode = "login" | "signup";

type Props = {
  mode: Mode;
  whatsAppNumber: string;
  // redirectTo lets callers (e.g. the checkout gate) tell the auth
  // page where to send the user once they're signed in. Defaults to
  // "/" so a direct navigation to /login returns to the storefront.
  redirectTo?: string;
};

// AuthPage is the combined /login and /cadastro screen. It shares the
// styling / layout between both modes and toggles between them inline
// via a link at the bottom. A separate route per mode is kept so the
// URL is shareable and the back-button behaves naturally.
export function AuthPage({ mode, whatsAppNumber, redirectTo = "/" }: Props) {
  const auth = useAuth();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  // Surface "?auth_error=..." from the Google OAuth callback so users
  // aren't left staring at a blank form when something goes wrong.
  const googleError = useGoogleAuthErrorMessage();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submitting) return;
    setFormError(null);
    setSubmitting(true);
    try {
      if (mode === "signup") {
        await auth.signup({ name: name.trim(), email: email.trim(), password });
      } else {
        await auth.login({ email: email.trim(), password });
      }
      window.location.assign(redirectTo);
    } catch (err) {
      setFormError(err instanceof Error ? mapApiError(err.message) : "Erro inesperado");
    } finally {
      setSubmitting(false);
    }
  };

  const handleGoogle = () => {
    // Full navigation instead of fetch — the OAuth flow redirects
    // through accounts.google.com and back to /api/auth/google/callback,
    // which then 302s to GOOGLE_POST_LOGIN_URL. That only works on a
    // top-level navigation.
    window.location.assign(authApi.googleStartUrl());
  };

  const otherMode: Mode = mode === "login" ? "signup" : "login";
  const heading = mode === "signup" ? "Criar conta" : "Entrar";
  const cta = mode === "signup" ? "Criar conta" : "Entrar";

  return (
    <div className="noise relative min-h-full">
      <Header cartCount={0} onOpenCart={() => { /* no cart on auth pages */ }} />
      <main className="mx-auto flex max-w-md flex-col px-6 py-16">
        <div className="eyebrow text-white/40">acesso</div>
        <h1 className="mt-2 text-3xl font-black tracking-tight text-white md:text-4xl">
          {heading}
        </h1>
        <p className="mt-3 text-sm text-white/60">
          {mode === "signup"
            ? "Crie sua conta para acompanhar pedidos e rastreio."
            : "Acesse para ver seus pedidos e códigos de rastreio."}
        </p>

        {googleError && (
          <div className="mt-6 rounded-xl border border-red-500/40 bg-red-500/10 p-4 text-sm text-red-100">
            {googleError}
          </div>
        )}
        {auth.disabled && (
          <div className="mt-6 rounded-xl border border-yellow-400/40 bg-yellow-400/10 p-4 text-sm text-yellow-100">
            Login ainda não está configurado no servidor. Tente novamente em
            alguns minutos.
          </div>
        )}

        <button
          type="button"
          onClick={handleGoogle}
          disabled={auth.disabled}
          className="mt-6 flex h-12 items-center justify-center gap-3 rounded-2xl border border-white/20 bg-white text-sm font-semibold text-black transition hover:bg-white/90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <GoogleGlyph />
          <span>Entrar com Google</span>
        </button>

        <div className="relative my-6 flex items-center">
          <div className="h-px flex-1 bg-white/10" />
          <span className="px-3 text-xs uppercase tracking-[0.2em] text-white/40">
            ou
          </span>
          <div className="h-px flex-1 bg-white/10" />
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {mode === "signup" && (
            <Field
              label="Nome completo"
              type="text"
              name="name"
              autoComplete="name"
              required
              minLength={2}
              maxLength={120}
              value={name}
              onChange={setName}
              disabled={submitting}
            />
          )}
          <Field
            label="Email"
            type="email"
            name="email"
            autoComplete={mode === "signup" ? "email" : "username"}
            required
            value={email}
            onChange={setEmail}
            disabled={submitting}
          />
          <Field
            label="Senha"
            type="password"
            name="password"
            autoComplete={mode === "signup" ? "new-password" : "current-password"}
            required
            minLength={8}
            value={password}
            onChange={setPassword}
            disabled={submitting}
            hint={
              mode === "signup"
                ? "Mínimo 8 caracteres, com letras e números."
                : undefined
            }
          />

          {formError && (
            <div className="rounded-xl border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-100">
              {formError}
            </div>
          )}

          <button
            type="submit"
            disabled={submitting || auth.disabled}
            className="mt-2 h-12 rounded-2xl bg-white text-sm font-semibold uppercase tracking-[0.2em] text-black transition hover:bg-white/90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting ? "..." : cta}
          </button>
        </form>

        <p className="mt-6 text-center text-sm text-white/60">
          {mode === "signup" ? "Já tem conta?" : "Novo por aqui?"}{" "}
          <a
            href={otherMode === "signup" ? "/cadastro" : "/login"}
            className="text-white underline decoration-white/30 underline-offset-4 hover:decoration-white"
          >
            {otherMode === "signup" ? "Criar conta" : "Entrar"}
          </a>
        </p>
      </main>
      <Footer whatsAppNumber={whatsAppNumber} />
    </div>
  );
}

function Field({
  label,
  hint,
  value,
  onChange,
  ...rest
}: Omit<React.InputHTMLAttributes<HTMLInputElement>, "onChange"> & {
  label: string;
  hint?: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1.5 text-sm text-white/80">
      <span>{label}</span>
      <input
        {...rest}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-11 rounded-xl border border-white/15 bg-white/5 px-3 text-white outline-none transition focus:border-white/40 focus:bg-white/10 disabled:opacity-50"
      />
      {hint && <span className="text-xs text-white/40">{hint}</span>}
    </label>
  );
}

// mapApiError converts the backend's raw "API 409: {json}" strings
// into user-friendly Portuguese. Unknown errors pass through so we
// never eat debugging information.
function mapApiError(raw: string): string {
  if (raw.includes("email já cadastrado")) {
    return "Este email já está cadastrado. Entre com sua conta ou use outro email.";
  }
  if (raw.includes("credenciais inválidas")) {
    return "Email ou senha incorretos.";
  }
  if (raw.includes("senha")) {
    return raw.replace(/^API \d+:\s*\{"error":"/, "").replace(/"\}$/, "");
  }
  if (raw.startsWith("API 503")) {
    return "Login temporariamente indisponível. Tente novamente em instantes.";
  }
  return raw;
}

// Parses the "?auth_error=..." tacked onto the URL by the Google
// OAuth failure redirect and returns a user-facing sentence.
function useGoogleAuthErrorMessage(): string | null {
  if (typeof window === "undefined") return null;
  const url = new URL(window.location.href);
  const code = url.searchParams.get("auth_error");
  if (!code) return null;
  switch (code) {
    case "email_collision":
      return "Este email já está vinculado a outra conta Google. Entre com email e senha.";
    case "state_mismatch":
      return "Sessão de login expirou. Tente novamente.";
    case "exchange_failed":
    case "missing_claims":
      return "Não conseguimos validar sua conta Google. Tente novamente.";
    case "unavailable":
      return "Login com Google ainda não está configurado. Use email e senha.";
    default:
      return "Erro ao entrar com Google. Tente novamente.";
  }
}

function GoogleGlyph() {
  // Inline SVG so we don't load an external file for a single icon.
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.258h2.908c1.702-1.567 2.684-3.875 2.684-6.615Z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A9 9 0 0 0 9 18Z"
      />
      <path
        fill="#FBBC05"
        d="M3.964 10.707A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.707V4.96H.957A9 9 0 0 0 0 9c0 1.452.348 2.827.957 4.04l3.007-2.333Z"
      />
      <path
        fill="#EA4335"
        d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.89 11.426 0 9 0A9 9 0 0 0 .957 4.96L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58Z"
      />
    </svg>
  );
}
