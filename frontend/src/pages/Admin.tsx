import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  adminApi,
  adminAuthApi,
  adminCouponsApi,
  adminOrdersApi,
  adminSettingsApi,
  type AdminCouponPayload,
  type AdminOrder,
  type AdminProductPayload,
  type AdminStats,
  type BannerSettings,
  type CountdownSettings,
  type SecretSettings,
} from "../api";
import type { Coupon, Product } from "../types";
import { CountdownView } from "../components/Countdown";

// Admin is an intentionally plain, no-deps management screen. Operators
// authenticate either with username+password (preferred, when the
// backend has ADMIN_USERNAME + ADMIN_PASSWORD_HASH configured) or by
// pasting a static ADMIN_TOKEN as a fallback. The issued session token
// is persisted to localStorage; every request carries X-Admin-Token
// and the backend enforces authentication (HMAC verify or constant-time
// compare). The UI never ships privileged data statically.
const TOKEN_STORAGE = "nast:admin-token:v1";

type Tab = "dashboard" | "orders" | "products" | "coupons" | "settings";

function emptyProduct(): AdminProductPayload {
  return {
    id: "",
    name: "",
    description: "",
    priceCents: 0,
    pixPriceCents: 0,
    category: "",
    image: "",
    backImage: "",
    colors: [],
    sizes: [],
    tags: [],
    stock: 0,
    stockBySize: {},
    transparentImage: false,
    hidden: false,
  };
}

/** Sum of every per-size entry. The totals must stay in sync with the
 * legacy `stock` integer the backend still stores (used for the catalog
 * table + coupon minimums), so we recompute it whenever a size input
 * changes. */
function sumStockBySize(m: Record<string, number>): number {
  let total = 0;
  for (const v of Object.values(m)) total += Number.isFinite(v) ? v : 0;
  return total;
}

function emptyCoupon(): AdminCouponPayload {
  return {
    code: "",
    kind: "percent",
    value: 10,
    minSubtotalCents: 0,
    maxUses: 0,
    active: true,
    note: "",
  };
}

function readToken(): string {
  try {
    return window.localStorage.getItem(TOKEN_STORAGE) ?? "";
  } catch {
    return "";
  }
}

function moneyBR(cents: number): string {
  return (cents / 100).toLocaleString("pt-BR", {
    style: "currency",
    currency: "BRL",
  });
}

function parseCsv(value: string): string[] {
  return value
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

// CsvInput is a controlled input that keeps the user's raw typing
// (commas, trailing spaces, etc.) intact while exposing a parsed
// string array via onChange. The previous version used `value.join(", ")`
// directly as the input value — that meant every keystroke parsed +
// rejoined the string, so trailing commas got eaten and the user could
// only ever add a single item. We hold the raw string locally and only
// re-sync from props when an outside change happens (e.g. switching to
// edit a different product). On blur we normalize the displayed text
// to the canonical "a, b, c" form so it doesn't accumulate stray
// whitespace across save/load cycles.
function CsvInput({
  value,
  onChange,
  className,
}: {
  value: string[];
  onChange: (next: string[]) => void;
  className?: string;
}) {
  const canonical = value.join(", ");
  const [text, setText] = useState(canonical);
  const lastCanonicalRef = useRef(canonical);
  // Re-sync from props only when the canonical form changes externally
  // (e.g. the parent reset to a different product). This keeps the user
  // free to type anything in between without us clobbering it.
  useEffect(() => {
    if (canonical !== lastCanonicalRef.current) {
      lastCanonicalRef.current = canonical;
      setText(canonical);
    }
  }, [canonical]);
  return (
    <input
      value={text}
      onChange={(e) => {
        const raw = e.target.value;
        setText(raw);
        const parsed = parseCsv(raw);
        const nextCanonical = parsed.join(", ");
        // Only push upstream when the parsed list actually changes —
        // typing a trailing comma or extra space shouldn't trigger a
        // re-render with the same array shape.
        if (nextCanonical !== lastCanonicalRef.current) {
          lastCanonicalRef.current = nextCanonical;
          onChange(parsed);
        }
      }}
      onBlur={() => {
        // On blur, normalize the displayed string so it matches the
        // canonical "a, b, c" — clears stray spaces / trailing commas.
        const parsed = parseCsv(text);
        const nextCanonical = parsed.join(", ");
        setText(nextCanonical);
        lastCanonicalRef.current = nextCanonical;
      }}
      className={className}
    />
  );
}

// ImageUploadField renders a drag-and-drop / click-to-upload card with a
// live preview. The underlying value is still the plain URL string, so
// older products (referencing /products/*.jpg in the repo) keep working
// — the upload just swaps in the Vercel Blob URL.
function ImageUploadField({
  label,
  value,
  onChange,
  token,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  token: string;
}) {
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const inputId = `upload-${label.replace(/\W+/g, "-").toLowerCase()}`;

  const preview = value
    ? value.startsWith("http") || value.startsWith("/")
      ? value
      : `/products/${value}`
    : "";

  const handleFile = async (file: File) => {
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      setError("Só JPG/PNG/WEBP/GIF.");
      return;
    }
    if (file.size > 8 * 1024 * 1024) {
      setError("Arquivo maior que 8 MB.");
      return;
    }
    setError(null);
    setUploading(true);
    try {
      const { url } = await adminApi.uploadImage(token, file);
      onChange(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha no upload");
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <span className="text-sm">{label}</span>
      <label
        htmlFor={inputId}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragOver(false);
          const f = e.dataTransfer.files?.[0];
          if (f) void handleFile(f);
        }}
        className={
          "flex cursor-pointer flex-col items-center justify-center rounded border-2 border-dashed p-3 text-center text-xs transition " +
          (dragOver
            ? "border-black bg-neutral-100"
            : "border-neutral-300 bg-neutral-50 hover:bg-neutral-100")
        }
      >
        {preview ? (
          <img
            src={preview}
            alt=""
            className="mb-2 h-28 w-28 rounded object-cover"
            onError={(e) => {
              (e.currentTarget as HTMLImageElement).style.opacity = "0.3";
            }}
          />
        ) : (
          <div className="mb-2 flex h-28 w-28 items-center justify-center rounded bg-neutral-200 text-2xl text-neutral-400">
            +
          </div>
        )}
        <span className="text-neutral-700">
          {uploading ? "Enviando…" : "Clique ou arraste uma imagem"}
        </span>
        <input
          id={inputId}
          type="file"
          accept="image/*"
          className="hidden"
          disabled={uploading}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void handleFile(f);
            e.target.value = "";
          }}
        />
      </label>
      {value && (
        <div className="flex items-center gap-2 text-xs">
          <input
            value={value}
            onChange={(e) => onChange(e.target.value)}
            className="flex-1 rounded border border-neutral-300 px-2 py-1 font-mono"
          />
          <button
            type="button"
            onClick={() => onChange("")}
            className="rounded border border-neutral-300 px-2 py-1 text-neutral-600 hover:bg-neutral-100"
            title="Remover"
          >
            Limpar
          </button>
        </div>
      )}
      {error && <span className="text-xs text-red-600">{error}</span>}
    </div>
  );
}

export function AdminPage() {
  const [token, setToken] = useState<string>(readToken);
  const [tab, setTab] = useState<Tab>("dashboard");

  const onLoggedIn = (newToken: string) => {
    try {
      window.localStorage.setItem(TOKEN_STORAGE, newToken);
    } catch {
      // localStorage unavailable (private mode) — still set in-memory.
    }
    setToken(newToken);
  };

  const logout = () => {
    try {
      window.localStorage.removeItem(TOKEN_STORAGE);
    } catch {
      // noop
    }
    setToken("");
  };

  if (!token) {
    return (
      <div className="min-h-screen bg-neutral-50 text-neutral-900">
        <LoginForm onLoggedIn={onLoggedIn} />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-neutral-50 text-neutral-900">
      <main className="mx-auto max-w-5xl px-6 py-10">
      <header className="mb-6 flex items-center justify-between">
        <h1 className="text-3xl font-bold">NAST — Admin</h1>
        <button
          type="button"
          onClick={logout}
          className="rounded border border-neutral-300 px-4 py-2 text-sm"
        >
          Sair
        </button>
      </header>

      <nav className="mb-6 flex gap-2 border-b border-neutral-300">
        {([
          ["dashboard", "Dashboard"],
          ["orders", "Pedidos"],
          ["products", "Produtos"],
          ["coupons", "Cupons"],
          ["settings", "Site"],
        ] as const).map(([id, label]) => {
          const active = tab === id;
          return (
            <button
              key={id}
              type="button"
              onClick={() => setTab(id)}
              className={`border-b-2 px-3 py-2 text-sm ${
                active
                  ? "border-black font-semibold text-black"
                  : "border-transparent text-neutral-500 hover:text-black"
              }`}
            >
              {label}
            </button>
          );
        })}
      </nav>

      {tab === "dashboard" && <DashboardAdmin token={token} />}
      {tab === "orders" && <OrdersAdmin token={token} />}
      {tab === "products" && <ProductsAdmin token={token} />}
      {tab === "coupons" && <CouponsAdmin token={token} />}
      {tab === "settings" && <SettingsAdmin token={token} />}
      </main>
    </div>
  );
}

// ----- Login form -----

function LoginForm({ onLoggedIn }: { onLoggedIn: (token: string) => void }) {
  const [mode, setMode] = useState<"credentials" | "token">("credentials");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [tokenDraft, setTokenDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const submitCredentials = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!username || !password) return;
    setBusy(true);
    setErr("");
    try {
      const res = await adminAuthApi.login(username, password);
      onLoggedIn(res.token);
    } catch (e) {
      const msg = (e as Error).message;
      if (msg.includes("503")) {
        // Login endpoint not configured — fall back to static token mode.
        setMode("token");
        setErr(
          "Login user/senha não configurado no backend. Use token estático.",
        );
      } else {
        setErr("Credenciais inválidas.");
      }
    } finally {
      setBusy(false);
    }
  };

  const submitToken = (e: React.FormEvent) => {
    e.preventDefault();
    if (!tokenDraft) return;
    onLoggedIn(tokenDraft);
  };

  return (
    <main className="mx-auto max-w-md px-6 py-20">
      <h1 className="mb-6 text-3xl font-bold">NAST — Admin</h1>
      {mode === "credentials" ? (
        <form onSubmit={submitCredentials} className="space-y-3">
          <p className="mb-4 text-sm text-neutral-600">
            Login de administrador. A sessão expira em 24h.
          </p>
          <input
            type="text"
            autoComplete="username"
            placeholder="Usuário"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="w-full rounded border border-neutral-300 px-3 py-2"
          />
          <input
            type="password"
            autoComplete="current-password"
            placeholder="Senha"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full rounded border border-neutral-300 px-3 py-2"
          />
          {err && <p className="text-sm text-red-600">{err}</p>}
          <button
            type="submit"
            disabled={busy || !username || !password}
            className="w-full rounded bg-black px-4 py-2 text-white disabled:opacity-50"
          >
            {busy ? "Entrando…" : "Entrar"}
          </button>
          <button
            type="button"
            onClick={() => {
              setErr("");
              setMode("token");
            }}
            className="w-full text-center text-xs text-neutral-500 underline"
          >
            Usar token estático (ADMIN_TOKEN)
          </button>
        </form>
      ) : (
        <form onSubmit={submitToken} className="space-y-3">
          <p className="mb-4 text-sm text-neutral-600">
            Informe o <code>ADMIN_TOKEN</code> configurado no backend.
          </p>
          <input
            type="password"
            autoComplete="off"
            placeholder="X-Admin-Token"
            value={tokenDraft}
            onChange={(e) => setTokenDraft(e.target.value)}
            className="w-full rounded border border-neutral-300 px-3 py-2"
          />
          {err && <p className="text-sm text-red-600">{err}</p>}
          <button
            type="submit"
            disabled={!tokenDraft}
            className="w-full rounded bg-black px-4 py-2 text-white disabled:opacity-50"
          >
            Entrar
          </button>
          <button
            type="button"
            onClick={() => {
              setErr("");
              setMode("credentials");
            }}
            className="w-full text-center text-xs text-neutral-500 underline"
          >
            Voltar para login user/senha
          </button>
        </form>
      )}
    </main>
  );
}

