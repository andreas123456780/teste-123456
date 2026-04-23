import { useCallback, useEffect, useState } from "react";
import { adminApi, type AdminProductPayload } from "../api";
import type { Product } from "../types";

// Admin is an intentionally plain, no-deps management screen. Operators
// paste the ADMIN_TOKEN once — it is persisted to localStorage so the
// page survives reloads. All network calls add the header
// X-Admin-Token; the backend enforces authentication (constant-time
// compare) so the UI never ships privileged data statically.
const TOKEN_STORAGE = "nast:admin-token:v1";

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
    if (!token) return;
    // refresh() performs async work and only touches state after the
    // fetch resolves; the set-state-in-effect rule flags it statically
    // so we disable locally.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
  }, [token, refresh]);

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
    setItems(null);
  };

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
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setEditing(emptyProduct())}
            className="rounded bg-black px-4 py-2 text-sm text-white"
          >
            Novo produto
          </button>
          <button
            type="button"
            onClick={logout}
            className="rounded border border-neutral-300 px-4 py-2 text-sm"
          >
            Sair
          </button>
        </div>
      </header>

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
        <EditForm
          value={editing}
          onChange={setEditing}
          onSave={save}
          onCancel={() => setEditing(null)}
          saving={saving}
        />
      )}
    </main>
  );
}

type EditProps = {
  value: AdminProductPayload;
  onChange: (v: AdminProductPayload) => void;
  onSave: () => void;
  onCancel: () => void;
  saving: boolean;
};

function EditForm({ value, onChange, onSave, onCancel, saving }: EditProps) {
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
          <label className="col-span-2 flex flex-col">
            <span>Imagem</span>
            <input
              value={value.image}
              onChange={(e) => set("image", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="col-span-2 flex flex-col">
            <span>Imagem (costas)</span>
            <input
              value={value.backImage ?? ""}
              onChange={(e) => set("backImage", e.target.value)}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Cores (vírgula)</span>
            <input
              value={(value.colors ?? []).join(", ")}
              onChange={(e) => set("colors", parseCsv(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="flex flex-col">
            <span>Tamanhos (vírgula)</span>
            <input
              value={(value.sizes ?? []).join(", ")}
              onChange={(e) => set("sizes", parseCsv(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="col-span-2 flex flex-col">
            <span>Tags (vírgula)</span>
            <input
              value={(value.tags ?? []).join(", ")}
              onChange={(e) => set("tags", parseCsv(e.target.value))}
              className="rounded border border-neutral-300 px-2 py-1"
            />
          </label>
          <label className="col-span-2 flex items-center gap-2">
            <input
              type="checkbox"
              checked={value.hidden ?? false}
              onChange={(e) => set("hidden", e.target.checked)}
            />
            <span>Ocultar da vitrine</span>
          </label>
        </div>
        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded border border-neutral-300 px-4 py-2 text-sm"
          >
            Cancelar
          </button>
          <button
            type="button"
            onClick={onSave}
            disabled={saving || !value.id || !value.name}
            className="rounded bg-black px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {saving ? "Salvando…" : "Salvar"}
          </button>
        </div>
      </div>
    </div>
  );
}
