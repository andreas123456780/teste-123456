import { useCallback, useEffect, useState } from "react";
import {
  adminApi,
  adminCouponsApi,
  type AdminCouponPayload,
  type AdminProductPayload,
} from "../api";
import type { Coupon, Product } from "../types";

// Admin is an intentionally plain, no-deps management screen. Operators
// paste the ADMIN_TOKEN once — it is persisted to localStorage so the
// page survives reloads. All network calls add the header
// X-Admin-Token; the backend enforces authentication (constant-time
// compare) so the UI never ships privileged data statically.
const TOKEN_STORAGE = "nast:admin-token:v1";

type Tab = "products" | "coupons";

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
    hidden: false,
  };
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

export function AdminPage() {
  const [token, setToken] = useState<string>(readToken);
  const [tokenDraft, setTokenDraft] = useState<string>(token);
  const [tab, setTab] = useState<Tab>("products");

  const login = () => {
    try {
      window.localStorage.setItem(TOKEN_STORAGE, tokenDraft);
    } catch {
      // localStorage unavailable (private mode) — still set in-memory.
    }
    setToken(tokenDraft);
  };

  const logout = () => {
    try {
      window.localStorage.removeItem(TOKEN_STORAGE);
    } catch {
      // noop
    }
    setToken("");
    setTokenDraft("");
  };

  if (!token) {
    return (
      <main className="mx-auto max-w-md px-6 py-20">
        <h1 className="mb-6 text-3xl font-bold">NAST — Admin</h1>
        <p className="mb-4 text-sm text-neutral-600">
          Informe o token de administração (<code>ADMIN_TOKEN</code> do
          backend). O token fica salvo no seu navegador.
        </p>
        <input
          type="password"
          autoComplete="off"
          placeholder="X-Admin-Token"
          value={tokenDraft}
          onChange={(e) => setTokenDraft(e.target.value)}
          className="mb-3 w-full rounded border border-neutral-300 px-3 py-2"
        />
        <button
          type="button"
          onClick={login}
          disabled={!tokenDraft}
          className="w-full rounded bg-black px-4 py-2 text-white disabled:opacity-50"
        >
          Entrar
        </button>
      </main>
    );
  }

  return (
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

      {tab === "products" && <ProductsAdmin token={token} />}
      {tab === "coupons" && <CouponsAdmin token={token} />}
    </main>
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
};

function ProductEditForm({
  value,
  onChange,
  onSave,
  onCancel,
  saving,
}: ProductEditProps) {
  const set = <K extends keyof AdminProductPayload>(
    key: K,
    v: AdminProductPayload[K],
  ) => onChange({ ...value, [key]: v });

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-2xl overflow-auto rounded bg-white p-6 shadow-lg">
        <h2 className="mb-4 text-xl font-semibold">
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
            <span>Estoque</span>
            <input
              type="number"
              value={value.stock}
              onChange={(e) => set("stock", Number(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Imagem (frente)</span>
            <input
              value={value.image}
              onChange={(e) => set("image", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Imagem (verso)</span>
            <input
              value={value.backImage}
              onChange={(e) => set("backImage", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
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
              onChange={(e) => set("sizes", parseCsv(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
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
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-xl overflow-auto rounded bg-white p-6 shadow-lg">
        <h2 className="mb-4 text-xl font-semibold">
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
