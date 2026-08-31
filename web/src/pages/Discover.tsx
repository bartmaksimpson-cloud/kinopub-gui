import { useCallback, useEffect, useMemo, useState } from "react";
import { LayoutGrid, Search, type LucideIcon } from "lucide-react";
import { api, type DiscoverItem, type ItemsQuery, type NamedRef } from "../api";
import { CATEGORIES, categoryByKey } from "../categories";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { dismiss, pushRoute, replaceRoute, useRoute } from "../router";
import { usePaged } from "../usePaged";
import { CatalogError, ItemsGrid, ListSpinner, SignInGate, SkeletonGrid } from "../components/catalog";
import { TitleDetail } from "../components/TitleDetail";
import {
  FilterPanel,
  YEAR_MAX,
  YEAR_MIN,
  defaultFilter,
  type FilterState,
} from "../components/FilterPanel";

// This page used to carry a tab row — Collections, I'm watching, Bookmarks,
// History — above the search box and the filter panel, neither of which applied
// to any of those four. Each is now its own rail entry (pages/Collections.tsx,
// Watching.tsx, Bookmarks.tsx, History.tsx), which leaves the Catalog with one
// job: find a title in kino.watch's library, by query or by filter.

function filterToQuery(f: FilterState): ItemsQuery {
  // The category fixes the content type (and, for Anime/Sport, a spanning genre);
  // within a type category the user's chosen sub-genre wins.
  const cat = categoryByKey(f.category);
  return {
    type: cat?.type || undefined,
    genre: cat?.genre || f.genre || undefined,
    country: f.country || undefined,
    sort: f.sort || undefined,
    yearFrom: f.yearFrom > YEAR_MIN ? f.yearFrom : undefined,
    yearTo: f.yearTo < YEAR_MAX ? f.yearTo : undefined,
    kpFrom: f.kpFrom > 0 ? f.kpFrom : undefined,
    kpTo: f.kpTo < 10 ? f.kpTo : undefined,
    imdbFrom: f.imdbFrom > 0 ? f.imdbFrom : undefined,
    imdbTo: f.imdbTo < 10 ? f.imdbTo : undefined,
    ac3: f.ac3 || undefined,
    subtitles: f.subtitles || undefined,
  };
}

