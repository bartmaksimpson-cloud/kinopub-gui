import { useEffect, useState } from "react";
import { abortPendingRequests } from "./api";

// A single hash-based router is the source of truth for the *entire* navigable
// view: the page, plus any open collection and title card. Encoding the card in
// the URL (e.g. "#/discover/c/678/i/12345") is what makes a card a real,
// shareable location that survives a page reload and browser back/forward —
// instead of living only in React state that resets on refresh.

export type Page =
  | "tops"
  | "discover"
  | "download"
  | "queue"
  | "library"
  | "collections"
  | "watching"
  | "bookmarks"
  | "history"
  | "doctor"
  | "settings"
  | "profile";

// Pages that own a nav-rail entry ("settings" among them) plus the two reached
// only from inside the app: advanced "download" and "profile" (opened from the
// sidebar account card). Listed so a bare legacy hash like "#queue" — or a typo
// — still resolves to a valid page.
export const PAGES: Page[] = [
  "tops",
  "discover",
  "download",
  "queue",
  "library",
  "collections",
  "watching",
  "bookmarks",
  "history",
  "doctor",
  "settings",
  "profile",
];

export interface Route {
  page: Page;
  // The open collection (подборка), when browsing one.
  collectionId?: string;
  // The open bookmark folder (папка закладок), when browsing one.
  bookmarkId?: string;
  // The open title card (карточка).
  itemId?: string;
}

// parseHash turns the URL fragment into a Route. The grammar is
// "/<page>[/c/<collectionId>][/b/<bookmarkId>][/i/<itemId>]"; a leading "#",
// "#/" or "/" and any bare legacy form ("#discover") are all accepted.
export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#/, "").replace(/^\//, "");
  const segs = raw.split("/").filter(Boolean);
  const page = (PAGES as string[]).includes(segs[0]) ? (segs[0] as Page) : "discover";
  const route: Route = { page };
  for (let i = 1; i < segs.length; i += 2) {
    const key = segs[i];
    const val = segs[i + 1];
    if (val === undefined) break;
    const decoded = decodeURIComponent(val);
    if (key === "c") route.collectionId = decoded;
    else if (key === "b") route.bookmarkId = decoded;
    else if (key === "i") route.itemId = decoded;
  }
  return route;
}

// buildHash renders a Route back into a fragment (without the leading "#").
export function buildHash(r: Route): string {
  let s = "/" + r.page;
  if (r.collectionId) s += "/c/" + encodeURIComponent(r.collectionId);
  if (r.bookmarkId) s += "/b/" + encodeURIComponent(r.bookmarkId);
  if (r.itemId) s += "/i/" + encodeURIComponent(r.itemId);
  return s;
}

export function readRoute(): Route {
  return parseHash(window.location.hash);
}

// legacyPage maps hashes minted by older versions onto the page that now owns
// that view. Kept pure and separate so it can be tested without rendering. It
// lives here (not in App.tsx) because pushRoute must compare RENDERED pages:
// "#/queue" renders the Library, and treating it as a different page made
// clicking the Library nav entry abort the Library's own in-flight requests.
export function legacyPage(page: Page, bookmarkId?: string, collectionId?: string): Page {
  if (page === "queue") return "library";
  if (page === "discover" && bookmarkId) return "bookmarks";
  if (page === "discover" && collectionId) return "collections";
  return page;
}

// renderedPage is the page a Route actually puts on screen.
function renderedPage(r: Route): Page {
  return legacyPage(r.page, r.bookmarkId, r.collectionId);
}

// kpDepth tracks how many entries we have pushed onto the history stack since the
// app loaded. It lets dismiss() know whether a real "back" target exists (an
// in-app open) or whether we landed here via a deep link / reload (depth 0), in
// which case going back would leave the app entirely.
function depth(): number {
  const s = window.history.state as { kpDepth?: number } | null;
  return s && typeof s.kpDepth === "number" ? s.kpDepth : 0;
}

// A hash-only pushState/replaceState does NOT fire "popstate" or "hashchange",
// so we dispatch a synthetic popstate to nudge useRoute() subscribers. Real
// back/forward and manual address-bar edits still fire their native events.
function notify() {
  window.dispatchEvent(new PopStateEvent("popstate"));
}

