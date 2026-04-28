import { useEffect, useState } from "react";
import { Instagram as InstagramIcon } from "./icons";
import { Reveal } from "./Reveal";

// InstagramFeed fetches /api/social/instagram and renders a 2-row grid
// of up to 6 recent posts. When the backend reports configured=false
// (missing INSTAGRAM_ACCESS_TOKEN / INSTAGRAM_USER_ID) OR the feed is
// empty, the widget renders nothing — the homepage should degrade
// silently rather than show a broken section.

type InstagramPost = {
  id: string;
  mediaType: string;
  mediaUrl: string;
  thumbnailUrl?: string;
  permalink: string;
  caption?: string;
  timestamp: string;
};

type FeedResponse = {
  posts: InstagramPost[];
  fetchedAt: string;
  configured: boolean;
};

const API_BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

export function InstagramFeed({ handle }: { handle: string }) {
  const [posts, setPosts] = useState<InstagramPost[] | null>(null);
  const [configured, setConfigured] = useState<boolean | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(`${API_BASE}/api/social/instagram`)
      .then((r) => (r.ok ? (r.json() as Promise<FeedResponse>) : null))
      .then((resp) => {
        if (cancelled || !resp) return;
        setPosts(resp.posts ?? []);
        setConfigured(resp.configured);
      })
      .catch(() => {
        if (!cancelled) setPosts([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Not configured → render nothing. The cleanest fallback for launch
  // day when the access token is still being generated.
  if (configured === false) return null;
  // No posts (loading state and empty API response share this branch) →
  // render nothing to avoid layout shift.
  if (!posts || posts.length === 0) return null;

  return (
    <section className="bg-white py-16 md:py-24" aria-labelledby="ig-heading">
      <div className="mx-auto max-w-6xl px-6">
        <Reveal>
        <header className="mb-8 flex items-center justify-between gap-4">
          <div>
            <p className="text-xs uppercase tracking-widest text-neutral-500">
              Instagram
            </p>
            <h2 id="ig-heading" className="text-3xl font-bold text-black">
              @{handle}
            </h2>
          </div>
          <a
            href={`https://instagram.com/${handle}`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 text-sm font-medium text-black hover:underline"
          >
            <InstagramIcon className="h-5 w-5" />
            Seguir
          </a>
        </header>
        </Reveal>

        <ul className="grid grid-cols-2 gap-2 md:grid-cols-3 md:gap-4">
          {posts.slice(0, 6).map((p, i) => {
            const src = p.thumbnailUrl || p.mediaUrl;
            return (
              <li key={p.id} className="aspect-square overflow-hidden">
                <Reveal delay={i * 0.05} y={20} className="h-full w-full">
                  <a
                    href={p.permalink}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label={p.caption || "Instagram post"}
                    className="group block h-full w-full"
                  >
                    <img
                      src={src}
                      alt={p.caption || "Instagram post"}
                      loading="lazy"
                      className="h-full w-full object-cover transition duration-500 group-hover:scale-105"
                    />
                  </a>
                </Reveal>
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
}
