import { useEffect, useRef, useState } from "react";
import { isNavigationAbort } from "./api";

// Every catalog-shaped list in the app — the catalog grid, collections, watch
// history, a bookmark folder — loads the same way: fetch page 1 when the source
// changes, append the next page when the user scrolls near the bottom, and tell
// the caller whether the *first* load failed (error panel) or a later append did
// (a toast, since results are already on screen). usePaged owns that loop once so
// the pages stay about layout.

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
}: {
  /** False parks the list empty — e.g. logged out, or another mode is showing. */
  enabled: boolean;
  /** Any change resets to page 1. Encode every input `load` closes over. */
  sourceKey: string;
  load: (page: number) => Promise<PagedResult<T>>;
  onAppendError?: (message: string) => void;
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
      setItems((prev) => (reset ? r.items : [...prev, ...r.items]));
      setHasMore(r.hasMore);
      pageRef.current = r.page || next;
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
    pageRef.current = 1;
    setItems([]);
    setHasMore(false);
    setError(false);
    if (enabled) loadRef.current(true);
  }, [enabled, sourceKey]);

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
