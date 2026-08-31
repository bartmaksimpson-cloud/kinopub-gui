import { useCallback, useState } from "react";
import { PlayCircle } from "lucide-react";
import { api, type DiscoverItem } from "../api";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { dismiss, pushRoute, replaceRoute, useRoute } from "../router";
import { usePaged } from "../usePaged";
import { EmptyState } from "../components/ui";
import { CatalogError, ItemsGrid, ListSpinner, SignInGate, SkeletonGrid } from "../components/catalog";
import { TitleDetail } from "../components/TitleDetail";

// "I'm watching" is the shelf of things left part-way through, now a rail entry
// of its own ("#/watching") instead of a chip inside the Catalog.
//
// Subscriptions moved here too. It used to hang off the Collections tab, but it
// is not a подборка at all — it hits the same /watching endpoint as the two tabs
// beside it, just with subscribed=1. Grouped with them it finally reads as what
// it is: another slice of "shows I follow".
type Tab = "serials" | "movies" | "subs";

export function WatchingPage({
  onSignIn,
  onOpenSettings,
}: {
  onSignIn: () => void;
  onOpenSettings: () => void;
}) {
  const { kpauth, toast } = useApp();
  const { t } = useI18n();
  const loggedIn = kpauth.loggedIn;

  const detailId = useRoute().itemId ?? null;
  const [tab, setTab] = useState<Tab>("serials");

  const load = useCallback(
    (p: number) =>
      tab === "subs"
        ? api.discoverWatching(true, "serials", p)
        : api.discoverWatching(false, tab, p),
    [tab],
  );
  const feed = usePaged<DiscoverItem>({
    enabled: loggedIn,
    sourceKey: tab,
    load,
    onAppendError: (m) => toast(m || t("Catalog request failed"), "error"),
  });

  if (!loggedIn) {
    return (
      <div className="mx-auto max-w-6xl space-y-5">
        <Header />
        <SignInGate title={t("Sign in to kino.watch to see what you're watching")} onSignIn={onSignIn} />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <Header />

      <div className="flex flex-wrap gap-2">
        <TabChip active={tab === "serials"} onClick={() => setTab("serials")}>{t("Series")}</TabChip>
        <TabChip active={tab === "movies"} onClick={() => setTab("movies")}>{t("Movies")}</TabChip>
        <TabChip active={tab === "subs"} onClick={() => setTab("subs")}>{t("Subscriptions")}</TabChip>
      </div>

      {feed.error && feed.items.length === 0 ? (
        <CatalogError onRetry={feed.reload} onOpenSettings={onOpenSettings} />
      ) : feed.loading && feed.items.length === 0 ? (
        <SkeletonGrid />
      ) : feed.items.length === 0 ? (
        <EmptyState
          icon={<PlayCircle className="h-6 w-6" />}
          title={tab === "subs" ? t("No subscriptions yet") : t("Nothing in progress")}
          hint={
            tab === "subs"
              ? t("Series you subscribe to on kino.watch are listed here.")
              : t("Start something on kino.watch and it waits for you here until you finish it.")
          }
        />
      ) : (
        <ItemsGrid items={feed.items} onOpen={(it) => pushRoute({ page: "watching", itemId: it.id })} />
      )}

      {feed.loading && feed.items.length > 0 && <ListSpinner />}
      <div ref={feed.sentinelRef} className="h-1" />

      {detailId && (
        <TitleDetail
          id={detailId}
          onClose={() => dismiss({ page: "watching" })}
          // A "similar" pick swaps the card in place (one modal = one history
          // entry), so the X closes cleanly instead of stepping back through
          // every card visited.
          onPick={(it) => replaceRoute({ page: "watching", itemId: it.id })}
        />
      )}
    </div>
  );
}

function Header() {
  const { t } = useI18n();
  return (
    <header>
      <h1 className="text-2xl font-bold text-slate-100">{t("I'm watching")}</h1>
      <p className="mt-1 text-sm text-slate-400">
        {t("Series and films you're part-way through, plus the shows you follow.")}
      </p>
    </header>
  );
}

function TabChip({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`rounded-lg px-3 py-1.5 text-sm transition ${active ? "bg-gold-500/[0.14] text-gold-200" : "text-slate-400 hover:bg-white/[0.05] hover:text-slate-200"}`}
    >
      {children}
    </button>
  );
}
