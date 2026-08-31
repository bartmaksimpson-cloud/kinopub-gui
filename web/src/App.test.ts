import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { legacyPage, useSettled } from "./App";
import { parseHash } from "./router";

// Hashes minted by older versions of the app are still out there — in the
// browser's own history, in anything the user bookmarked. legacyPage is what
// keeps them landing somewhere sensible after a page moved.
describe("legacyPage", () => {
  it("sends the retired queue to the Library that absorbed it", () => {
    expect(legacyPage("queue")).toBe("library");
  });

  it("opens a Catalog-era bookmark folder on the Bookmarks page", () => {
    expect(legacyPage("discover", "42")).toBe("bookmarks");
  });

  it("opens a Catalog-era collection on the Collections page", () => {
    expect(legacyPage("discover", undefined, "678")).toBe("collections");
  });

  it("leaves a bare Catalog link alone", () => {
    expect(legacyPage("discover")).toBe("discover");
    // An open title card is still the Catalog's own.
    expect(legacyPage("discover", undefined, undefined)).toBe("discover");
  });

  it("passes current pages through untouched", () => {
    for (const p of ["collections", "watching", "bookmarks", "history", "library", "settings"] as const) {
      expect(legacyPage(p)).toBe(p);
    }
  });

  it("maps a whole legacy hash end to end", () => {
    const r = parseHash("#/discover/c/678/i/12345");
    expect(legacyPage(r.page, r.bookmarkId, r.collectionId)).toBe("collections");
    // The ids survive, so the page opens the right collection and card.
    expect(r.collectionId).toBe("678");
    expect(r.itemId).toBe("12345");
  });
});

// A page load starts with the SSE link down, and it comes back up in a few
// hundred ms. Reporting that verbatim flashed a "Reconnecting to app…" banner on
// every reload, so the flag has to hold before the UI believes it.
describe("useSettled", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("stays false while the flag flips back before the delay", () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(({ v }) => useSettled(v, 3000), {
      initialProps: { v: true },
    });
    expect(result.current).toBe(false);
    act(() => vi.advanceTimersByTime(2000));
    rerender({ v: false }); // link came back in time — no banner ever shown
    act(() => vi.advanceTimersByTime(5000));
    expect(result.current).toBe(false);
  });

  it("reports a flag that holds for the whole delay", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useSettled(true, 3000));
    act(() => vi.advanceTimersByTime(2999));
    expect(result.current).toBe(false);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current).toBe(true);
  });

  it("clears immediately once the flag drops", () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(({ v }) => useSettled(v, 3000), {
      initialProps: { v: true },
    });
    act(() => vi.advanceTimersByTime(3000));
    expect(result.current).toBe(true);
    rerender({ v: false });
    expect(result.current).toBe(false);
  });
});
