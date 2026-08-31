import { useCallback } from "react";
import { Bookmark } from "lucide-react";
import { api, type DiscoverBookmark, type DiscoverItem } from "../api";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { bookmarkTitle, dismiss, pushRoute, rememberBookmarkTitle, replaceRoute, useRoute } from "../router";
import { usePaged } from "../usePaged";
import { EmptyState } from "../components/ui";
import { CatalogError, ItemsGrid, ListSpinner, SignInGate, SkeletonGrid } from "../components/catalog";
import { TitleDetail } from "../components/TitleDetail";

// Bookmarks were a chip inside the Catalog; they are now a rail entry of their
// own. The page has two states, both addressable: the folder list ("#/bookmarks")
// and one open folder ("#/bookmarks/b/<id>"). Links minted while bookmarks still
// lived under the Catalog ("#/discover/b/<id>") land here too — App maps them.
export function BookmarksPage({
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
  const folderId = route.bookmarkId ?? null;
  const detailId = route.itemId ?? null;
  const appendFailed = (m: string) => toast(m || t("Catalog request failed"), "error");

  // Folders come back as one unpaginated list; a folder's titles are paged.
  const loadFolders = useCallback(
    async () => ({ items: (await api.discoverBookmarks()).items || [], hasMore: false }),
    [],
  );
  const folders = usePaged<DiscoverBookmark>({
    enabled: loggedIn && !folderId,
    sourceKey: "folders",
    load: loadFolders,
    onAppendError: appendFailed,
  });

  const loadItems = useCallback((p: number) => api.discoverBookmark(folderId || "", p), [folderId]);
  const feed = usePaged<DiscoverItem>({
    enabled: loggedIn && !!folderId,
    sourceKey: folderId || "",
    load: loadItems,
    onAppendError: appendFailed,
  });

  // Whichever of the two lists is on screen drives the header, the loaders and
  // the infinite-scroll sentinel.
  const list = folderId ? feed : folders;

  const openFolder = (b: DiscoverBookmark) => {
    // The per-folder endpoint returns titles only, so stash the name for the
    // breadcrumb — it has to survive a reload of "#/bookmarks/b/<id>".
    rememberBookmarkTitle(b.id, b.title);
    pushRoute({ page: "bookmarks", bookmarkId: b.id });
  };

  if (!loggedIn) {
    return (
      <div className="mx-auto max-w-6xl space-y-5">
        <Header />
        <SignInGate title={t("Sign in to kino.watch to see your bookmarks")} onSignIn={onSignIn} />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <Header />

      {folderId && (
        <div className="flex items-center gap-2 text-sm">
          <button className="text-gold-300 hover:underline" onClick={() => dismiss({ page: "bookmarks" })}>
            ← {t("Bookmarks")}
          </button>
          <span className="text-slate-500">/</span>
          <span className="font-medium text-slate-200">{bookmarkTitle(folderId) || t("Folder")}</span>
        </div>
      )}

      {list.error && list.items.length === 0 ? (
        <CatalogError onRetry={list.reload} onOpenSettings={onOpenSettings} />
      ) : list.loading && list.items.length === 0 ? (
        <SkeletonGrid wide={!folderId} />
      ) : list.items.length === 0 ? (
        <EmptyState
          icon={<Bookmark className="h-6 w-6" />}
          title={folderId ? t("This folder is empty") : t("No bookmarks yet")}
          hint={folderId ? undefined : t("Folders you create on kino.watch show up here, ready to download from.")}
        />
      ) : folderId ? (
        <ItemsGrid
          items={feed.items}
          onOpen={(it) => pushRoute({ page: "bookmarks", bookmarkId: folderId, itemId: it.id })}
        />
      ) : (
        <FoldersGrid folders={folders.items} onOpen={openFolder} />
      )}

      {list.loading && list.items.length > 0 && <ListSpinner />}
      <div ref={list.sentinelRef} className="h-1" />

      {detailId && (
        <TitleDetail
          id={detailId}
          onClose={() => dismiss({ page: "bookmarks", bookmarkId: folderId ?? undefined })}
          // A "similar" pick swaps the card in place (one modal = one history
          // entry), so the X closes back to the folder, not through every card.
          onPick={(it) =>
            replaceRoute({ page: "bookmarks", bookmarkId: folderId ?? undefined, itemId: it.id })
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
      <h1 className="text-2xl font-bold text-slate-100">{t("Bookmarks")}</h1>
      <p className="mt-1 text-sm text-slate-400">
        {t("Your kino.watch bookmark folders — open one and download straight from it.")}
      </p>
    </header>
  );
}

// FoldersGrid lists the account's bookmark folders. A folder has no poster (the
// API returns only id/title/count), so each card is an icon tile.
function FoldersGrid({ folders, onOpen }: { folders: DiscoverBookmark[]; onOpen: (b: DiscoverBookmark) => void }) {
  const { t } = useI18n();
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
      {folders.map((b) => (
        <button key={b.id} onClick={() => onOpen(b)} className="group text-left" title={b.title}>
          <div className="flex aspect-video w-full items-center justify-center rounded-xl bg-gradient-to-br from-ink-700 to-ink-850 transition duration-200 group-hover:from-ink-600 group-hover:to-ink-800">
            <Bookmark className="h-8 w-8 text-gold-300/70" />
          </div>
          <p className="mt-1.5 truncate text-sm font-medium text-slate-200">{b.title}</p>
          <p className="text-[11px] text-slate-500">{t("{n} titles", { n: b.count })}</p>
        </button>
      ))}
    </div>
  );
}