// Two escape hatches, two destinations: signing in lives on the Profile page,
// while the proxy that unblocks kino.watch lives in Settings.
export function DiscoverPage({
  onSignIn,
  onOpenSettings,
}: {
  onSignIn: () => void;
  onOpenSettings: () => void;
}) {
  const { kpauth, toast } = useApp();
  const { t } = useI18n();
  const loggedIn = kpauth.loggedIn;

  // The open title card lives in the URL hash (the single source of truth), so
  // it survives a reload and browser back/forward.
  const detailId = useRoute().itemId ?? null;

  // Live search (debounced).
  const [search, setSearch] = useState("");
  const [committedSearch, setCommittedSearch] = useState("");
  useEffect(() => {
    const q = search.trim();
    const id = window.setTimeout(() => setCommittedSearch(q.length >= 2 ? q : ""), 350);
    return () => window.clearTimeout(id);
  }, [search]);

  // Filter (debounced so dragging sliders doesn't spam the API).
  const [filter, setFilter] = useState<FilterState>(defaultFilter);
  const [debFilter, setDebFilter] = useState<FilterState>(filter);
  useEffect(() => {
    const id = window.setTimeout(() => setDebFilter(filter), 400);
    return () => window.clearTimeout(id);
  }, [filter]);

  const [genres, setGenres] = useState<NamedRef[]>([]);

  const searching = committedSearch.length >= 2;

  // The active category and the genre group to offer as sub-genres beneath it.
  // Genre-based categories (Anime/Sport) are themselves a genre, so they carry no
  // genreType and show no sub-genre row.
  const activeCat = categoryByKey(filter.category);
  const genreType = activeCat?.genreType ?? "";

  // Load the selected category's genres (kept clean by querying per type; the
  // unfiltered endpoint mixes in music/docu junk).
  useEffect(() => {
    if (!loggedIn || !genreType) {
      setGenres([]);
      return;
    }
    api.discoverGenres(genreType).then((r) => setGenres(r.items || [])).catch(() => setGenres([]));
  }, [loggedIn, genreType]);

  const load = useCallback(
    (p: number) =>
      searching
        ? api.discoverSearch(committedSearch, p)
        : api.discoverItems({ ...filterToQuery(debFilter), page: p }),
    [searching, committedSearch, debFilter],
  );
  const sourceKey = useMemo(
    () => JSON.stringify([committedSearch, debFilter]),
    [committedSearch, debFilter],
  );
  const feed = usePaged<DiscoverItem>({
    enabled: loggedIn,
    sourceKey,
    load,
    onAppendError: (m) => toast(m || t("Catalog request failed"), "error"),
  });

  // Picking a category (the catalog's spine) clears any sub-genre, since genres
  // are scoped to the new category, and drops the search that would override it.
  const selectCategory = (key: string) => {
    setFilter((f) => ({ ...f, category: key, genre: "" }));
    setSearch("");
  };
  const selectGenre = (id: string) => setFilter((f) => ({ ...f, genre: id }));

  // Touching the filter brings the user to the results it controls.
  const onFilterChange = (f: FilterState) => {
    setFilter(f);
    setSearch("");
  };

  if (!loggedIn) {
    return (
      <div className="mx-auto max-w-6xl">
        <Header />
        <SignInGate title={t("Sign in to kino.watch to browse the catalog")} onSignIn={onSignIn} />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <Header />

      {/* Live search */}
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
        <input
          className="input pl-9"
          placeholder={t("Search films and series on kino.watch…")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        {searching && (
          <button
            onClick={() => setSearch("")}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-slate-500 hover:text-slate-300"
          >
            {t("Clear")}
          </button>
        )}
      </div>

      {searching ? (
        // kino.watch's title search is a dedicated endpoint that can't be narrowed
        // by category/genre/filters, so hide those controls while a query is
        // active (they'd only fight the search) and tell the user how to browse
        // by genre instead — clear the search.
        <p className="px-1 text-xs text-slate-500">
          {t("Showing title matches. Clear the search to browse by category, genre and filters.")}
        </p>
      ) : (
        <>
          {/* The category bar (mirrors kino.watch's category sidebar), then the
              selected category's genres. The Fresh/Hot/Popular chips that used to
              close this page are kino.watch's own charts, which take neither a
              genre nor a filter — they now live on their own "What's new" page. */}
          <div className="-mx-1 flex gap-2 overflow-x-auto px-1 pb-1">
            <CategoryChip active={!filter.category} icon={LayoutGrid} label={t("All")} onClick={() => selectCategory("")} />
            {CATEGORIES.map((c) => (
              <CategoryChip
                key={c.key}
                active={filter.category === c.key}
                icon={c.icon}
                label={t(c.label)}
                onClick={() => selectCategory(c.key)}
              />
            ))}
          </div>

          {genreType && genres.length > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <span className="mr-1 text-xs font-medium text-slate-500">{t("Genres")}:</span>
              <GenreChip active={!filter.genre} onClick={() => selectGenre("")}>{t("All genres")}</GenreChip>
              {genres.map((g) => (
                <GenreChip key={g.id} active={filter.genre === g.id} onClick={() => selectGenre(g.id)}>
                  {g.title}
                </GenreChip>
              ))}
            </div>
          )}

          {/* Fine-grained narrowing comes last: the user picks the shelf
              (category → genre) first, then the conditions on top of it. */}
          <FilterPanel value={filter} onChange={onFilterChange} />
        </>
      )}

      {/* Content */}
      {feed.error && feed.items.length === 0 ? (
        <CatalogError onRetry={feed.reload} onOpenSettings={onOpenSettings} />
      ) : feed.loading && feed.items.length === 0 ? (
        <SkeletonGrid />
      ) : feed.items.length === 0 ? (
        <p className="py-10 text-center text-sm text-slate-500">{t("Nothing found.")}</p>
      ) : (
        <ItemsGrid items={feed.items} onOpen={(it) => pushRoute({ page: "discover", itemId: it.id })} />
      )}

      {/* Loader for an in-flight append, then the infinite-scroll sentinel. */}
      {feed.loading && feed.items.length > 0 && <ListSpinner />}
      <div ref={feed.sentinelRef} className="h-1" />

      {detailId && (
        <TitleDetail
          id={detailId}
          onClose={() => dismiss({ page: "discover" })}
          // A "similar" pick swaps the card in place (one modal = one history
          // entry), so the X button closes cleanly instead of stepping back
          // through every card visited.
          onPick={(it) => replaceRoute({ page: "discover", itemId: it.id })}
        />
      )}
    </div>
  );
}

function Header() {
  const { t } = useI18n();
  return (
    <header>
      <h1 className="text-2xl font-bold text-slate-100">{t("Catalog")}</h1>
      <p className="mt-1 text-sm text-slate-400">
        {t("Search kino.watch by title, or narrow the whole library down by category, genre and rating.")}
      </p>
    </header>
  );
}

// CategoryChip is a content-category pill with its icon, used in the catalog's
// top category bar (mirrors kino.watch's category sidebar). shrink-0 keeps chips
// from squashing inside the horizontally scrollable bar.
function CategoryChip({
  active,
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`flex shrink-0 items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition ${active ? "bg-gold-500/[0.14] text-gold-200" : "text-slate-400 hover:bg-white/[0.05] hover:text-slate-200"}`}
    >
      <Icon className="h-4 w-4" />
      {label}
    </button>
  );
}

// GenreChip is a smaller pill for the selected category's sub-genres.
function GenreChip({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`rounded-full px-2.5 py-1 text-xs transition ${active ? "bg-gold-500/[0.14] text-gold-200" : "bg-white/[0.04] text-slate-400 hover:bg-white/[0.08] hover:text-slate-200"}`}
    >
      {children}
    </button>
  );
}
