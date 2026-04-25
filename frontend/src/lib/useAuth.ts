import { useCallback, useEffect, useRef, useState } from "react";
import { authApi, type AuthUser } from "../api";

// useAuth is the single source of truth for "is the current visitor
// logged in?". It fetches /api/auth/me on mount, caches the result,
// and exposes login/signup/logout helpers that keep the cache in
// sync. Deliberately minimal — no context provider needed since the
// app only has a handful of routes that care about auth.
//
// State transitions:
//   loading=true, user=null     → fetching /me
//   loading=false, user=null    → anonymous
//   loading=false, user=AuthUser → authenticated
//   loading=false, disabled=true → backend returned 503 (AUTH_SESSION_SECRET not set)

export type UseAuthState = {
  user: AuthUser | null;
  loading: boolean;
  disabled: boolean;
  error: string | null;
};

export type UseAuthApi = UseAuthState & {
  refresh: () => Promise<void>;
  signup: (input: { name: string; email: string; password: string }) => Promise<AuthUser>;
  login: (input: { email: string; password: string }) => Promise<AuthUser>;
  logout: () => Promise<void>;
};

export function useAuth(): UseAuthApi {
  const [state, setState] = useState<UseAuthState>({
    user: null,
    loading: true,
    disabled: false,
    error: null,
  });
  // alive prevents setting state on an unmounted component during
  // strict-mode double-invoke in dev.
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    try {
      const me = await authApi.me();
      if (alive.current) {
        setState({ user: me, loading: false, disabled: false, error: null });
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (!alive.current) return;
      if (msg.startsWith("API 503")) {
        setState({ user: null, loading: false, disabled: true, error: null });
      } else if (msg.startsWith("API 401")) {
        setState({ user: null, loading: false, disabled: false, error: null });
      } else {
        setState({ user: null, loading: false, disabled: false, error: msg });
      }
    }
  }, []);

  useEffect(() => {
    // Fire-and-forget: state setters already early-out when unmounted.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
  }, [refresh]);

  const signup = useCallback(
    async (input: { name: string; email: string; password: string }) => {
      const me = await authApi.signup(input);
      if (alive.current) {
        setState({ user: me, loading: false, disabled: false, error: null });
      }
      return me;
    },
    [],
  );

  const login = useCallback(async (input: { email: string; password: string }) => {
    const me = await authApi.login(input);
    if (alive.current) {
      setState({ user: me, loading: false, disabled: false, error: null });
    }
    return me;
  }, []);

  const logout = useCallback(async () => {
    await authApi.logout();
    if (alive.current) {
      setState({ user: null, loading: false, disabled: false, error: null });
    }
  }, []);

  return { ...state, refresh, signup, login, logout };
}
