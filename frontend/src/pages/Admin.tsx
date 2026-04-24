import { useCallback, useEffect, useState } from "react";
import {
  adminApi,
  adminAuthApi,
  adminCouponsApi,
  type AdminCouponPayload,
  type AdminProductPayload,
  type AdminStats,
} from "../api";
import type { Coupon, Product } from "../types";

// Admin is an intentionally plain, no-deps management screen. Operators
// authenticate either with username+password (preferred, when the
// backend has ADMIN_USERNAME + ADMIN_PASSWORD_HASH configured) or by
// pasting a static ADMIN_TOKEN as a fallback. The issued session token
// is persisted to localStorage; every request carries X-Admin-Token
// and the backend enforces authentication (HMAC verify or constant-time
// compare). The UI never ships privileged data statically.
const TOKEN_STORAGE = "nast:admin-token:v1";

type Tab = "dashboard" | "products" | "coupons";

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
          ["products", "Produtos"],
          ["coupons", "Cupons"],
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
      {tab === "products" && <ProductsAdmin token={token} />}
      {tab === "coupons" && <CouponsAdmin token={token} />}
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

  return (
    <section className="space-y-8">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Kpi label="Pedidos totais" value={stats.orders.total.toString()} />
        <Kpi label="Pagos" value={stats.orders.paid.toString()} />
        <Kpi label="Pendentes" value={stats.orders.pendingPayment.toString()} />
        <Kpi label="Cancelados" value={stats.orders.canceled.toString()} />
        <Kpi label="Receita bruta" value={moneyBR(stats.revenue.grossCents)} />
        <Kpi
          label="Frete arrecadado"
          value={moneyBR(stats.revenue.shippingCents)}
        />
        <Kpi
          label="Descontos aplicados"
          value={moneyBR(stats.revenue.discountCents)}
        />
        <Kpi
          label="Ticket médio"
          value={moneyBR(stats.revenue.avgTicketCents)}
        />
      </div>

      <div>
        <h2 className="mb-3 text-lg font-semibold">Mais vendidos</h2>
        {stats.topProducts.length === 0 ? (
          <p className="text-sm text-neutral-500">Nenhuma venda ainda.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 text-left">
                <th className="py-2">Produto</th>
                <th className="py-2">Qtd vendida</th>
                <th className="py-2">Receita</th>
              </tr>
            </thead>
            <tbody>
              {stats.topProducts.map((p) => (
                <tr key={p.productId} className="border-b border-neutral-100">
                  <td className="py-2">{p.productName || p.productId}</td>
                  <td className="py-2">{p.quantity}</td>
                  <td className="py-2">{moneyBR(p.grossCents)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div>
        <h2 className="mb-3 text-lg font-semibold">Receita por dia (últimos 30d)</h2>
        {stats.revenueByDay.length === 0 ? (
          <p className="text-sm text-neutral-500">Sem receita no período.</p>
        ) : (
          <DailyRevenueChart data={stats.revenueByDay} />
        )}
      </div>

      <div>
        <h2 className="mb-3 text-lg font-semibold">Últimos pedidos</h2>
        {stats.recentOrders.length === 0 ? (
          <p className="text-sm text-neutral-500">Nenhum pedido.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 text-left">
                <th className="py-2">ID</th>
                <th className="py-2">Cliente</th>
                <th className="py-2">Método</th>
                <th className="py-2">Status</th>
                <th className="py-2">Valor</th>
                <th className="py-2">Data</th>
              </tr>
            </thead>
            <tbody>
              {stats.recentOrders.map((o) => (
                <tr key={o.id} className="border-b border-neutral-100">
                  <td className="py-2 font-mono text-xs">{o.id.slice(0, 10)}</td>
                  <td className="py-2">{o.customerName}</td>
                  <td className="py-2">{o.paymentMethod}</td>
                  <td className="py-2">
                    <StatusBadge status={o.status} />
                  </td>
                  <td className="py-2">{moneyBR(o.amountCents)}</td>
                  <td className="py-2 text-neutral-500">
                    {new Date(o.createdAt).toLocaleString("pt-BR")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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

function Kpi({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-neutral-200 p-3">
      <p className="text-xs uppercase tracking-wide text-neutral-500">{label}</p>
      <p className="mt-1 text-xl font-semibold">{value}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    paid: "bg-green-100 text-green-800",
    shipped: "bg-blue-100 text-blue-800",
    pending_payment: "bg-yellow-100 text-yellow-800",
    failed: "bg-red-100 text-red-800",
    canceled: "bg-neutral-200 text-neutral-700",
  };
  const cls = map[status] ?? "bg-neutral-100 text-neutral-700";
  return (
    <span className={`rounded px-2 py-0.5 text-xs ${cls}`}>{status}</span>
  );
}

function DailyRevenueChart({
  data,
}: {
  data: { day: string; grossCents: number; orderCount: number }[];
}) {
  const max = Math.max(1, ...data.map((d) => d.grossCents));
  return (
    <div className="flex items-end gap-1 overflow-x-auto" style={{ height: 120 }}>
      {data.map((d) => {
        const h = Math.max(2, Math.round((d.grossCents / max) * 100));
        return (
          <div
            key={d.day}
            className="flex min-w-[18px] flex-col items-center"
            title={`${d.day}: ${moneyBR(d.grossCents)} (${d.orderCount} pedidos)`}
          >
            <div
              className="w-full rounded-sm bg-black"
              style={{ height: `${h}%` }}
            />
            <span className="mt-1 text-[10px] text-neutral-400">
              {d.day.slice(5)}
            </span>
          </div>
        );
      })}
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
            {items.map((p) => (
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
            <input
              value={value.colors.join(", ")}
              onChange={(e) => set("colors", parseCsv(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Tamanhos (csv)</span>
            <input
              value={value.sizes.join(", ")}
              onChange={(e) => {
                const sizes = parseCsv(e.target.value);
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
            <input
              value={value.tags.join(", ")}
              onChange={(e) => set("tags", parseCsv(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
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
