import { useEffect, useState } from "react";
import type { BannerSettings } from "../api";

type Props = {
  settings: BannerSettings | null;
};

const DISMISS_KEY = "nast:banner-dismissed";

const REPEAT = 6;

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

  const separator = settings.bgColor === "white" ? "✦" : settings.bgColor === "black" ? "✦" : "✦";

  const item = (
    <>
      <span className="whitespace-nowrap font-bold uppercase tracking-[0.25em]">
        {settings.text}
      </span>
      {settings.link && settings.linkLabel ? (
        <a
          href={settings.link}
          className="whitespace-nowrap underline underline-offset-2 opacity-70 hover:opacity-100 font-bold uppercase tracking-[0.25em]"
          onClick={(e) => e.stopPropagation()}
        >
          {settings.linkLabel}
        </a>
      ) : null}
      <span className="opacity-40 select-none mx-4">{separator}</span>
    </>
  );

  return (
    <div className={`relative z-50 overflow-hidden py-2 text-xs ${bg}`}>
      <div className="marquee" aria-label={settings.text}>
        {Array.from({ length: REPEAT }).map((_, i) => (
          <span key={i} className="flex items-center gap-6 shrink-0">
            {item}
          </span>
        ))}
      </div>
      <button
        onClick={dismiss}
        aria-label="Fechar banner"
        className="absolute right-3 top-1/2 -translate-y-1/2 z-10 text-[10px] opacity-50 hover:opacity-100 transition-opacity bg-transparent border-none cursor-pointer"
      >
        ✕
      </button>
    </div>
  );
}
