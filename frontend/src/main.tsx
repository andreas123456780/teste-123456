import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";

// Plausible is opt-in: set VITE_PLAUSIBLE_DOMAIN=nast.com.br at build
// time and the snippet is injected here. We inject at runtime (rather
// than in index.html) so a site built without the env var ships zero
// third-party JS — keeping CSP narrow and avoiding a cookie banner
// obligation for tracking scripts that never get loaded.
const plausibleDomain = (import.meta.env.VITE_PLAUSIBLE_DOMAIN ?? "").trim();
const plausibleSrc =
  (import.meta.env.VITE_PLAUSIBLE_SRC ?? "https://plausible.io/js/script.js").trim();
if (plausibleDomain) {
  const s = document.createElement("script");
  s.defer = true;
  s.src = plausibleSrc;
  s.setAttribute("data-domain", plausibleDomain);
  document.head.appendChild(s);
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
