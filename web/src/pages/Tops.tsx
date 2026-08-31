import { useCallback, useState } from "react";
import { Flame, LayoutGrid } from "lucide-react";
import { api, type DiscoverItem, type TopKind } from "../api";
import { CATEGORIES } from "../categories";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { dismiss, pushRoute, replaceRoute, useRoute } from "../router";
import { usePaged } from "../usePaged";
import { EmptyState } from "../components/ui";
import { CatalogError, ItemsGrid, ListSpinner, SignInGate, SkeletonGrid } from "../components/catalog";
import { TitleDetail } from "../components/TitleDetail";

// kino.watch's three charts (/v1/items/{fresh,hot,popular}) — the lists the site
// itself opens with. They are dedicated endpoints, not a sort over the catalog:
// ordering everything by views returns an all-time hall of fame rather than
// what's on now, which is what this page used to show as chips inside the
// Catalog.
//
// A chart is ranked server-side from a curated pool and accepts exactly one
// narrowing — the content type. So the page is two choices and a grid, while the
// Catalog keeps the search box, genres and filters that a chart can't honour.
const CHARTS: { kind: TopKind; label: string }[] = [
  { kind: "fresh", label: "Fresh" },
  { kind: "hot", label: "Hot" },
  { kind: "popular", label: "Popular" },
];

// The API rejects a chart without a type, so the genre-based categories (Anime,
// Sport) can't appear here — they are a genre spanning several types, with no
// type of their own to send.
const TYPES = CATEGORIES.filter((c) => c.type);

// There is no "any type" value — type=all comes back empty — but the parameter
// does accept a CSV, and the API merges and ranks the combined list itself
// (movie+serial returns exactly their two totals, interleaved). So "All" is a
// real single feed with working pagination, not a client-side splice of several.
const ALL_TYPES = TYPES.map((c) => c.type).join(",");

export function TopsPage({
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

  const [kind, setKind] = useState<TopKind>("fresh");
  // Everything by default — the charts read as one list, and the type chips are
  // there to narrow it rather than something you must pick first.
  const [type, setType] = useState<string>(ALL_TYPES);

  const load = useCallback((p: number) => api.discoverTop(kind, type, p), [kind, type]);
  const feed = usePaged<DiscoverItem>({
    enabled: loggedIn,
    sourceKey: `${kind}:${type}`,
    load,
    onAppendError: (m) => toast(m || t("Catalog request failed"), "error"),
  });

  if (!loggedIn) {
    return (
      <div className="mx-auto max-w-6xl space-y-5">
        <Header />
        <SignInGate title={t("Sign in to kino.watch to see what's new")} onSignIn={onSignIn} />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <Header />

      {/* Which chart, then which type — the only two knobs a chart accepts. */}
      <div className="flex flex-wrap gap-2">
        {CHARTS.map((c) => (
          <Chip key={c.kind} active={kind === c.kind} onClick={() => setKind(c.kind)}>
            {t(c.label)}
          </Chip>
        ))}
      </div>

      <div className="-mx-1 flex gap-2 overflow-x-auto px-1 pb-1">
        <TypeChip active={type === ALL_TYPES} icon={LayoutGrid} label={t("All")} onClick={() => setType(ALL_TYPES)} />
        {TYPES.map((c) => (
          <TypeChip key={c.key} active={type === c.type} icon={c.icon} label={t(c.label)} onClick={() => setType(c.type)} />
        ))}
      </div>

      {feed.error && feed.items.length === 0 ? (
        <CatalogError onRetry={feed.reload} onOpenSettings={onOpenSettings} />
      ) : feed.loading && feed.items.length === 0 ? (
        <SkeletonGrid />
      ) : feed.items.length === 0 ? (
        <EmptyState icon={<Flame className="h-6 w-6" />} title={t("This chart is empty right now")} />
      ) : (
        <ItemsGrid items={feed.items} onOpen={(it) => pushRoute({ page: "tops", itemId: it.id })} />
      )}

      {feed.loading && feed.items.length > 0 && <ListSpinner />}
      <div ref={feed.sentinelRef} className="h-1" />

      {detailId && (
        <TitleDetail
          id={detailId}
          onClose={() => dismiss({ page: "tops" })}
          // A "similar" pick swaps the card in place (one modal = one history
          // entry), so the X closes back to the grid, not through every card.
          onPick={(it) => replaceRoute({ page: "tops", itemId: it.id })}
        />
      )}
    </div>
  );
}

function Header() {
  const { t } = useI18n();
  return (
    <header>
      <h1 className="text-2xl font-bold text-slate-100">{t("What's new")}</h1>
      <p className="mt-1 text-sm text-slate-400">
        {t("kino.watch's own charts. To browse by genre, rating and year, use the Catalog.")}
      </p>
    </header>
  );
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`rounded-lg px-3 py-1.5 text-sm transition ${active ? "bg-gold-500/[0.14] text-gold-200" : "text-slate-400 hover:bg-white/[0.05] hover:text-slate-200"}`}
    >
      {children}
    </button>
  );
}

function TypeChip({
  active,
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`flex shrink-0 items-center gap-2 rounded-xl px-3 py-2 text-sm transition ${active ? "bg-gold-500/[0.14] text-gold-200" : "text-slate-400 hover:bg-white/[0.05] hover:text-slate-200"}`}
    >
      <Icon className="h-4 w-4" />
      {label}
    </button>
  );
}