// pushRoute adds a history entry — use it for forward navigations that a single
// browser-back should undo (open a card, open a collection, switch tabs).
export function pushRoute(r: Route) {
  // Switching pages abandons whatever the old one was fetching, so hand its
  // connections back (see abortPendingRequests: a blocked request holds one of
  // the browser's ~6 per-origin sockets for its whole timeout, and a few of them
  // stall every page that follows). Only on a real page change — opening a card
  // keeps the same page mounted and its requests are still wanted. Done here,
  // synchronously before React re-renders, so the incoming page's own requests
  // start afterwards and are never caught by this.
  if (renderedPage(r) !== renderedPage(readRoute())) abortPendingRequests();
  window.history.pushState({ kpDepth: depth() + 1 }, "", "#" + buildHash(r));
  notify();
}

// replaceRoute rewrites the current entry in place (no new back step).
export function replaceRoute(r: Route) {
  window.history.replaceState({ kpDepth: depth() }, "", "#" + buildHash(r));
  notify();
}

// dismiss closes the topmost layer (a card or collection). When we opened it
// in-app there is a real entry to pop, so a plain back() keeps history clean and
// lets the forward button reopen it. When we arrived by deep link/reload there
// is nothing to pop, so we replace the URL with the given parent route instead
// of stepping out of the app.
export function dismiss(parent: Route) {
  if (depth() > 0) window.history.back();
  else replaceRoute(parent);
}

// useRoute subscribes a component to the current Route, re-rendering on back/
// forward, manual hash edits, and our own push/replace.
export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(readRoute);
  useEffect(() => {
    const on = () => setRoute(readRoute());
    window.addEventListener("popstate", on);
    window.addEventListener("hashchange", on);
    return () => {
      window.removeEventListener("popstate", on);
      window.removeEventListener("hashchange", on);
    };
  }, []);
  return route;
}

// Collection (подборки) and bookmark-folder (закладки) cards carry their title
// only in the list response, not the per-id items endpoint. Stash titles we have
// seen in localStorage so the breadcrumb survives both a reload and a deep link
// (e.g. "#/collections/c/<id>" / "#/bookmarks/b/<id>") opened in a fresh tab. An id
// never seen before falls back to a generic label — harmless, since the items
// still load from the id alone.
const COL_TITLES_KEY = "kp.collectionTitles";
const BM_TITLES_KEY = "kp.bookmarkTitles";

// How many titles to keep per key. The store used to gain an entry for every
// collection or folder ever opened and never dropped one, so a long-lived install
// crept toward the ~5 MB localStorage quota — past which setItem throws and, by
// the catch below, no title would ever be remembered again. Entries are held
// most-recent-first and the oldest fall off, so what is actually being browsed
// keeps its breadcrumb. (An id that has fallen off just shows the generic label.)
const MAX_REMEMBERED_TITLES = 200;

type TitleEntry = [id: string, title: string];

function readTitles(key: string): TitleEntry[] {
  try {
    const raw = JSON.parse(localStorage.getItem(key) || "[]");
    if (Array.isArray(raw)) {
      return raw.filter((e): e is TitleEntry => Array.isArray(e) && e.length === 2);
    }
    // Older builds stored a plain id → title object; carry those entries over.
    if (raw && typeof raw === "object") return Object.entries(raw as Record<string, string>);
  } catch {
    /* unreadable — start fresh rather than never remember a title again */
  }
  return [];
}

function rememberTitle(key: string, id: string, title: string) {
  if (!id || !title) return;
  // Re-inserting at the front doubles as the recency update, so a folder in
  // regular use is never the one evicted.
  const entries: TitleEntry[] = [[id, title], ...readTitles(key).filter(([k]) => k !== id)];
  try {
    localStorage.setItem(key, JSON.stringify(entries.slice(0, MAX_REMEMBERED_TITLES)));
  } catch {
    /* localStorage unavailable — breadcrumb just falls back to a label */
  }
}

function cachedTitle(key: string, id: string): string {
  if (!id) return "";
  return readTitles(key).find(([k]) => k === id)?.[1] || "";
}

export const rememberCollectionTitle = (id: string, title: string) => rememberTitle(COL_TITLES_KEY, id, title);
export const collectionTitle = (id: string) => cachedTitle(COL_TITLES_KEY, id);
export const rememberBookmarkTitle = (id: string, title: string) => rememberTitle(BM_TITLES_KEY, id, title);
export const bookmarkTitle = (id: string) => cachedTitle(BM_TITLES_KEY, id);
