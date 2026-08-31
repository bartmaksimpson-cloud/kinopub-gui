import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { usePaged, type PagedResult } from "./usePaged";
import { NAV_ABORT_MESSAGE } from "./api";

// A loader that hands back one item per page ("p1", "p2", …) and records which
// pages were asked for.
function counting(pages: number) {
  const asked: number[] = [];
  const load = vi.fn(async (p: number): Promise<PagedResult<string>> => {
    asked.push(p);
    return { items: [`p${p}`], hasMore: p < pages };
  });
  return { asked, load };
}

// A loader whose every call is resolved by hand, so a test can land two
// responses out of order.
function deferred<T>() {
  const resolvers: ((r: PagedResult<T>) => void)[] = [];
  const load = vi.fn(
    (_p: number) => new Promise<PagedResult<T>>((res) => resolvers.push(res)),
  );
  return { resolvers, load };
}

describe("usePaged", () => {
  it("loads the first page on mount", async () => {
    const { asked, load } = counting(3);
    const { result } = renderHook(() => usePaged({ enabled: true, sourceKey: "a", load }));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual(["p1"]);
    expect(result.current.hasMore).toBe(true);
    expect(result.current.error).toBe(false);
    expect(asked).toEqual([1]);
  });

  it("stays empty and issues no request while disabled", async () => {
    const { load } = counting(3);
    const { result } = renderHook(() => usePaged({ enabled: false, sourceKey: "a", load }));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual([]);
    expect(load).not.toHaveBeenCalled();
  });

  it("loads page 1 once it becomes enabled", async () => {
    const { asked, load } = counting(3);
    const { result, rerender } = renderHook(
      ({ enabled }) => usePaged({ enabled, sourceKey: "a", load }),
      { initialProps: { enabled: false } },
    );
    expect(load).not.toHaveBeenCalled();

    rerender({ enabled: true });
    await waitFor(() => expect(result.current.items).toEqual(["p1"]));
    expect(asked).toEqual([1]);
  });

  it("loadMore appends the next page", async () => {
    const { asked, load } = counting(3);
    const { result } = renderHook(() => usePaged({ enabled: true, sourceKey: "a", load }));
    await waitFor(() => expect(result.current.items).toEqual(["p1"]));

    await act(async () => result.current.loadMore());
    await waitFor(() => expect(result.current.items).toEqual(["p1", "p2"]));

    await act(async () => result.current.loadMore());
    await waitFor(() => expect(result.current.items).toEqual(["p1", "p2", "p3"]));
    // Page 3 is the last one, so the sentinel stops asking.
    expect(result.current.hasMore).toBe(false);
    expect(asked).toEqual([1, 2, 3]);
  });

  it("loadMore is a no-op once the last page is in", async () => {
    const { asked, load } = counting(1);
    const { result } = renderHook(() => usePaged({ enabled: true, sourceKey: "a", load }));
    await waitFor(() => expect(result.current.hasMore).toBe(false));

    await act(async () => result.current.loadMore());
    expect(asked).toEqual([1]);
  });

  it("a new sourceKey resets to page 1", async () => {
    const { asked, load } = counting(3);
    const { result, rerender } = renderHook(
      ({ sourceKey }) => usePaged({ enabled: true, sourceKey, load }),
      { initialProps: { sourceKey: "a" } },
    );
    await waitFor(() => expect(result.current.items).toEqual(["p1"]));
    await act(async () => result.current.loadMore());
    await waitFor(() => expect(result.current.items).toEqual(["p1", "p2"]));

    rerender({ sourceKey: "b" });
    await waitFor(() => expect(result.current.items).toEqual(["p1"]));
    expect(asked).toEqual([1, 2, 1]);
  });

  it("reload re-fetches from page 1", async () => {
    const { asked, load } = counting(3);
    const { result } = renderHook(() => usePaged({ enabled: true, sourceKey: "a", load }));
    await waitFor(() => expect(result.current.items).toEqual(["p1"]));
    await act(async () => result.current.loadMore());
    await waitFor(() => expect(result.current.items).toEqual(["p1", "p2"]));

    await act(async () => result.current.reload());
    await waitFor(() => expect(result.current.items).toEqual(["p1"]));
    expect(asked).toEqual([1, 2, 1]);
  });

  it("a failed first load raises error without toasting", async () => {
    const onAppendError = vi.fn();
    const load = vi.fn(async () => {
      throw new Error("offline");
    });
    const { result } = renderHook(() =>
      usePaged({ enabled: true, sourceKey: "a", load, onAppendError }),
    );

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.items).toEqual([]);
    // The caller renders a panel for this one; a toast would be noise.
    expect(onAppendError).not.toHaveBeenCalled();
  });

  // Navigating away cancels the page's in-flight loads. That is not a failed
  // load: flashing the retry panel on the way out would be wrong, and the toast
  // would land on the page the user just moved to.
  it("a load cancelled by navigation raises neither the error panel nor a toast", async () => {
    const onAppendError = vi.fn();
    const load = vi.fn(async () => {
      throw new Error(NAV_ABORT_MESSAGE);
    });
    const { result } = renderHook(() =>
      usePaged({ enabled: true, sourceKey: "a", load, onAppendError }),
    );

    await waitFor(() => expect(load).toHaveBeenCalled());
    expect(result.current.error).toBe(false);
    expect(onAppendError).not.toHaveBeenCalled();
  });

  it("a failed append keeps the list and reports the message", async () => {
    const onAppendError = vi.fn();
    const load = vi.fn(async (p: number): Promise<PagedResult<string>> => {
      if (p > 1) throw new Error("boom");
      return { items: ["p1"], hasMore: true };
    });
    const { result } = renderHook(() =>
      usePaged({ enabled: true, sourceKey: "a", load, onAppendError }),
    );
    await waitFor(() => expect(result.current.items).toEqual(["p1"]));

    await act(async () => result.current.loadMore());
    await waitFor(() => expect(onAppendError).toHaveBeenCalledWith("boom"));
    expect(result.current.items).toEqual(["p1"]);
  });

  it("drops a stale response that lands after a source switch", async () => {
    const { resolvers, load } = deferred<string>();
    const { result, rerender } = renderHook(
      ({ sourceKey }) => usePaged({ enabled: true, sourceKey, load }),
      { initialProps: { sourceKey: "a" } },
    );
    expect(resolvers).toHaveLength(1);

    // Switch source while the first request is still in flight, then let the
    // *old* one resolve last. Without a sequence guard it would overwrite the
    // fresh list.
    rerender({ sourceKey: "b" });
    await waitFor(() => expect(resolvers).toHaveLength(2));
    await act(async () => {
      resolvers[1]({ items: ["fresh"], hasMore: false });
      resolvers[0]({ items: ["stale"], hasMore: true });
    });

    expect(result.current.items).toEqual(["fresh"]);
    expect(result.current.hasMore).toBe(false);
    expect(result.current.loading).toBe(false);
  });
});
