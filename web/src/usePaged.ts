import { useEffect, useRef, useState } from "react";
import { isNavigationAbort } from "./api";

// Every catalog-shaped list in the app — the catalog grid, collections, watch
// history, a bookmark folder — loads the same way: fetch page 1 when the source
// changes, append the next page when the user scrolls near the bottom, and tell
// the caller whether the *first* load failed (error panel) or a later append did
// (a toast, since results are already on screen). usePaged owns that loop once so
// the pages stay about layout.

// Снимок последнего показанного списка на страницу-«владельца» (ключ cache).
// Каталог размонтируется при переходе в другой раздел, и без этого возвращение
// к нему стирало найденное: поиск сбрасывался, сетка уезжала на первую
// страницу. Хранится ровно один снимок на владельца и только для последнего
// источника — это память о том, что человек только что смотрел, а не кэш.
interface Snapshot {
  key: string;
  items: unknown[];
  page: number;
  hasMore: boolean;
}
const snapshots = new Map<string, Snapshot>();

/** A page of results, as every list endpoint returns it. */
export interface PagedResult<T> {
  items: T[];
  /** Page the server actually served; defaults to the one we asked for. */
  page?: number;
  hasMore: boolean;
}

export interface Paged<T> {
  items: T[];
  loading: boolean;
  /** Last request failed. Only meaningful for a first load — see above. */
  error: boolean;
  hasMore: boolean;
  /** Attach to an empty element after the list to drive infinite scroll. */
  sentinelRef: React.RefObject<HTMLDivElement>;
  /** Append the next page. What the sentinel calls; safe to call by hand. */
  loadMore: () => void;
  /** Re-fetch from page 1 (the retry button). */
  reload: () => void;
}

export function usePaged<T>({
  enabled,
  sourceKey,
  load,
  onAppendError,
  cache,
}: {
  /** False parks the list empty — e.g. logged out, or another mode is showing. */
  enabled: boolean;
  /** Any change resets to page 1. Encode every input `load` closes over. */
  sourceKey: string;
  load: (page: number) => Promise<PagedResult<T>>;
  onAppendError?: (message: string) => void;
  /**
   * Имя владельца списка ("discover"). С ним показанное переживает уход на
   * другую страницу и возвращается без повторного запроса. Без него — прежнее
   * поведение: каждый заход грузит с первой страницы.
   */
  cache?: string;
}): Paged<T> {
  const [items, setItems] = useState<T[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const pageRef = useRef(1);

  // Sequence number of the newest request. Switching source (or scrolling on)
  // while a fetch is in flight would otherwise let the stale response land on
  // top of the fresh list; anything but the newest seq is dropped.
  const seqRef = useRef(0);

  // Rebuilt every render so it always closes over the current props; callers
  // reach it only through refs, so no dependency arrays to keep in sync.
  const loadRef = useRef<(reset: boolean) => void>(() => {});
  loadRef.current = async (reset: boolean) => {
    const next = reset ? 1 : pageRef.current + 1;
    const seq = ++seqRef.current;
    setLoading(true);
    if (reset) setError(false);
    try {
      const r = await load(next);
      if (seq !== seqRef.current) return;
      const page = r.page || next;
      setItems((prev) => {
        const merged = reset ? r.items : [...prev, ...r.items];
        // Запись идемпотентна, поэтому безопасна и при двойном вызове
        // апдейтера в StrictMode: тот же ключ, те же элементы.
        if (cache) snapshots.set(cache, { key: sourceKey, items: merged, page, hasMore: r.hasMore });
        return merged;
      });
      setHasMore(r.hasMore);
      pageRef.current = page;
    } catch (e: any) {
      if (seq !== seqRef.current) return;
      // Leaving the page cancels its in-flight loads. That is not a failed load:
      // flipping to the error panel would flash a retry prompt on the way out.
      if (isNavigationAbort(e)) return;
      setError(true);
      // A failed first load is rendered as a panel by the caller; a failed
      // append keeps what's on screen, so it only earns a toast.
      if (!reset) onAppendError?.(e?.message || "");
    } finally {
      if (seq === seqRef.current) setLoading(false);
    }
  };

  const moreRef = useRef<(manual?: boolean) => void>(() => {});
  moreRef.current = (manual = false) => {
    // A failed append parks auto-paging: the sentinel is usually still in view
    // after a failure, and re-firing on every loading flip turned one
    // unreachable upstream into an unbounded request loop with an error toast
    // per iteration. A deliberate loadMore (retry) or a source change resumes.
    if (error && !manual) return;
    if (hasMore && !loading) {
      if (error) setError(false);
      loadRef.current(false);
    }
  };

  // Reset + load whenever the data source changes. Clearing even when disabled
  // keeps a stale list from flashing when the source comes back.
  useEffect(() => {
    seqRef.current++;
    const snap = cache ? snapshots.get(cache) : undefined;
    if (enabled && snap && snap.key === sourceKey) {
      // Возврат на ту же страницу с тем же источником: показываем ровно то, что
      // человек оставил, вместо мигания пустой сеткой и повторного запроса.
      pageRef.current = snap.page;
      setItems(snap.items as T[]);
      setHasMore(snap.hasMore);
      setError(false);
      return;
    }
    pageRef.current = 1;
    setItems([]);
    setHasMore(false);
    setError(false);
    if (enabled) loadRef.current(true);
  }, [enabled, sourceKey, cache]);

  // Infinite scroll. An IntersectionObserver only fires on an off→on-screen
  // transition, and a short page (wide collection cards, a folder with four
  // titles) can leave the sentinel permanently in view — it would never re-fire
  // and paging would stall. Re-observing after every append re-checks the
  // current intersection, so paging continues until the sentinel scrolls off.
  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !hasMore) return;
    const ob = new IntersectionObserver((es) => es[0]?.isIntersecting && moreRef.current(), {
      rootMargin: "400px",
    });
    ob.observe(el);
    return () => ob.disconnect();
  }, [hasMore, loading, items.length]);

  return {
    items,
    loading,
    error,
    hasMore,
    sentinelRef,
    loadMore: () => moreRef.current(true),
    reload: () => loadRef.current(true),
  };
}
