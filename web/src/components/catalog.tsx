import { KeyRound, Loader2, RefreshCw, WifiOff } from "lucide-react";
import { imgURL, type DiscoverItem } from "../api";
import { useI18n } from "../i18n";
import { EmptyState } from "./ui";
import { Ratings } from "./Ratings";

// The poster grid and its companions (skeleton, error panel, sign-in gate) are
// shared by every kino.watch-backed page — Catalog, Bookmarks and History — so a
// title card looks and behaves the same wherever it is listed.

export function badgeOf(
  it: DiscoverItem,
  t: (k: string, v?: Record<string, string | number>) => string,
): string {
  if (it.season && it.season > 0 && it.episode) return t("Season {s}. Episode {e}", { s: it.season, e: it.episode });
  if (it.episode && it.episode > 1) return t("Episode {n}", { n: it.episode });
  return it.subtitle || "";
}

export function ItemsGrid({ items, onOpen }: { items: DiscoverItem[]; onOpen: (it: DiscoverItem) => void }) {
  const { t } = useI18n();
  return (
    <div className="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6">
      {items.map((it) => {
        const badge = badgeOf(it, t);
        return (
          <button key={it.id} onClick={() => onOpen(it)} className="group text-left" title={it.originalTitle || it.title}>
            <div className="relative overflow-hidden rounded-xl bg-gradient-to-br from-ink-700 to-ink-850">
              <img
                src={imgURL(it.poster)}
                alt={it.title}
                loading="lazy"
                className="aspect-[2/3] w-full object-cover transition duration-200 group-hover:scale-[1.03]"
                onError={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = "hidden")}
              />
              {badge && (
                <span className="absolute bottom-1.5 left-1.5 right-1.5 truncate rounded-md bg-black/75 px-1.5 py-0.5 text-center text-[10px] font-semibold text-emerald-300">
                  {badge}
                </span>
              )}
            </div>
            {/* Every meta line is always present (nbsp filler) and the ratings row
                has a reserved fixed height, so cards are uniformly tall and the
                grid rows don't jump between titles that have more/less metadata. */}
            <p className="mt-1.5 truncate text-xs font-semibold text-slate-100">{it.title}</p>
            <p className="truncate text-[11px] text-slate-500">{it.originalTitle || " "}</p>
            <p className="text-[11px] text-slate-500">{it.year || " "}</p>
            <div className="mt-1 flex h-5 items-center overflow-hidden">
              <Ratings item={it} />
            </div>
          </button>
        );
      })}
    </div>
  );
}

// SkeletonGrid shows placeholder cards on cold start so a page isn't empty while
// its first batch loads. `wide` matches the 16:9 folder/collection tiles.
export function SkeletonGrid({ wide }: { wide?: boolean }) {
  return (
    <div className={wide ? "grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4" : "grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6"}>
      {Array.from({ length: wide ? 8 : 18 }).map((_, i) => (
        <div key={i} className="animate-pulse">
          <div className={`${wide ? "aspect-video" : "aspect-[2/3]"} w-full rounded-xl bg-white/[0.05]`} />
          <div className="mt-1.5 h-3.5 w-3/4 rounded bg-white/[0.05]" />
          {!wide && (
            <>
              {/* Mirror the real card's meta lines (original title, year, ratings)
                  so the grid height doesn't jump when posters finish loading. */}
              <div className="mt-1.5 h-2.5 w-1/2 rounded bg-white/[0.04]" />
              <div className="mt-1.5 h-2.5 w-1/4 rounded bg-white/[0.04]" />
              <div className="mt-1.5 h-4 w-2/3 rounded bg-white/[0.04]" />
            </>
          )}
        </div>
      ))}
    </div>
  );
}

// CatalogError replaces the grid when a request fails — most commonly because
// kino.watch is unreachable without a VPN. Offers a one-tap retry and a shortcut
// to Settings (where the proxy lives).
export function CatalogError({ onRetry, onOpenSettings }: { onRetry: () => void; onOpenSettings: () => void }) {
  const { t } = useI18n();
  return (
    <EmptyState
      icon={<WifiOff className="h-6 w-6" />}
      title={t("Couldn't reach kino.watch")}
      hint={t("If kino.watch is blocked in your region, enable a VPN (or set a proxy in Settings), then try again.")}
      action={
        <div className="flex flex-wrap items-center justify-center gap-2">
          <button className="btn-primary" onClick={onRetry}>
            <RefreshCw className="h-4 w-4" /> {t("Retry")}
          </button>
          <button className="btn-ghost" onClick={onOpenSettings}>
            {t("Go to Settings")}
          </button>
        </div>
      }
    />
  );
}

// SignInGate stands in for a page's whole body while logged out. The title says
// what *this* page needs the account for; the route to fix it is always Profile.
export function SignInGate({ title, onSignIn }: { title: string; onSignIn: () => void }) {
  const { t } = useI18n();
  return (
    <EmptyState
      icon={<KeyRound className="h-6 w-6" />}
      title={title}
      hint={t("The catalog, search, voiceovers and one-click downloads use the official kino.watch API. Sign in once in Profile.")}
      action={
        <button className="btn-primary" onClick={onSignIn}>
          <KeyRound className="h-4 w-4" /> {t("Go to Profile")}
        </button>
      }
    />
  );
}

// ListSpinner marks a page-append in progress, under results already on screen.
export function ListSpinner() {
  return (
    <div className="flex justify-center py-6 text-slate-400">
      <Loader2 className="h-5 w-5 animate-spin" />
    </div>
  );
}
