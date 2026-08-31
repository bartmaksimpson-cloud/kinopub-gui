import { useCallback } from "react";
import { History as HistoryIcon } from "lucide-react";
import { api, type DiscoverItem } from "../api";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { dismiss, pushRoute, replaceRoute, useRoute } from "../router";
import { usePaged } from "../usePaged";
import { EmptyState } from "../components/ui";
import { CatalogError, ItemsGrid, ListSpinner, SignInGate, SkeletonGrid } from "../components/catalog";
import { TitleDetail } from "../components/TitleDetail";

// Watch history used to be a chip inside the Catalog, three clicks deep and
// sharing that page's search box and filters — neither of which history supports.
// It is now its own rail entry with its own route ("#/history"), so it survives a
// reload and can be linked to directly.
export function HistoryPage({
  onSignIn,
  onOpenSettings,
}: {
  onSignIn: () => void;
  onOpenSettings: () => void;
}) {
  const { kpauth, toast } = useApp();
  const { t } = useI18n();
  const loggedIn = kpauth.loggedIn;

  // The open card lives in the hash, so browser-back closes it.
  const detailId = useRoute().itemId ?? null;

  const load = useCallback((p: number) => api.discoverHistory(p), []);
  const feed = usePaged<DiscoverItem>({
    enabled: loggedIn,
    sourceKey: "history",
    load,
    onAppendError: (m) => toast(m || t("Catalog request failed"), "error"),
  });

  if (!loggedIn) {
    return (
      <div className="mx-auto max-w-6xl space-y-5">
        <Header />
        <SignInGate title={t("Sign in to kino.watch to see your watch history")} onSignIn={onSignIn} />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <Header />

      {feed.error && feed.items.length === 0 ? (
        <CatalogError onRetry={feed.reload} onOpenSettings={onOpenSettings} />
      ) : feed.loading && feed.items.length === 0 ? (
        <SkeletonGrid />
      ) : feed.items.length === 0 ? (
        <EmptyState
          icon={<HistoryIcon className="h-6 w-6" />}
          title={t("Nothing watched yet")}
          hint={t("Titles you play on kino.watch — here or anywhere else — show up on this page.")}
        />
      ) : (
        <HistoryDays items={feed.items} onOpen={(it) => pushRoute({ page: "history", itemId: it.id })} />
      )}

      {feed.loading && feed.items.length > 0 && <ListSpinner />}
      <div ref={feed.sentinelRef} className="h-1" />

      {detailId && (
        <TitleDetail
          id={detailId}
          onClose={() => dismiss({ page: "history" })}
          // A "similar" pick swaps the card in place (one modal = one history
          // entry), so the X closes cleanly instead of stepping back through
          // every card visited.
          onPick={(it) => replaceRoute({ page: "history", itemId: it.id })}
        />
      )}
    </div>
  );
}

function Header() {
  const { t } = useI18n();
  return (
    <header>
      <h1 className="text-2xl font-bold text-slate-100">{t("History")}</h1>
      <p className="mt-1 text-sm text-slate-400">
        {t("Everything you've watched on kino.watch, newest first — open a title to download it.")}
      </p>
    </header>
  );
}

// HistoryDays groups the feed into per-day sections. The API already returns
// newest first, so a section break is simply "the day changed".
function HistoryDays({ items, onOpen }: { items: DiscoverItem[]; onOpen: (it: DiscoverItem) => void }) {
  const { lang } = useI18n();
  const fmt = new Intl.DateTimeFormat(lang === "ru" ? "ru-RU" : "en-US", { day: "numeric", month: "long", year: "numeric" });
  const groups: { day: string; items: DiscoverItem[] }[] = [];
  for (const it of items) {
    const day = it.watchedAt ? fmt.format(new Date(it.watchedAt * 1000)) : "";
    const last = groups[groups.length - 1];
    if (last && last.day === day) last.items.push(it);
    else groups.push({ day, items: [it] });
  }
  return (
    <div className="space-y-6">
      {groups.map((g, i) => (
        <div key={i}>
          {g.day && <h3 className="mb-2.5 text-sm font-bold text-slate-200">{g.day}</h3>}
          <ItemsGrid items={g.items} onOpen={onOpen} />
        </div>
      ))}
    </div>
  );
}
