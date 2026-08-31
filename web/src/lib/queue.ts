import type { JobStatus, JobView } from "../api";

// What is already lined up for a title. Queueing the same episode twice
// downloads the same bytes into the same file, so both the card and the server
// (see queueCoverage in internal/gui/jobs.go) refuse the duplicate — this module
// is the browser half of that rule.

// A job holds a claim on its episodes while it still has work ahead of it.
// A failed job does not: its card offers a Retry, and starting the title over is
// a legitimate way out of a failure, so it must not lock the title forever.
const ACTIVE: JobStatus[] = ["queued", "resolving", "running", "paused"];

// itemIdFromURL pulls the kino.watch item id out of a catalog URL.
export function itemIdFromURL(url: string): string {
  const m = (url || "").match(/\/item\/view\/(\d+)/);
  return m ? m[1] : "";
}

// epRef is the season/episode identity used for matching. Key *strings* are
// formatted differently in different places ("S1E1" vs "S01E01"), so the numbers
// are the only safe thing to compare.
export function epRef(season: number, episode: number): string {
  return `${season}:${episode}`;
}

export interface QueueCoverage {
  /** Episodes an active job will still download, as epRef() strings. */
  refs: Set<string>;
  /**
   * True when an active job is committed to the whole title: it hasn't resolved
   * its episode list yet, so we can't say which episodes it will take — only
   * that it will take everything the user asked for the first time.
   */
  whole: boolean;
}

// queueCoverage summarizes what the queue already holds for one item.
//
// A job that has resolved its plan reports its episodes, and only those it still
// intends to produce count: a failed episode is fair game to queue again. A job
// that has NOT resolved yet falls back to its explicit selection — the same
// fallback the server's guard uses — so a one-episode job waiting for a slot no
// longer locks the whole title. Only a job with neither (no rows, no selection)
// covers the title as a whole, which is deliberately the conservative answer.
//
// outputPath, when given, scopes the answer to jobs downloading into that
// folder — the server's rule keys on URL+folder, and a job filling a different
// folder does not collide with this one.
export function queueCoverage(jobs: JobView[], itemId: string, outputPath?: string): QueueCoverage {
  const refs = new Set<string>();
  let whole = false;
  if (!itemId) return { refs, whole };
  for (const j of jobs) {
    if (!ACTIVE.includes(j.status)) continue;
    if (j.dryRun) continue; // a preview run produces no files
    if (itemIdFromURL(j.url) !== itemId) continue;
    if (outputPath && j.outputPath && j.outputPath !== outputPath) continue;
    if (j.episodes.length > 0) {
      for (const e of j.episodes) {
        if (e.state === "failed") continue;
        refs.add(epRef(e.season, e.episode));
      }
      continue;
    }
    if (j.selectedEpisodes?.length) {
      for (const k of j.selectedEpisodes) {
        const m = /^S(\d+)E(\d+)$/i.exec(k);
        if (m) refs.add(epRef(Number(m[1]), Number(m[2])));
      }
      continue;
    }
    whole = true;
  }
  return { refs, whole };
}

// isQueued reports whether one episode is already covered.
export function isQueued(cov: QueueCoverage, season: number, episode: number): boolean {
  return cov.whole || cov.refs.has(epRef(season, episode));
}

// completedSignal counts everything that has landed on disk: finished jobs plus
// the individual episodes finished inside jobs still running. The library list
// is built by scanning the folder, so it only learns about a new file when
// something asks it to rescan — and watching whole jobs alone meant a season
// downloading one episode at a time put nine files on disk that the list below
// would not show until the user pressed Rescan by hand.
//
// The caller rescans when this number GROWS; it can also shrink (a card removed,
// finished jobs cleared), which is not news from the disk.
export function completedSignal(jobs: JobView[]): number {
  let n = 0;
  for (const j of jobs) {
    if (j.status === "completed") n++;
    for (const e of j.episodes) {
      if (e.state === "completed") n++;
    }
  }
  return n;
}

// hasOtherActiveJob reports whether any OTHER job is waiting or downloading —
// i.e. whether "Prioritize" has anything to get ahead of. Without competition
// the button does nothing, and on a job that is dispatched the instant it is
// queued (a per-episode resume) it would flash for a frame and vanish.
export function hasOtherActiveJob(jobs: JobView[], id: string): boolean {
  return jobs.some((o) => o.id !== id && ACTIVE_OR_RESOLVING.includes(o.status));
}

const ACTIVE_OR_RESOLVING: JobStatus[] = ["queued", "resolving", "running"];

// A Set of keys or a Map keyed by them — both answer .has(), which is all these
// two rules need.
type KeyLookup = { has(key: string): boolean };

// initialEpisodeSelection is what a freshly opened title card starts ticked:
// every episode that is neither already on disk nor already queued. Including a
// downloaded one would promise work that never happens — the engine skips what
// the state file records as complete — so the "N selected" count would overstate
// the download before it even starts.
export function initialEpisodeSelection(
  allKeys: string[],
  downloaded: KeyLookup,
  queued: KeyLookup,
): Set<string> {
  return new Set(allKeys.filter((k) => !downloaded.has(k) && !queued.has(k)));
}

// nothingLeftToQueue reports whether every episode of a serial is already queued
// or already downloaded, so a new job could add nothing.
export function nothingLeftToQueue(allKeys: string[], downloaded: KeyLookup, queued: KeyLookup): boolean {
  return allKeys.length > 0 && allKeys.every((k) => queued.has(k) || downloaded.has(k));
}
