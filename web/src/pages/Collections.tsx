import { useCallback, useState } from "react";
import { LayoutGrid } from "lucide-react";
import { api, imgURL, type DiscoverCollection, type DiscoverItem } from "../api";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { collectionTitle, dismiss, pushRoute, rememberCollectionTitle, replaceRoute, useRoute } from "../router";
import { usePaged } from "../usePaged";
import { EmptyState } from "../components/ui";
import { CatalogError, ItemsGrid, ListSpinner, SignInGate, SkeletonGrid } from "../components/catalog";
import { TitleDetail } from "../components/TitleDetail";

// Collections (подборки) are kino.watch's own curated lists. They were a tab
// inside the Catalog, which meant the search box and the filter panel sat above
// a grid they had no effect on. As their own rail entry the page is just the
// list and one open collection, both addressable:
//   "#/collections"        — the list, sorted by the chips below;
//   "#/collections/c/<id>" — one collection's titles.
type Sort = "new" | "popular" | "watched";

const SORT_PARAM: Record<Sort, string> = {
  new: "created-",
  popular: "views-",
  watched: "watchers-",
};

export function CollectionsPage({
  onSignIn,
  onOpenSettings,
}: {
  onSignIn: () => void;
  onOpenSettings: () => void;
}) {
  const { kpauth, toast } = useApp();
  const { t } = useI18n();
  const loggedIn = kpauth.loggedIn;

  const route = useRoute();
  const collectionId = route.collectionId ?? null;
  const detailId = route.itemId ?? null;
  const appendFailed = (m: string) => toast(m || t("Catalog request failed"), "error");

  const [sort, setSort] = useState<Sort>("new");
  // Re-sorting while a collection is open would silently rebuild the list behind
  // it, so close the collection first — the user asked to see the other list.
  const selectSort = (s: Sort) => {
    setSort(s);
    if (collectionId) replaceRoute({ page: "collections" });
  };

  const loadCollections = useCallback(
    async (p: number) => {
      const r = await api.discoverCollections(SORT_PARAM[sort], p);
      // This endpoint reports neither a page count nor a "has more" flag: stop on
      // the first empty page, and cap the walk so we can't page forever.
      return { items: r.items || [], hasMore: (r.items?.length ?? 0) > 0 && p < 25 };
    },
    [sort],
  );
  const cols = usePaged<DiscoverCollection>({
    enabled: loggedIn && !collectionId,
    sourceKey: sort,
    load: loadCollections,
    onAppendError: appendFailed,
  });

  const loadItems = useCallback((p: number) => api.discoverCollection(collectionId || "", p), [collectionId]);
  const feed = usePaged<DiscoverItem>({
    enabled: loggedIn && !!collectionId,
    sourceKey: collectionId || "",
    load: loadItems,
    onAppendError: appendFailed,
  });

  // Exactly one of the two feeds is on screen; it owns the skeleton, the error
  // panel, the "loading more" spinner and the infinite-scroll sentinel.
  const list = collectionId ? feed : cols;

  const openCollection = (c: DiscoverCollection) => {
    // The per-collection endpoint returns titles only, so stash the name for the
    // breadcrumb — it has to survive a reload of "#/collections/c/<id>".
    rememberCollectionTitle(c.id, c.title);
    pushRoute({ page: "collections", collectionId: c.id });
  };

  if (!loggedIn) {
    return (
      <div className="mx-auto max-w-6xl space-y-5">
        <Header />
        <SignInGate title={t("Sign in to kino.watch to browse collections")} onSignIn={onSignIn} />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <Header />

      {collectionId ? (
        <div className="flex items-center gap-2 text-sm">
          <button className="text-gold-300 hover:underline" onClick={() => dismiss({ page: "collections" })}>
            ← {t("Collections")}
          </button>
          <span className="text-slate-500">/</span>
          <span className="font-medium text-slate-200">{collectionTitle(collectionId) || t("Collection")}</span>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          <SortChip active={sort === "new"} onClick={() => selectSort("new")}>{t("New")}</SortChip>
          <SortChip active={sort === "popular"} onClick={() => selectSort("popular")}>{t("Popular")}</SortChip>
          <SortChip active={sort === "watched"} onClick={() => selectSort("watched")}>{t("Most watched")}</SortChip>
        </div>
      )}

      {list.error && list.items.length === 0 ? (
        <CatalogError onRetry={list.reload} onOpenSettings={onOpenSettings} />
      ) : list.loading && list.items.length === 0 ? (
        <SkeletonGrid wide={!collectionId} />
      ) : list.items.length === 0 ? (
        <EmptyState
          icon={<LayoutGrid className="h-6 w-6" />}
          title={collectionId ? t("This collection is empty") : t("No collections found")}
        />
      ) : collectionId ? (
        <ItemsGrid
          items={feed.items}
          onOpen={(it) => pushRoute({ page: "collections", collectionId, itemId: it.id })}
        />
      ) : (
        <CollectionsGrid collections={cols.items} onOpen={openCollection} />
      )}

      {list.loading && list.items.length > 0 && <ListSpinner />}
      <div ref={list.sentinelRef} className="h-1" />

      {detailId && (
        <TitleDetail
          id={detailId}
          onClose={() => dismiss({ page: "collections", collectionId: collectionId ?? undefined })}
          // A "similar" pick swaps the card in place (one modal = one history
          // entry), so the X closes back to the list, not through every card.
          onPick={(it) =>
            replaceRoute({ page: "collections", collectionId: collectionId ?? undefined, itemId: it.id })
          }
        />
      )}
    </div>
  );
}

function Header() {
  const { t } = useI18n();
  return (
    <header>
      <h1 className="text-2xl font-bold text-slate-100">{t("Collections")}</h1>
      <p className="mt-1 text-sm text-slate-400">
        {t("kino.watch's own curated lists — open one and download straight from it.")}
      </p>
    </header>
  );
}

function SortChip({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`rounded-lg px-3 py-1.5 text-sm transition ${active ? "bg-gold-500/[0.14] text-gold-200" : "text-slate-400 hover:bg-white/[0.05] hover:text-slate-200"}`}
    >
      {children}
    </button>
  );
}

// A collection card is a 16:9 banner — the API gives collections a wide poster
// rather than the 2:3 one a title carries.
function CollectionsGrid({
  collections,
  onOpen,
}: {
  collections: DiscoverCollection[];
  onOpen: (c: DiscoverCollection) => void;
}) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
      {collections.map((c) => (
        <button key={c.id} onClick={() => onOpen(c)} className="group text-left" title={c.title}>
          <div className="relative overflow-hidden rounded-xl bg-gradient-to-br from-ink-700 to-ink-850">
            <img
              src={imgURL(c.poster)}
              alt={c.title}
              loading="lazy"
              className="aspect-video w-full object-cover transition duration-200 group-hover:scale-[1.03]"
              onError={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = "hidden")}
            />
          </div>
          <p className="mt-1.5 truncate text-sm font-medium text-slate-200">{c.title}</p>
        </button>
      ))}
    </div>
  );
}
