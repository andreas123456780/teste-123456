import { useEffect, useState } from "react";
import type { BannerSettings } from "../api";

type Props = {
  settings: BannerSettings | null;
};

const DISMISS_KEY = "nast:banner-dismissed";

export function AnnouncementBanner({ settings }: Props) {
  const [dismissed, setDismissed] = useState(() => {
    try {
      return sessionStorage.getItem(DISMISS_KEY) === "1";
    } catch {
      return false;
    }
  });

  useEffect(() => {
    if (!settings) return;
    try {
      if (sessionStorage.getItem(DISMISS_KEY) !== "1") {
        setDismissed(false);
      }
    } catch {
      /* ignore */
    }
  }, [settings]);

  if (!settings?.visible || dismissed) return null;

  const dismiss = () => {
    setDismissed(true);
    try {
      sessionStorage.setItem(DISMISS_KEY, "1");
    } catch {
      /* ignore */
    }
  };

  const bg =
    settings.bgColor === "white"
      ? "bg-white text-black"
      : settings.bgColor === "black"
        ? "bg-black text-white border-b border-white/10"
        : "bg-[var(--color-accent)] text-black";

  return (
    <div
      className={`relative z-50 flex items-center justify-center gap-3 px-10 py-2.5 text-xs font-bold uppercase tracking-[0.2em] ${bg}`}
    >
      <span>{settings.text}</span>
      {settings.link && settings.linkLabel && (
        <a
          href={settings.link}
          className="underline underline-offset-2 opacity-80 hover:opacity-100"
        >
          {settings.linkLabel}
        </a>
      )}
      <button
        onClick={dismiss}
        aria-label="Fechar banner"
        className="absolute right-3 top-1/2 -translate-y-1/2 opacity-60 hover:opacity-100"
      >
        ✕
      </button>
    </div>
  );
}