// ----- Dashboard tab -----

function DashboardAdmin({ token }: { token: string }) {
  const [stats, setStats] = useState<AdminStats | null>(null);
  const [err, setErr] = useState("");

  const refresh = useCallback(async () => {
    setErr("");
    try {
      const s = await adminAuthApi.stats(token);
      setStats(s);
    } catch (e) {
      setErr((e as Error).message);
      setStats(null);
    }
  }, [token]);

  useEffect(() => {
    // refresh() only touches state after the fetch resolves.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
  }, [refresh]);

  if (err) {
    return (
      <section className="space-y-3">
        <p className="text-sm text-red-600">Erro: {err}</p>
        <button
          type="button"
          onClick={() => void refresh()}
          className="rounded border border-neutral-300 px-3 py-1 text-sm"
        >
          Tentar novamente
        </button>
      </section>
    );
  }
  if (!stats) return <p className="text-sm text-neutral-500">Carregando…</p>;

  const paidPct =
    stats.orders.total > 0
      ? Math.round((stats.orders.paid / stats.orders.total) * 100)
      : 0;

  return (
    <section className="space-y-5">
      {/* Revenue KPIs */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Kpi
          label="Receita bruta"
          value={moneyBR(stats.revenue.grossCents)}
          sub={`${stats.revenue.paidOrderCount} pedido${stats.revenue.paidOrderCount !== 1 ? "s" : ""} pago${stats.revenue.paidOrderCount !== 1 ? "s" : ""}`}
          icon={<IcoTrend />}
        />
        <Kpi
          label="Ticket médio"
          value={moneyBR(stats.revenue.avgTicketCents)}
          color="blue"
          icon={<IcoTag />}
        />
        <Kpi
          label="Frete arrecadado"
          value={moneyBR(stats.revenue.shippingCents)}
          icon={<IcoTruck />}
        />
        <Kpi
          label="Descontos"
          value={moneyBR(stats.revenue.discountCents)}
          color="amber"
          icon={<IcoPercent />}
        />
      </div>

      {/* Orders KPIs */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Kpi
          label="Pedidos totais"
          value={stats.orders.total.toString()}
          icon={<IcoBag />}
        />
        <Kpi
          label="Pagos"
          value={stats.orders.paid.toString()}
          color="green"
          sub={paidPct > 0 ? `${paidPct}% do total` : undefined}
          icon={<IcoCheck />}
        />
        <Kpi
          label="Pendentes"
          value={stats.orders.pendingPayment.toString()}
          color="amber"
          icon={<IcoClock />}
        />
        <Kpi
          label="Cancelados"
          value={stats.orders.canceled.toString()}
          color={stats.orders.canceled > 0 ? "red" : "neutral"}
          icon={<IcoXmark />}
        />
      </div>

      {/* Chart + Top products side by side */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div className="rounded-lg border border-neutral-200 bg-white p-4">
          <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-neutral-500">
            Receita por dia (30d)
          </h2>
          {stats.revenueByDay.length === 0 ? (
            <p className="text-sm text-neutral-400">Sem receita no período.</p>
          ) : (
            <DailyRevenueChart data={stats.revenueByDay} allTimeCents={stats.revenue.grossCents} />
          )}
        </div>
        <div className="rounded-lg border border-neutral-200 bg-white p-4">
          <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-neutral-500">
            Mais vendidos
          </h2>
          {stats.topProducts.length === 0 ? (
            <p className="text-sm text-neutral-400">Nenhuma venda ainda.</p>
          ) : (
            <TopProductsChart data={stats.topProducts} />
          )}
        </div>
      </div>

      {/* Recent orders */}
      <div className="rounded-lg border border-neutral-200 bg-white">
        <div className="border-b border-neutral-100 px-4 py-3">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
            Últimos pedidos
          </h2>
        </div>
        {stats.recentOrders.length === 0 ? (
          <p className="px-4 py-3 text-sm text-neutral-400">Nenhum pedido.</p>
        ) : (
          <div className="divide-y divide-neutral-100">
            {stats.recentOrders.map((o) => (
              <RecentOrderRow key={o.id} order={o} />
            ))}
          </div>
        )}
      </div>

      <p className="text-xs text-neutral-400">
        Atualizado em {new Date(stats.generatedAt).toLocaleString("pt-BR")}.{" "}
        <button
          type="button"
          onClick={() => void refresh()}
          className="underline"
        >
          Atualizar
        </button>
      </p>
    </section>
  );
}

type KpiColor = "neutral" | "green" | "amber" | "red" | "blue";

function Kpi({
  label,
  value,
  sub,
  color = "neutral",
  icon,
}: {
  label: string;
  value: string;
  sub?: string;
  color?: KpiColor;
  icon?: ReactNode;
}) {
  const bg: Record<KpiColor, string> = {
    neutral: "border-neutral-200 bg-white",
    green: "border-green-200 bg-green-50",
    amber: "border-amber-200 bg-amber-50",
    red: "border-red-200 bg-red-50",
    blue: "border-blue-200 bg-blue-50",
  };
  const ic: Record<KpiColor, string> = {
    neutral: "text-neutral-400",
    green: "text-green-500",
    amber: "text-amber-500",
    red: "text-red-400",
    blue: "text-blue-500",
  };
  const sb: Record<KpiColor, string> = {
    neutral: "text-neutral-400",
    green: "text-green-600",
    amber: "text-amber-600",
    red: "text-red-500",
    blue: "text-blue-600",
  };
  return (
    <div className={`rounded-lg border p-4 ${bg[color]}`}>
      <div className="flex items-start justify-between gap-2">
        <p className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
          {label}
        </p>
        {icon && <span className={`mt-0.5 ${ic[color]}`}>{icon}</span>}
      </div>
      <p className="mt-2 text-2xl font-black tracking-tight text-neutral-900">
        {value}
      </p>
      {sub && <p className={`mt-1 text-xs ${sb[color]}`}>{sub}</p>}
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { cls: string; label: string }> = {
    paid: { cls: "bg-green-100 text-green-800", label: "Pago" },
    awaiting_shipment: { cls: "bg-amber-100 text-amber-800", label: "Pronto p/ envio" },
    shipped: { cls: "bg-blue-100 text-blue-800", label: "Enviado" },
    pending_payment: { cls: "bg-yellow-100 text-yellow-800", label: "Aguardando pag." },
    failed: { cls: "bg-red-100 text-red-800", label: "Falhou" },
    canceled: { cls: "bg-neutral-200 text-neutral-700", label: "Cancelado" },
  };
  const m = map[status] ?? { cls: "bg-neutral-100 text-neutral-700", label: status };
  return (
    <span className={`rounded px-2 py-0.5 text-xs font-medium ${m.cls}`}>{m.label}</span>
  );
}

const PT_MONTHS = [
  "Jan", "Fev", "Mar", "Abr", "Mai", "Jun",
  "Jul", "Ago", "Set", "Out", "Nov", "Dez",
];

function monthLabel(ym: string): string {
  const [y, m] = ym.split("-");
  const idx = parseInt(m, 10) - 1;
  const shortYear = y.slice(2);
  return `${PT_MONTHS[idx] ?? m}/${shortYear}`;
}

function DailyRevenueChart({
  data,
  allTimeCents,
}: {
  data: { day: string; grossCents: number; orderCount: number }[];
  allTimeCents: number;
}) {
  const months = useMemo(() => {
    const seen = new Set<string>();
    for (const d of data) seen.add(d.day.slice(0, 7));
    return Array.from(seen).sort();
  }, [data]);

  const [selected, setSelected] = useState<string>("");

  useEffect(() => {
    if (months.length > 0 && !months.includes(selected)) {
      setSelected(months[months.length - 1] ?? "");
    }
  }, [months, selected]);

  const filtered = useMemo(
    () =>
      selected === "all"
        ? data
        : data.filter((d) => d.day.startsWith(selected)),
    [data, selected],
  );

  const max = Math.max(1, ...filtered.map((d) => d.grossCents));
  const periodTotal = filtered.reduce((acc, d) => acc + d.grossCents, 0);

  const step = Math.max(1, Math.floor(filtered.length / 5));

  return (
    <div>
      {/* Totals row */}
      <div className="mb-3 flex items-center justify-between text-xs">
        <span className="text-neutral-400">
          Total geral:{" "}
          <span className="font-semibold text-neutral-700">
            {moneyBR(allTimeCents)}
          </span>
        </span>
        <span className="text-neutral-400">
          Período:{" "}
          <span className="font-semibold text-neutral-700">
            {moneyBR(periodTotal)}
          </span>
        </span>
      </div>

      {/* Month tabs */}
      {months.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1">
          <button
            type="button"
            onClick={() => setSelected("all")}
            className={`rounded px-2 py-0.5 text-xs font-medium transition-colors ${
              selected === "all"
                ? "bg-black text-white"
                : "bg-neutral-100 text-neutral-500 hover:bg-neutral-200"
            }`}
          >
            Tudo
          </button>
          {months.map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setSelected(m)}
              className={`rounded px-2 py-0.5 text-xs font-medium capitalize transition-colors ${
                selected === m
                  ? "bg-black text-white"
                  : "bg-neutral-100 text-neutral-500 hover:bg-neutral-200"
              }`}
            >
              {monthLabel(m)}
            </button>
          ))}
        </div>
      )}

      {/* Bar chart */}
      {filtered.length === 0 ? (
        <p className="text-xs text-neutral-400">Sem dados neste período.</p>
      ) : (
        <>
          <div className="flex items-end gap-0.5" style={{ height: 112 }}>
            {filtered.map((d) => {
              const h = Math.max(4, Math.round((d.grossCents / max) * 100));
              return (
                <div
                  key={d.day}
                  className="group relative flex flex-1 flex-col items-center justify-end"
                  style={{ height: "100%" }}
                >
                  <div className="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1.5 hidden -translate-x-1/2 whitespace-nowrap rounded bg-neutral-900 px-2 py-1 text-[10px] text-white group-hover:block">
                    {d.day.slice(8)}/{d.day.slice(5, 7)}
                    <br />
                    {moneyBR(d.grossCents)}
                    <br />
                    {d.orderCount} pedido{d.orderCount !== 1 ? "s" : ""}
                  </div>
                  <div
                    className="w-full rounded-t-sm bg-black transition-all duration-150"
                    style={{ height: `${h}%` }}
                  />
                </div>
              );
            })}
          </div>
          <div className="mt-1 flex gap-0.5">
            {filtered.map((d, i) => (
              <div key={d.day} className="flex-1 text-center">
                {(i === 0 || i === filtered.length - 1 || i % step === 0) && (
                  <span className="text-[9px] text-neutral-400">
                    {d.day.slice(8)}
                  </span>
                )}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// Top-products horizontal bar chart — replaces the plain table.
function TopProductsChart({
  data,
}: {
  data: { productId: string; productName: string; quantity: number; grossCents: number }[];
}) {
  const max = Math.max(1, ...data.map((p) => p.grossCents));
  return (
    <div className="space-y-3">
      {data.map((p, i) => (
        <div key={p.productId}>
          <div className="mb-1 flex items-center justify-between gap-2">
            <span className="flex min-w-0 items-center gap-1.5 text-sm">
              <span className="shrink-0 text-xs font-bold text-neutral-300">
                #{i + 1}
              </span>
              <span className="truncate">{p.productName || p.productId}</span>
            </span>
            <span className="shrink-0 text-xs text-neutral-500">
              {moneyBR(p.grossCents)} · {p.quantity} un
            </span>
          </div>
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-neutral-100">
            <div
              className="h-1.5 rounded-full bg-black"
              style={{ width: `${Math.max(4, (p.grossCents / max) * 100)}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

// Recent-order row — avatar initials + name + status + amount.
function RecentOrderRow({
  order,
}: {
  order: {
    id: string;
    status: string;
    paymentMethod: string;
    amountCents: number;
    customerName: string;
    createdAt: string;
  };
}) {
  const initials = order.customerName
    .split(" ")
    .slice(0, 2)
    .map((s) => s[0] ?? "")
    .join("")
    .toUpperCase();
  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-neutral-100 text-xs font-bold text-neutral-600">
        {initials}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">{order.customerName}</span>
          <StatusBadge status={order.status} />
        </div>
        <div className="mt-0.5 flex items-center gap-1.5 text-xs text-neutral-400">
          <span>{order.paymentMethod === "pix" ? "PIX" : "Cartão"}</span>
          <span>·</span>
          <span>
            {new Date(order.createdAt).toLocaleDateString("pt-BR", {
              day: "2-digit",
              month: "2-digit",
            })}
          </span>
        </div>
      </div>
      <span className="shrink-0 text-sm font-semibold">
        {moneyBR(order.amountCents)}
      </span>
    </div>
  );
}

// Inline icon set — no external dep needed.
function IcoTrend() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="22 7 13.5 15.5 8.5 10.5 2 17" /><polyline points="16 7 22 7 22 13" />
    </svg>
  );
}
function IcoTag() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z" /><line x1="7" y1="7" x2="7.01" y2="7" />
    </svg>
  );
}
function IcoTruck() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="1" y="3" width="15" height="13" /><polygon points="16 8 20 8 23 11 23 16 16 16 16 8" /><circle cx="5.5" cy="18.5" r="2.5" /><circle cx="18.5" cy="18.5" r="2.5" />
    </svg>
  );
}
function IcoPercent() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <line x1="19" y1="5" x2="5" y2="19" /><circle cx="6.5" cy="6.5" r="2.5" /><circle cx="17.5" cy="17.5" r="2.5" />
    </svg>
  );
}
function IcoBag() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 2L3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4z" /><line x1="3" y1="6" x2="21" y2="6" /><path d="M16 10a4 4 0 0 1-8 0" />
    </svg>
  );
}
function IcoCheck() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}
function IcoClock() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10" /><polyline points="12 6 12 12 16 14" />
    </svg>
  );
}
function IcoXmark() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

// ----- Orders tab -----

function formatCpf(doc: string | undefined): string {
  const d = (doc ?? "").replace(/\D/g, "");
  if (d.length === 11) return `${d.slice(0, 3)}.${d.slice(3, 6)}.${d.slice(6, 9)}-${d.slice(9)}`;
  if (d.length === 14) return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8, 12)}-${d.slice(12)}`;
  return doc ?? "";
}

function formatFullAddress(o: AdminOrder): string {
  const line1 = [o.address, o.addressNumber].filter(Boolean).join(", ");
  const line1b = o.addressComplement ? `${line1} - ${o.addressComplement}` : line1;
  const line2 = [o.district, o.city && o.state ? `${o.city}/${o.state}` : o.city]
    .filter(Boolean)
    .join(" · ");
  const line3 = o.zip ? `CEP ${o.zip}` : "";
  return [line1b, line2, line3].filter(Boolean).join("\n");
}

function OrdersAdmin({ token }: { token: string }) {
  const [orders, setOrders] = useState<AdminOrder[] | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [err, setErr] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setErr("");
    try {
      const res = await adminOrdersApi.list(token, {
        status: statusFilter || undefined,
        limit: 100,
      });
      setOrders(res.orders);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, [token, statusFilter]);

  useEffect(() => {
    // load() only touches state after the fetch resolves.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);

  const selected = useMemo(
    () => orders?.find((o) => o.orderId === selectedId) ?? null,
    [orders, selectedId],
  );

  async function handleDelete(id: string) {
    if (!window.confirm(`Apagar pedido ${id}? Esta ação é permanente.`)) return;
    setBusyId(id);
    try {
      await adminOrdersApi.remove(token, id);
      setSelectedId(null);
      await load();
    } catch (e) {
      alert("Falha ao apagar: " + (e instanceof Error ? e.message : String(e)));
    } finally {
      setBusyId(null);
    }
  }

  async function handleRetry(id: string) {
    setBusyId(id);
    try {
      const res = await adminOrdersApi.retryLabel(token, id);
      const suffix =
        res.status === "awaiting_shipment"
          ? " — pague a etiqueta no app do SuperFrete"
          : res.trackingCode
          ? ` · tracking=${res.trackingCode}`
          : "";
      alert(`Status: ${res.status}${suffix}`);
      await load();
    } catch (e) {
      alert("Falhou: " + (e instanceof Error ? e.message : String(e)));
    } finally {
      setBusyId(null);
    }
  }

  async function handleRefreshTracking(id: string) {
    setBusyId(id);
    try {
      const res = await adminOrdersApi.refreshTracking(token, id);
      if (res.updated) {
        alert(`Rastreio encontrado: ${res.trackingCode}\nCliente notificado por email.`);
      } else {
        alert(
          res.hint ||
            "SuperFrete ainda não emitiu rastreio — confira se a etiqueta já foi paga no app.",
        );
      }
      await load();
    } catch (e) {
      alert("Falha ao atualizar rastreio: " + (e instanceof Error ? e.message : String(e)));
    } finally {
      setBusyId(null);
    }
  }

  async function handleAddItem(
    id: string,
    payload: { productId: string; size: string; quantity: number; mode: "gift" | "extra" },
  ) {
    setBusyId(id);
    try {
      await adminOrdersApi.addItem(token, id, payload);
      await load();
    } catch (e) {
      alert("Falha ao adicionar item: " + (e instanceof Error ? e.message : String(e)));
    } finally {
      setBusyId(null);
    }
  }

  async function handleMarkPaid(id: string) {
    if (
      !window.confirm(
        "Confirmar que o cliente pagou esse pedido por Pix? O estoque será baixado e o cliente vai receber o email de confirmação.",
      )
    ) {
      return;
    }
    setBusyId(id);
    try {
      await adminOrdersApi.markPaid(token, id);
      alert("Pedido marcado como pago. Agora você consegue colar o código de rastreio.");
      await load();
    } catch (e) {
      alert("Falha: " + (e instanceof Error ? e.message : String(e)));
    } finally {
      setBusyId(null);
    }
  }

  async function handleMarkShipped(id: string, trackingCode: string) {
    const code = trackingCode.trim();
    if (!code) {
      alert("Digite o código de rastreio dos Correios antes.");
      return;
    }
    setBusyId(id);
    try {
      await adminOrdersApi.markShipped(token, id, { trackingCode: code });
      alert("Pedido marcado como enviado. Cliente notificado por email.");
      await load();
    } catch (e) {
      alert("Falha: " + (e instanceof Error ? e.message : String(e)));
    } finally {
      setBusyId(null);
    }
  }

  return (
    <section className="space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Pedidos</h2>
        <div className="flex items-center gap-2">
          <label className="text-xs uppercase tracking-wide text-neutral-500">
            status
          </label>
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="rounded border border-neutral-300 bg-white px-2 py-1 text-sm"
          >
            <option value="">(todos)</option>
            <option value="pending_payment">pending_payment</option>
            <option value="paid">paid</option>
            <option value="awaiting_shipment">awaiting_shipment</option>
            <option value="shipped">shipped</option>
            <option value="failed">failed</option>
            <option value="canceled">canceled</option>
          </select>
          <button
            type="button"
            onClick={() => void load()}
            className="rounded border border-neutral-300 px-3 py-1 text-sm"
          >
            Recarregar
          </button>
        </div>
      </header>

      {err && <p className="text-sm text-red-600">{err}</p>}
      {!orders && !err && <p className="text-sm text-neutral-500">Carregando…</p>}
      {orders && orders.length === 0 && (
        <p className="text-sm text-neutral-500">Nenhum pedido encontrado.</p>
      )}

      {orders && orders.length > 0 && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
          <div className="overflow-x-auto rounded border border-neutral-200">
            <table className="min-w-full text-sm">
              <thead className="bg-neutral-50 text-left text-xs uppercase text-neutral-500">
                <tr>
                  <th className="px-3 py-2">Pedido</th>
                  <th className="px-3 py-2">Cliente</th>
                  <th className="px-3 py-2">Status</th>
                  <th className="px-3 py-2 text-right">Total</th>
                </tr>
              </thead>
              <tbody>
                {orders.map((o) => {
                  const active = o.orderId === selectedId;
                  return (
                    <tr
                      key={o.orderId}
                      onClick={() => setSelectedId(o.orderId)}
                      className={`cursor-pointer border-t border-neutral-100 ${
                        active ? "bg-neutral-100" : "hover:bg-neutral-50"
                      }`}
                    >
                      <td className="px-3 py-2 font-mono text-xs">
                        {o.orderId}
                      </td>
                      <td className="px-3 py-2">
                        <div className="font-medium">{o.name}</div>
                        <div className="text-xs text-neutral-500">{o.email}</div>
                      </td>
                      <td className="px-3 py-2">
                        <StatusBadge status={o.status} />
                      </td>
                      <td className="px-3 py-2 text-right font-semibold">
                        {moneyBR(o.amountCents)}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          <aside className="rounded border border-neutral-200 p-4">
            {!selected && (
              <p className="text-sm text-neutral-500">
                Selecione um pedido à esquerda para ver os detalhes.
              </p>
            )}
            {selected && <OrderDetail
              order={selected}
              token={token}
              busy={busyId === selected.orderId}
              onRetry={() => void handleRetry(selected.orderId)}
              onDelete={() => void handleDelete(selected.orderId)}
              onRefreshTracking={() => void handleRefreshTracking(selected.orderId)}
              onMarkShipped={(code) => void handleMarkShipped(selected.orderId, code)}
              onMarkPaid={() => void handleMarkPaid(selected.orderId)}
              onAddItem={(payload) => void handleAddItem(selected.orderId, payload)}
            />}
          </aside>
        </div>
      )}
    </section>
  );
}

function OrderDetail({
  order,
  token,
  busy,
  onRetry,
  onDelete,
  onRefreshTracking,
  onMarkShipped,
  onMarkPaid,
  onAddItem,
}: {
  order: AdminOrder;
  token: string;
  busy: boolean;
  onRetry: () => void;
  onDelete: () => void;
  onRefreshTracking: () => void;
  onMarkShipped: (trackingCode: string) => void;
  onMarkPaid: () => void;
  onAddItem: (payload: { productId: string; size: string; quantity: number; mode: "gift" | "extra" }) => void;
}) {
  const canRetryLabel = order.status === "paid" && !order.trackingCode;
  const isAwaitingShipment = order.status === "awaiting_shipment";
  // Pix recebido por fora (WhatsApp) — operador confirma manualmente.
  // Cartão é controlado pelo Stripe webhook, então não aparece esse botão.
  const canConfirmPix =
    order.status === "pending_payment" &&
    order.paymentMethod.toLowerCase() === "pix";
  // Adicionar item: liberado em qualquer status que não seja final.
  // Depois de shipped/canceled/failed o pacote físico já foi cortado e
  // editar a lista só causaria divergência com a etiqueta.
  const canAddItem =
    order.status === "pending_payment" ||
    order.status === "paid" ||
    order.status === "awaiting_shipment";
  // Always offer the manual tracking input on any paid-but-unshipped
  // order, even when SuperFrete hasn't (or couldn't) put a row in the
  // cart yet. Lets the operator skip the SuperFrete flow entirely —
  // useful when the label was generated outside the system or when
  // /api/v0/cart fails for reasons we can't auto-recover from.
  const canManualShip =
    !order.trackingCode &&
    (order.status === "paid" || order.status === "awaiting_shipment");
  const [manualTracking, setManualTracking] = useState("");
  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h3 className="font-mono text-sm text-neutral-500">{order.orderId}</h3>
          <h2 className="text-lg font-semibold">{order.name}</h2>
          <p className="text-sm text-neutral-600">{order.email}</p>
        </div>
        <StatusBadge status={order.status} />
      </header>

      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <dt className="text-xs uppercase text-neutral-500">CPF</dt>
        <dd className="font-mono">{order.document ? formatCpf(order.document) : "—"}</dd>

        <dt className="text-xs uppercase text-neutral-500">Pagamento</dt>
        <dd className="capitalize">{order.paymentMethod}</dd>

        <dt className="text-xs uppercase text-neutral-500">Total</dt>
        <dd className="font-semibold">{moneyBR(order.amountCents)}</dd>

        <dt className="text-xs uppercase text-neutral-500">Frete</dt>
        <dd>
          {moneyBR(order.shippingCents)}
          {order.shippingServiceName ? ` · ${order.shippingServiceName}` : ""}
        </dd>

        {order.couponCode && (
          <>
            <dt className="text-xs uppercase text-neutral-500">Cupom</dt>
            <dd>
              {order.couponCode}
              {order.discountCents ? ` (-${moneyBR(order.discountCents)})` : ""}
            </dd>
          </>
        )}

        {order.trackingCode && (
          <>
            <dt className="text-xs uppercase text-neutral-500">Rastreio</dt>
            <dd>
              <span className="font-mono">{order.trackingCode}</span>
              {order.trackingUrl && (
                <>
                  {" "}
                  <a
                    href={order.trackingUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="text-blue-700 underline"
                  >
                    rastrear
                  </a>
                </>
              )}
              {order.labelUrl && (
                <>
                  {" · "}
                  <a
                    href={order.labelUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="text-blue-700 underline"
                  >
                    etiqueta
                  </a>
                </>
              )}
            </dd>
          </>
        )}

        {order.trackingLastError && (
          <>
            <dt className="text-xs uppercase text-red-600">Erro da etiqueta</dt>
            <dd className="text-red-700">{order.trackingLastError}</dd>
          </>
        )}
      </dl>

      <div>
        <p className="text-xs uppercase text-neutral-500">Endereço</p>
        <pre className="whitespace-pre-wrap rounded bg-neutral-50 p-2 text-sm">
          {formatFullAddress(order)}
        </pre>
      </div>

      <div>
        <p className="mb-1 text-xs uppercase text-neutral-500">Itens</p>
        <ul className="divide-y divide-neutral-200 rounded border border-neutral-200">
          {order.items.length === 0 && (
            <li className="px-3 py-2 text-sm text-neutral-500">
              Sem itens registrados.
            </li>
          )}
          {order.items.map((it, i) => (
            <li key={i} className="flex items-center justify-between px-3 py-2 text-sm">
              <div>
                <div className="font-medium">{it.productName}</div>
                <div className="text-xs text-neutral-500">
                  {[it.size && `tam ${it.size}`, it.color].filter(Boolean).join(" · ")}
                </div>
              </div>
              <div className="text-right">
                <div>{it.quantity}×</div>
                <div className="text-xs text-neutral-500">
                  {moneyBR(it.unitPriceCents)}
                </div>
              </div>
            </li>
          ))}
        </ul>
      </div>

      {canAddItem && <AddOrderItemPanel token={token} order={order} busy={busy} onAddItem={onAddItem} />}

      {canConfirmPix && (
        <div className="space-y-2 rounded border border-emerald-300 bg-emerald-50 p-3">
          <h4 className="text-sm font-semibold text-emerald-900">
            Confirmar pagamento Pix
          </h4>
          <p className="text-xs text-emerald-800">
            O cliente foi pro WhatsApp pra finalizar o Pix. Quando você
            confirmar o recebimento na sua conta, clique no botão pra marcar
            o pedido como pago — isso baixa estoque, libera o campo de
            rastreio e dispara o email de confirmação.
          </p>
          <button
            type="button"
            disabled={busy}
            onClick={onMarkPaid}
            className="rounded bg-emerald-700 px-3 py-1.5 text-sm text-white disabled:opacity-50"
          >
            {busy ? "Confirmando…" : "Marcar como pago"}
          </button>
        </div>
      )}

      {canManualShip && (
        <div className="space-y-3 rounded border border-amber-300 bg-amber-50 p-3">
          <div>
            <h4 className="text-sm font-semibold text-amber-900">
              {isAwaitingShipment
                ? "Etiqueta no carrinho do SuperFrete"
                : "Marcar como enviado"}
            </h4>
            {isAwaitingShipment ? (
              <p className="mt-1 text-xs text-amber-800">
                O pedido já está na sua conta do SuperFrete como{" "}
                <strong>aguardando pagamento</strong>.{" "}
                <a
                  href="https://web.superfrete.com/#/cart"
                  target="_blank"
                  rel="noreferrer"
                  className="underline"
                >
                  abrir carrinho
                </a>
                {" · "}
                pague pelo app (Pix costuma ser mais barato), imprima e posta.
                Depois volte aqui para:
              </p>
            ) : (
              <p className="mt-1 text-xs text-amber-800">
                Pagamento confirmado. Você pode tentar gerar a etiqueta pelo
                SuperFrete (botão "Gerar etiqueta" abaixo) ou{" "}
                <strong>colar manualmente</strong> o código de rastreio dos
                Correios — útil quando você imprimiu a etiqueta por fora.
              </p>
            )}
            {order.superfreteId && (
              <p className="mt-1 font-mono text-xs text-amber-700">
                ID SuperFrete: {order.superfreteId}
              </p>
            )}
          </div>
          {isAwaitingShipment && (
            <div className="flex flex-wrap items-center gap-2">
              <button
                type="button"
                disabled={busy}
                onClick={onRefreshTracking}
                className="rounded border border-amber-700 bg-white px-3 py-1.5 text-sm text-amber-900 disabled:opacity-50"
              >
                {busy ? "Consultando…" : "Atualizar rastreio do SuperFrete"}
              </button>
            </div>
          )}
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="text"
              value={manualTracking}
              onChange={(e) => setManualTracking(e.target.value)}
              placeholder="Código de rastreio (ex: BR123456789BR)"
              className="flex-1 min-w-[16rem] rounded border border-amber-300 bg-white px-2 py-1 text-sm font-mono"
            />
            <button
              type="button"
              disabled={busy || !manualTracking.trim()}
              onClick={() => onMarkShipped(manualTracking)}
              className="rounded bg-amber-700 px-3 py-1.5 text-sm text-white disabled:opacity-50"
            >
              {busy ? "Salvando…" : "Marcar como enviado"}
            </button>
          </div>
          <p className="text-xs text-amber-800">
            Marcar como enviado dispara o email “Seu pedido está a caminho” para
            o cliente com o rastreio.
          </p>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        {canRetryLabel && (
          <button
            type="button"
            disabled={busy}
            onClick={onRetry}
            className="rounded bg-black px-3 py-1.5 text-sm text-white disabled:opacity-50"
          >
            {busy ? "Gerando…" : "Gerar etiqueta"}
          </button>
        )}
        <button
          type="button"
          disabled={busy}
          onClick={onDelete}
          className="rounded border border-red-600 px-3 py-1.5 text-sm text-red-700 disabled:opacity-50"
        >
          {busy ? "Apagando…" : "Apagar pedido"}
        </button>
      </div>
    </div>
  );
}

// AddOrderItemPanel renders the "adicionar peça ao pedido" surface in
// the order detail aside. The operator picks a catalog product, size,
// quantity, and decides on the spot whether the item is a courtesy
// (gift, totals untouched) or an upsell (extra, cart total bumps).
//
// Products are fetched lazily on first expand to avoid an extra API
// call for orders the operator only views.
function AddOrderItemPanel({
  token,
  order,
  busy,
  onAddItem,
}: {
  token: string;
  order: AdminOrder;
  busy: boolean;
  onAddItem: (payload: {
    productId: string;
    size: string;
    quantity: number;
    mode: "gift" | "extra";
  }) => void;
}) {
  const [open, setOpen] = useState(false);
  const [products, setProducts] = useState<Product[] | null>(null);
  const [loadErr, setLoadErr] = useState("");
  const [productId, setProductId] = useState("");
  const [size, setSize] = useState("");
  const [quantity, setQuantity] = useState(1);
  const [mode, setMode] = useState<"gift" | "extra">("gift");

  useEffect(() => {
    if (!open || products !== null) return;
    let cancelled = false;
    void adminApi
      .list(token)
      .then((list) => {
        if (cancelled) return;
        setProducts(list);
        if (list.length > 0) {
          setProductId(list[0].id);
          setSize(list[0].sizes?.[0] ?? "");
        }
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        setLoadErr(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [open, products, token]);

  const selected = products?.find((p) => p.id === productId);
  const sizesAvailable = selected?.sizes ?? [];
  const isPix = order.paymentMethod.toLowerCase() === "pix";
  const previewUnit = selected
    ? mode === "gift"
      ? 0
      : isPix && selected.pixPriceCents
      ? selected.pixPriceCents
      : selected.priceCents
    : 0;
  const previewDelta = previewUnit * quantity;

  function submit() {
    if (!productId) {
      alert("Escolha um produto.");
      return;
    }
    if (sizesAvailable.length > 0 && !size) {
      alert("Escolha um tamanho.");
      return;
    }
    const verb = mode === "gift" ? "como brinde (sem cobrar)" : `cobrando ${moneyBR(previewDelta)} a mais`;
    if (
      !window.confirm(
        `Adicionar ${quantity}× ${selected?.name ?? productId} (${size || "—"}) ${verb}?`,
      )
    ) {
      return;
    }
    onAddItem({ productId, size, quantity, mode });
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="w-full rounded border border-dashed border-neutral-400 px-3 py-2 text-sm text-neutral-600 hover:border-neutral-700 hover:text-neutral-900"
      >
        + Adicionar item ao pedido
      </button>
    );
  }

  return (
    <div className="space-y-3 rounded border border-blue-300 bg-blue-50 p-3">
      <div className="flex items-start justify-between gap-2">
        <h4 className="text-sm font-semibold text-blue-900">Adicionar item</h4>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="text-xs text-blue-700 hover:underline"
        >
          fechar
        </button>
      </div>
      {loadErr && <p className="text-sm text-red-600">{loadErr}</p>}
      {!products && !loadErr && <p className="text-sm text-blue-800">Carregando produtos…</p>}
      {products && (
        <div className="space-y-2">
          <label className="block text-xs uppercase text-blue-900">
            Produto
            <select
              value={productId}
              onChange={(e) => {
                setProductId(e.target.value);
                const p = products.find((it) => it.id === e.target.value);
                setSize(p?.sizes?.[0] ?? "");
              }}
              className="mt-1 block w-full rounded border border-blue-300 bg-white px-2 py-1 text-sm normal-case text-neutral-900"
            >
              {products.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          {sizesAvailable.length > 0 && (
            <label className="block text-xs uppercase text-blue-900">
              Tamanho
              <select
                value={size}
                onChange={(e) => setSize(e.target.value)}
                className="mt-1 block w-full rounded border border-blue-300 bg-white px-2 py-1 text-sm normal-case text-neutral-900"
              >
                {sizesAvailable.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="block text-xs uppercase text-blue-900">
            Quantidade
            <input
              type="number"
              min={1}
              max={20}
              value={quantity}
              onChange={(e) => setQuantity(Math.max(1, parseInt(e.target.value, 10) || 1))}
              className="mt-1 block w-24 rounded border border-blue-300 bg-white px-2 py-1 text-sm text-neutral-900"
            />
          </label>
          <fieldset className="rounded border border-blue-200 bg-white px-3 py-2">
            <legend className="px-1 text-xs uppercase text-blue-900">Modo</legend>
            <label className="flex cursor-pointer items-start gap-2 py-1 text-sm text-neutral-800">
              <input
                type="radio"
                name={`add-mode-${order.orderId}`}
                checked={mode === "gift"}
                onChange={() => setMode("gift")}
              />
              <span>
                <strong>Brinde</strong> — total não muda. Estoque baixa normal.
              </span>
            </label>
            <label className="flex cursor-pointer items-start gap-2 py-1 text-sm text-neutral-800">
              <input
                type="radio"
                name={`add-mode-${order.orderId}`}
                checked={mode === "extra"}
                onChange={() => setMode("extra")}
              />
              <span>
                <strong>Cobrar extra</strong> — soma{" "}
                {selected ? moneyBR(previewDelta) : "—"} no total. Você cobra a
                diferença por fora (Pix manual).
              </span>
            </label>
          </fieldset>
          {order.status === "awaiting_shipment" && (
            <p className="rounded bg-amber-100 px-2 py-1 text-xs text-amber-900">
              Aviso: esse pedido já está no carrinho do SuperFrete. Se a
              etiqueta foi paga/impressa, ela não vai incluir o item novo —
              refaça pelo botão "Gerar etiqueta" antes de despachar.
            </p>
          )}
          <button
            type="button"
            disabled={busy || !productId}
            onClick={submit}
            className="rounded bg-blue-700 px-3 py-1.5 text-sm text-white disabled:opacity-50"
          >
            {busy ? "Adicionando…" : "Adicionar item"}
          </button>
        </div>
      )}
    </div>
  );
}

// ----- Products tab -----

function ProductsAdmin({ token }: { token: string }) {
  const [items, setItems] = useState<Product[] | null>(null);
  const [err, setErr] = useState<string>("");
  const [editing, setEditing] = useState<AdminProductPayload | null>(null);
  const [saving, setSaving] = useState(false);

  const refresh = useCallback(async () => {
    setErr("");
    try {
      const list = await adminApi.list(token);
      setItems(list);
    } catch (e) {
      setErr((e as Error).message);
      setItems(null);
    }
  }, [token]);

  useEffect(() => {
    // refresh() only touches state after the fetch resolves.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
  }, [refresh]);

  const save = async () => {
    if (!editing) return;
    setSaving(true);
    setErr("");
    try {
      if (items?.some((p) => p.id === editing.id)) {
        await adminApi.update(token, editing.id, editing);
      } else {
        await adminApi.create(token, editing);
      }
      setEditing(null);
      await refresh();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const remove = async (id: string) => {
    if (!window.confirm(`Excluir ${id}?`)) return;
    setErr("");
    try {
      await adminApi.remove(token, id);
      await refresh();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  return (
    <section>
      <div className="mb-4 flex justify-end">
        <button
          type="button"
          onClick={() => setEditing(emptyProduct())}
          className="rounded bg-black px-4 py-2 text-sm text-white"
        >
          Novo produto
        </button>
      </div>

      {err && (
        <div className="mb-4 rounded border border-red-400 bg-red-50 px-4 py-3 text-sm text-red-800">
          {err}
        </div>
      )}

      {!items && !err && <p>Carregando…</p>}
      {items && items.length === 0 && (
        <p className="text-sm text-neutral-600">Nenhum produto cadastrado.</p>
      )}

      {items && items.length > 0 && (
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b text-left">
              <th className="py-2 pr-3">ID</th>
              <th className="py-2 pr-3">Nome</th>
              <th className="py-2 pr-3">Categoria</th>
              <th className="py-2 pr-3">Preço</th>
              <th className="py-2 pr-3">Pix</th>
              <th className="py-2 pr-3">Estoque</th>
              <th className="py-2">Ações</th>
            </tr>
          </thead>
          <tbody>
            {items
              .filter((p, i, arr) => arr.findIndex((x) => x.id === p.id) === i)
              .map((p) => (
              <tr key={p.id} className="border-b">
                <td className="py-2 pr-3 font-mono text-xs">{p.id}</td>
                <td className="py-2 pr-3">{p.name}</td>
                <td className="py-2 pr-3">{p.category}</td>
                <td className="py-2 pr-3">{moneyBR(p.priceCents)}</td>
                <td className="py-2 pr-3">{moneyBR(p.pixPriceCents)}</td>
                <td className="py-2 pr-3">{p.stock}</td>
                <td className="py-2">
                  <button
                    type="button"
                    onClick={() => setEditing({ ...p })}
                    className="mr-3 underline"
                  >
                    editar
                  </button>
                  <button
                    type="button"
                    onClick={() => void remove(p.id)}
                    className="text-red-600 underline"
                  >
                    excluir
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {editing && (
        <ProductEditForm
          value={editing}
          onChange={setEditing}
          onSave={save}
          onCancel={() => setEditing(null)}
          saving={saving}
          token={token}
        />
      )}
    </section>
  );
}

type ProductEditProps = {
  value: AdminProductPayload;
  onChange: (v: AdminProductPayload) => void;
  onSave: () => void;
  onCancel: () => void;
  saving: boolean;
  token: string;
};

function ProductEditForm({
  value,
  onChange,
  onSave,
  onCancel,
  saving,
  token,
}: ProductEditProps) {
  const set = <K extends keyof AdminProductPayload>(
    key: K,
    v: AdminProductPayload[K],
  ) => onChange({ ...value, [key]: v });

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 p-4">
      <div className="max-h-[90vh] w-full max-w-2xl overflow-auto rounded bg-white p-6 text-neutral-900 shadow-lg">
        <h2 className="mb-4 text-xl font-semibold text-neutral-900">
          {value.id ? `Editar ${value.id}` : "Novo produto"}
        </h2>
        <div className="grid grid-cols-2 gap-3 text-sm">
          <label className="flex flex-col">
            <span>ID</span>
            <input
              value={value.id}
              onChange={(e) => set("id", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Nome</span>
            <input
              value={value.name}
              onChange={(e) => set("name", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="col-span-2 flex flex-col">
            <span>Descrição</span>
            <textarea
              rows={3}
              value={value.description}
              onChange={(e) => set("description", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Preço (centavos)</span>
            <input
              type="number"
              value={value.priceCents}
              onChange={(e) => set("priceCents", Number(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Preço Pix (centavos)</span>
            <input
              type="number"
              value={value.pixPriceCents}
              onChange={(e) => set("pixPriceCents", Number(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Categoria</span>
            <input
              value={value.category}
              onChange={(e) => set("category", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Estoque total</span>
            <input
              type="number"
              value={value.stock}
              readOnly
              title="Calculado a partir do estoque por tamanho"
              className="rounded border border-neutral-200 bg-neutral-100 px-2 py-1 text-neutral-700"
            />
          </label>
          <ImageUploadField
            label="Imagem (frente)"
            value={value.image}
            onChange={(v) => set("image", v)}
            token={token}
          />
          <ImageUploadField
            label="Imagem (verso)"
            value={value.backImage}
            onChange={(v) => set("backImage", v)}
            token={token}
          />

          <label className="flex flex-col">
            <span>Cores (csv)</span>
            <CsvInput
              value={value.colors}
              onChange={(colors) => set("colors", colors)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Tamanhos (csv)</span>
            <CsvInput
              value={value.sizes}
              onChange={(sizes) => {
                // Keep stockBySize aligned with the size list: drop
                // entries for sizes that were removed and seed 0 for
                // newly added ones. Recompute the total in lockstep.
                const next: Record<string, number> = {};
                for (const s of sizes) {
                  next[s] = value.stockBySize?.[s] ?? 0;
                }
                onChange({
                  ...value,
                  sizes,
                  stockBySize: next,
                  stock: sumStockBySize(next),
                });
              }}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          {value.sizes.length > 0 && (
            <div className="col-span-2 flex flex-col gap-2">
              <span>Estoque por tamanho</span>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                {value.sizes.map((s) => (
                  <label
                    key={s}
                    className="flex items-center gap-2 rounded border border-neutral-200 bg-neutral-50 px-2 py-1"
                  >
                    <span className="min-w-[3.5rem] text-xs font-semibold uppercase tracking-wider text-neutral-600">
                      {s}
                    </span>
                    <input
                      type="number"
                      min={0}
                      step={1}
                      value={value.stockBySize?.[s] ?? 0}
                      onChange={(e) => {
                        const qty = Math.max(0, Number(e.target.value) || 0);
                        const next = { ...(value.stockBySize ?? {}), [s]: qty };
                        onChange({
                          ...value,
                          stockBySize: next,
                          stock: sumStockBySize(next),
                        });
                      }}
                      className="w-full rounded border border-neutral-300 px-2 py-1 text-sm"
                    />
                  </label>
                ))}
              </div>
              <span className="text-xs text-neutral-500">
                Tamanho com 0 é exibido como esgotado e não entra no
                carrinho.
              </span>
            </div>
          )}
          <label className="flex flex-col">
            <span>Tags (csv)</span>
            <CsvInput
              value={value.tags}
              onChange={(tags) => set("tags", tags)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="col-span-2 flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={value.transparentImage ?? false}
              onChange={(e) => set("transparentImage", e.target.checked)}
            />
            <span>
              Imagem sem fundo (PNG transparente)
              <span className="ml-1 text-xs text-neutral-500">
                — desliga o fundo branco do catálogo/modal pra esse produto
              </span>
            </span>
          </label>
          <label className="col-span-2 flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={value.hidden ?? false}
              onChange={(e) => set("hidden", e.target.checked)}
            />
            <span>Oculto no catálogo público</span>
          </label>
        </div>
        <div className="mt-6 flex justify-end gap-3">
          <button
            type="button"
            onClick={onCancel}
            className="rounded border border-neutral-300 px-4 py-2 text-sm"
            disabled={saving}
          >
            Cancelar
          </button>
          <button
            type="button"
            onClick={onSave}
            className="rounded bg-black px-4 py-2 text-sm text-white disabled:opacity-50"
            disabled={saving || !value.id || !value.name}
          >
            {saving ? "Salvando…" : "Salvar"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ----- Coupons tab -----

function CouponsAdmin({ token }: { token: string }) {
  const [items, setItems] = useState<Coupon[] | null>(null);
  const [err, setErr] = useState<string>("");
  const [editing, setEditing] = useState<AdminCouponPayload | null>(null);
  const [originalCode, setOriginalCode] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const refresh = useCallback(async () => {
    setErr("");
    try {
      const list = await adminCouponsApi.list(token);
      setItems(list);
    } catch (e) {
      setErr((e as Error).message);
      setItems(null);
    }
  }, [token]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
  }, [refresh]);

  const save = async () => {
    if (!editing) return;
    setSaving(true);
    setErr("");
    try {
      if (originalCode) {
        await adminCouponsApi.update(token, originalCode, editing);
      } else {
        await adminCouponsApi.create(token, editing);
      }
      setEditing(null);
      setOriginalCode(null);
      await refresh();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const remove = async (code: string) => {
    if (!window.confirm(`Excluir cupom ${code}?`)) return;
    setErr("");
    try {
      await adminCouponsApi.remove(token, code);
      await refresh();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  const describeKind = (c: Coupon): string => {
    switch (c.kind) {
      case "percent":
        return `-${c.value}%`;
      case "amount":
        return `-${moneyBR(c.value)}`;
      case "free_shipping":
        return "frete grátis";
    }
  };

  const startEdit = (c: Coupon) => {
    setOriginalCode(c.code);
    setEditing({
      code: c.code,
      kind: c.kind,
      value: c.value,
      minSubtotalCents: c.minSubtotalCents,
      maxUses: c.maxUses,
      startsAt: c.startsAt ?? null,
      expiresAt: c.expiresAt ?? null,
      active: c.active,
      note: c.note ?? "",
    });
  };

  return (
    <section>
      <div className="mb-4 flex justify-end">
        <button
          type="button"
          onClick={() => {
            setOriginalCode(null);
            setEditing(emptyCoupon());
          }}
          className="rounded bg-black px-4 py-2 text-sm text-white"
        >
          Novo cupom
        </button>
      </div>

      {err && (
        <div className="mb-4 rounded border border-red-400 bg-red-50 px-4 py-3 text-sm text-red-800">
          {err}
        </div>
      )}

      {!items && !err && <p>Carregando…</p>}
      {items && items.length === 0 && (
        <p className="text-sm text-neutral-600">
          Nenhum cupom cadastrado. Crie o primeiro acima.
        </p>
      )}

      {items && items.length > 0 && (
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b text-left">
              <th className="py-2 pr-3">Código</th>
              <th className="py-2 pr-3">Desconto</th>
              <th className="py-2 pr-3">Mín. carrinho</th>
              <th className="py-2 pr-3">Usos</th>
              <th className="py-2 pr-3">Expira</th>
              <th className="py-2 pr-3">Ativo</th>
              <th className="py-2">Ações</th>
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.code} className="border-b">
                <td className="py-2 pr-3 font-mono text-xs">{c.code}</td>
                <td className="py-2 pr-3">{describeKind(c)}</td>
                <td className="py-2 pr-3">
                  {c.minSubtotalCents > 0 ? moneyBR(c.minSubtotalCents) : "—"}
                </td>
                <td className="py-2 pr-3">
                  {c.usedCount}
                  {c.maxUses > 0 ? ` / ${c.maxUses}` : ""}
                </td>
                <td className="py-2 pr-3">
                  {c.expiresAt
                    ? new Date(c.expiresAt).toLocaleDateString("pt-BR")
                    : "—"}
                </td>
                <td className="py-2 pr-3">{c.active ? "sim" : "não"}</td>
                <td className="py-2">
                  <button
                    type="button"
                    onClick={() => startEdit(c)}
                    className="mr-3 underline"
                  >
                    editar
                  </button>
                  <button
                    type="button"
                    onClick={() => void remove(c.code)}
                    className="text-red-600 underline"
                  >
                    excluir
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {editing && (
        <CouponEditForm
          value={editing}
          onChange={setEditing}
          onSave={save}
          onCancel={() => {
            setEditing(null);
            setOriginalCode(null);
          }}
          saving={saving}
          renaming={originalCode !== null}
        />
      )}
    </section>
  );
}

type CouponEditProps = {
  value: AdminCouponPayload;
  onChange: (v: AdminCouponPayload) => void;
  onSave: () => void;
  onCancel: () => void;
  saving: boolean;
  renaming: boolean;
};

// toDateInputValue converts an RFC3339 string (or null) to the
// YYYY-MM-DD shape <input type="date"> expects.
function toDateInputValue(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString().slice(0, 10);
}

function fromDateInputValue(value: string): string | null {
  if (!value) return null;
  // Store as UTC midnight to match the backend's TIMESTAMPTZ handling.
  return new Date(`${value}T00:00:00Z`).toISOString();
}

function CouponEditForm({
  value,
  onChange,
  onSave,
  onCancel,
  saving,
  renaming,
}: CouponEditProps) {
  const set = <K extends keyof AdminCouponPayload>(
    key: K,
    v: AdminCouponPayload[K],
  ) => onChange({ ...value, [key]: v });

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 p-4">
      <div className="max-h-[90vh] w-full max-w-xl overflow-auto rounded bg-white p-6 text-neutral-900 shadow-lg">
        <h2 className="mb-4 text-xl font-semibold text-neutral-900">
          {renaming ? `Editar ${value.code}` : "Novo cupom"}
        </h2>
        <div className="grid grid-cols-2 gap-3 text-sm">
          <label className="col-span-2 flex flex-col">
            <span>Código</span>
            <input
              value={value.code}
              onChange={(e) =>
                set("code", e.target.value.toUpperCase().replace(/\s+/g, ""))
              }
              placeholder="ex: NAST10"
              className="rounded border border-neutral-300 px-2 py-1 font-mono uppercase"
              disabled={renaming}
            />
          </label>
          <label className="flex flex-col">
            <span>Tipo</span>
            <select
              value={value.kind}
              onChange={(e) =>
                set("kind", e.target.value as AdminCouponPayload["kind"])
              }
              className="rounded border border-neutral-300 px-2 py-1"
            >
              <option value="percent">Porcentagem (%)</option>
              <option value="amount">Valor fixo (R$)</option>
              <option value="free_shipping">Frete grátis</option>
            </select>
          </label>
          <label className="flex flex-col">
            <span>
              {value.kind === "percent"
                ? "% desconto (1–100)"
                : value.kind === "amount"
                  ? "Valor em centavos"
                  : "Valor (não se aplica)"}
            </span>
            <input
              type="number"
              min={0}
              max={value.kind === "percent" ? 100 : undefined}
              value={value.value}
              disabled={value.kind === "free_shipping"}
              onChange={(e) => set("value", Number(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1 disabled:bg-neutral-100"
            />
          </label>
          <label className="flex flex-col">
            <span>Subtotal mínimo (centavos)</span>
            <input
              type="number"
              min={0}
              value={value.minSubtotalCents}
              onChange={(e) =>
                set("minSubtotalCents", Number(e.target.value))
              }
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Máx. usos (0 = ilimitado)</span>
            <input
              type="number"
              min={0}
              value={value.maxUses}
              onChange={(e) => set("maxUses", Number(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Começa em</span>
            <input
              type="date"
              value={toDateInputValue(value.startsAt)}
              onChange={(e) =>
                set("startsAt", fromDateInputValue(e.target.value))
              }
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Expira em</span>
            <input
              type="date"
              value={toDateInputValue(value.expiresAt)}
              onChange={(e) =>
                set("expiresAt", fromDateInputValue(e.target.value))
              }
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="col-span-2 flex flex-col">
            <span>Observação (admin)</span>
            <input
              value={value.note ?? ""}
              onChange={(e) => set("note", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="col-span-2 flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={value.active ?? true}
              onChange={(e) => set("active", e.target.checked)}
            />
            <span>Ativo</span>
          </label>
        </div>
        <div className="mt-6 flex justify-end gap-3">
          <button
            type="button"
            onClick={onCancel}
            className="rounded border border-neutral-300 px-4 py-2 text-sm"
            disabled={saving}
          >
            Cancelar
          </button>
          <button
            type="button"
            onClick={onSave}
            className="rounded bg-black px-4 py-2 text-sm text-white disabled:opacity-50"
            disabled={saving || !value.code}
          >
            {saving ? "Salvando…" : "Salvar"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ----- Settings tab -----

// SettingsAdmin currently exposes a single key — the storefront
// countdown — but is structured as a generic settings hub so we can
// drop in announcement banners, hero CTAs, etc. without restructuring
// the tab.
function SettingsAdmin({ token }: { token: string }) {
  return (
    <div className="space-y-6">
      <BannerAdmin token={token} />
      <SecretAdmin token={token} />
      <CountdownAdmin token={token} />
    </div>
  );
}

const DEFAULT_BANNER: BannerSettings = {
  visible: false,
  text: "Frete gratis com o cupom NASTT",
  link: "",
  linkLabel: "",
  bgColor: "accent",
};

function BannerAdmin({ token }: { token: string }) {
  const [settings, setSettings] = useState<BannerSettings | null>(null);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    adminSettingsApi
      .getBanner(token)
      .then((data) => {
        if (!cancelled) setSettings(data);
      })
      .catch((e: Error) => {
        if (!cancelled) setErr(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  const update = (patch: Partial<BannerSettings>) =>
    setSettings((prev) => ({ ...(prev ?? DEFAULT_BANNER), ...patch }));

  const save = async () => {
    if (!settings) return;
    setErr("");
    setSaving(true);
    try {
      const saved = await adminSettingsApi.saveBanner(token, settings);
      setSettings(saved);
      setSavedAt(Date.now());
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  if (!settings && !err) {
    return (
      <div className="rounded border border-neutral-200 bg-white p-6 text-sm text-neutral-500">
        Carregando…
      </div>
    );
  }

  const value = settings ?? DEFAULT_BANNER;
  return (
    <section className="rounded border border-neutral-200 bg-white p-6">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Banner de Anuncio</h2>
          <p className="mt-1 text-xs text-neutral-500">
            Faixa no topo do site. Ative para mostrar uma mensagem ou promocao.
          </p>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={value.visible}
            onChange={(e) => update({ visible: e.target.checked })}
            className="h-4 w-4"
          />
          <span className="font-medium">Ativo</span>
        </label>
      </header>

      {err && (
        <div className="mt-4 rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      <div className="mt-6 space-y-4">
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-neutral-700">
            Texto do banner <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            value={value.text}
            onChange={(e) => update({ text: e.target.value })}
            placeholder="Frete gratis acima de R$299 — use o cupom NAST10"
            maxLength={200}
            className="rounded border border-neutral-300 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700">
              Link (opcional)
            </label>
            <input
              type="text"
              value={value.link ?? ""}
              onChange={(e) => update({ link: e.target.value })}
              placeholder="/colecao ou https://..."
              className="rounded border border-neutral-300 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
            />
          </div>
          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700">
              Texto do link (opcional)
            </label>
            <input
              type="text"
              value={value.linkLabel ?? ""}
              onChange={(e) => update({ linkLabel: e.target.value })}
              placeholder="Ver colecao"
              maxLength={60}
              className="rounded border border-neutral-300 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
            />
          </div>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-neutral-700">Cor de fundo</label>
          <div className="flex gap-3">
            {(["accent", "white", "black"] as const).map((c) => (
              <label key={c} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="radio"
                  name="bgColor"
                  value={c}
                  checked={(value.bgColor ?? "accent") === c}
                  onChange={() => update({ bgColor: c })}
                />
                <span className={
                  c === "accent" ? "font-medium text-yellow-600" :
                  c === "white" ? "font-medium" : "font-medium text-neutral-500"
                }>
                  {c === "accent" ? "Amarelo" : c === "white" ? "Branco" : "Preto"}
                </span>
              </label>
            ))}
          </div>
        </div>
      </div>

      <footer className="mt-6 flex items-center gap-4 border-t border-neutral-100 pt-4">
        <button
          onClick={save}
          disabled={saving}
          className="rounded bg-black px-4 py-2 text-sm font-semibold text-white hover:bg-neutral-800 disabled:opacity-50"
        >
          {saving ? "Salvando…" : "Salvar banner"}
        </button>
        {savedAt && (
          <span className="text-xs text-green-600">Salvo com sucesso!</span>
        )}
      </footer>
    </section>
  );
}

const DEFAULT_SECRET: SecretSettings = { locked: true, code: "10820", productId: "" };

function SecretAdmin({ token }: { token: string }) {
  const [settings, setSettings] = useState<SecretSettings | null>(null);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<number | null>(null);
  const [products, setProducts] = useState<AdminProductPayload[]>([]);

  useEffect(() => {
    adminApi.list(token).then(setProducts).catch(() => {});
  }, [token]);

  useEffect(() => {
    let cancelled = false;
    adminSettingsApi
      .getSecret(token)
      .then((data) => {
        if (!cancelled) setSettings(data);
      })
      .catch((e: Error) => {
        if (!cancelled) setErr(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  const update = (patch: Partial<SecretSettings>) =>
    setSettings((prev) => ({ ...(prev ?? DEFAULT_SECRET), ...patch }));

  const save = async () => {
    if (!settings) return;
    setErr("");
    setSaving(true);
    try {
      const saved = await adminSettingsApi.saveSecret(token, settings);
      setSettings(saved);
      setSavedAt(Date.now());
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  if (!settings && !err) {
    return (
      <div className="rounded border border-neutral-200 bg-white p-6 text-sm text-neutral-500">
        Carregando…
      </div>
    );
  }

  const value = settings ?? DEFAULT_SECRET;
  return (
    <section className="rounded border border-neutral-200 bg-white p-6">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Peça Secreta</h2>
          <p className="mt-1 text-xs text-neutral-500">
            Escolha qual produto fica escondido e defina o código de desbloqueio.
          </p>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={value.locked}
            onChange={(e) => update({ locked: e.target.checked })}
            className="h-4 w-4"
          />
          <span className="font-medium">Bloqueado (exige código)</span>
        </label>
      </header>

      {err && (
        <div className="mt-4 rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      <div className="mt-6 space-y-4">
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-neutral-700">
            Produto secreto
          </label>
          <select
            value={value.productId ?? ""}
            onChange={(e) => update({ productId: e.target.value })}
            className="rounded border border-neutral-300 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
          >
            <option value="">— selecione o produto —</option>
            {products.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <p className="text-xs text-neutral-400">
            Esse produto desaparece da grade normal e fica oculto até o cliente digitar o código.
          </p>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-neutral-700">
            Código de desbloqueio
          </label>
          <div className="flex gap-2">
            <input
              type="text"
              value={value.code}
              onChange={(e) => update({ code: e.target.value.toUpperCase() })}
              placeholder="ex: 10820"
              className="w-48 rounded border border-neutral-300 px-3 py-2 font-mono text-sm uppercase tracking-widest"
              disabled={!value.locked}
            />
            {!value.locked && (
              <span className="self-center text-xs text-neutral-400">
                (não necessário enquanto desbloqueado)
              </span>
            )}
          </div>
          <p className="text-xs text-neutral-400">
            Maiúsculas/minúsculas são ignoradas na comparação.
          </p>
        </div>
      </div>

      <div className="mt-6 flex items-center justify-end gap-4">
        {savedAt && (
          <span className="text-xs text-green-600">
            Salvo {new Date(savedAt).toLocaleTimeString("pt-BR")} — recarregue a loja para ver
          </span>
        )}
        <button
          type="button"
          onClick={() => void save()}
          disabled={saving}
          className="rounded bg-black px-4 py-2 text-sm text-white disabled:opacity-50"
        >
          {saving ? "Salvando…" : "Salvar"}
        </button>
      </div>
    </section>
  );
}

const DEFAULT_COUNTDOWN: CountdownSettings = {
  visible: false,
  title: "PRÓXIMO DROP",
  subtitle: "Edição limitada — peças numeradas",
  targetAt: "",
  ctaLabel: "Avise-me",
  ctaUrl: "",
  endedLabel: "Drop liberado",
};

function CountdownAdmin({ token }: { token: string }) {
  const [settings, setSettings] = useState<CountdownSettings | null>(null);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    adminSettingsApi
      .getCountdown(token)
      .then((data) => {
        if (!cancelled) setSettings(data);
      })
      .catch((e: Error) => {
        if (!cancelled) setErr(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  const update = (patch: Partial<CountdownSettings>) =>
    setSettings((prev) => ({ ...(prev ?? DEFAULT_COUNTDOWN), ...patch }));

  const save = async () => {
    if (!settings) return;
    setErr("");
    setSaving(true);
    try {
      // Convert <input type="datetime-local"> to RFC3339 with the
      // local timezone offset so the backend round-trips cleanly.
      const payload: CountdownSettings = {
        ...settings,
        targetAt: localDateTimeToRfc3339(settings.targetAt),
      };
      const saved = await adminSettingsApi.saveCountdown(token, payload);
      setSettings(saved);
      setSavedAt(Date.now());
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  if (!settings && !err) {
    return (
      <div className="rounded border border-neutral-200 bg-white p-6 text-sm text-neutral-500">
        Carregando…
      </div>
    );
  }

  const value = settings ?? DEFAULT_COUNTDOWN;
  return (
    <section className="rounded border border-neutral-200 bg-white p-6">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Countdown — próximo drop</h2>
          <p className="mt-1 text-xs text-neutral-500">
            Bloco que aparece na home, acima dos produtos. Quando desligado, a
            seção some completamente do site.
          </p>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={value.visible}
            onChange={(e) => update({ visible: e.target.checked })}
            className="h-4 w-4"
          />
          <span className="font-medium">Visível</span>
        </label>
      </header>

      {err && (
        <div className="mt-4 rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700">
          {err}
        </div>
      )}

      <div className="mt-6 grid grid-cols-1 gap-4 md:grid-cols-2">
        <Field label="Título">
          <input
            type="text"
            value={value.title}
            onChange={(e) => update({ title: e.target.value })}
            placeholder="PRÓXIMO DROP"
            className="w-full rounded border border-neutral-300 bg-white px-3 py-2 text-sm"
          />
        </Field>
        <Field label="Subtítulo">
          <input
            type="text"
            value={value.subtitle}
            onChange={(e) => update({ subtitle: e.target.value })}
            placeholder="Edição limitada — peças numeradas"
            className="w-full rounded border border-neutral-300 bg-white px-3 py-2 text-sm"
          />
        </Field>
        <Field label="Data e hora do drop">
          <input
            type="datetime-local"
            value={rfc3339ToLocalDateTime(value.targetAt)}
            onChange={(e) => update({ targetAt: e.target.value })}
            className="w-full rounded border border-neutral-300 bg-white px-3 py-2 text-sm"
          />
          <p className="mt-1 text-[11px] text-neutral-500">
            Deixar em branco mostra apenas a mensagem de “Drop liberado”.
          </p>
        </Field>
        <Field label="Mensagem após terminar">
          <input
            type="text"
            value={value.endedLabel}
            onChange={(e) => update({ endedLabel: e.target.value })}
            placeholder="Drop liberado"
            className="w-full rounded border border-neutral-300 bg-white px-3 py-2 text-sm"
          />
        </Field>
        <Field label="Botão (texto)">
          <input
            type="text"
            value={value.ctaLabel}
            onChange={(e) => update({ ctaLabel: e.target.value })}
            placeholder="Avise-me"
            className="w-full rounded border border-neutral-300 bg-white px-3 py-2 text-sm"
          />
        </Field>
        <Field label="Botão (link)">
          <input
            type="url"
            value={value.ctaUrl}
            onChange={(e) => update({ ctaUrl: e.target.value })}
            placeholder="https://wa.me/55..."
            className="w-full rounded border border-neutral-300 bg-white px-3 py-2 text-sm"
          />
          <p className="mt-1 text-[11px] text-neutral-500">
            Sem link, o botão não aparece.
          </p>
        </Field>
      </div>

      {/* Live preview — updates in real time as you edit */}
      <div className="mt-8 overflow-hidden rounded border border-neutral-200">
        <div className="border-b border-neutral-200 bg-neutral-50 px-3 py-2">
          <p className="text-[11px] font-semibold uppercase tracking-wider text-neutral-400">
            Preview ao vivo
          </p>
        </div>
        <CountdownView
          settings={{
            ...value,
            visible: true,
            targetAt: localDateTimeToRfc3339(value.targetAt),
          }}
        />
      </div>

      <div className="mt-6 flex items-center gap-3">
        <button
          type="button"
          onClick={() => void save()}
          disabled={saving}
          className="rounded bg-black px-4 py-2 text-sm text-white disabled:opacity-50"
        >
          {saving ? "Salvando…" : "Salvar"}
        </button>
        {savedAt && !saving && (
          <span className="text-xs text-emerald-700">
            Salvo {new Date(savedAt).toLocaleTimeString("pt-BR")}.
          </span>
        )}
      </div>
    </section>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
        {label}
      </span>
      {children}
    </label>
  );
}

// rfc3339ToLocalDateTime converts an RFC3339 timestamp into the
// "YYYY-MM-DDTHH:mm" string that <input type="datetime-local">
// expects. Returns "" when the input is empty or unparseable.
function rfc3339ToLocalDateTime(value: string): string {
  if (!value) return "";
  const ts = Date.parse(value);
  if (Number.isNaN(ts)) return "";
  const d = new Date(ts);
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

// localDateTimeToRfc3339 converts the datetime-local string back into
// RFC3339 (with the browser timezone offset). The backend re-normalizes
// to UTC, so the round trip is lossless.
function localDateTimeToRfc3339(value: string): string {
  if (!value) return "";
  // <input type="datetime-local"> doesn't include seconds — append :00
  // before parsing so Date can resolve it consistently.
  const normalized = value.length === 16 ? `${value}:00` : value;
  const ts = Date.parse(normalized);
  if (Number.isNaN(ts)) return "";
  return new Date(ts).toISOString();
}
