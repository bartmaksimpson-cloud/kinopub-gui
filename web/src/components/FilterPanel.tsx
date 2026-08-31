import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import clsx from "clsx";
import {
  ArrowUpDown,
  Captions,
  Check,
  ChevronDown,
  Filter,
  RotateCcw,
  Search,
  Volume2,
  X,
} from "lucide-react";
import { api, type NamedRef } from "../api";
import { useI18n } from "../i18n";

export interface FilterState {
  category: string; // category key (see categories.tsx); drives type/genre
  genre: string; // sub-genre id (type categories only)
  country: string;
  sort: string;
  yearFrom: number;
  yearTo: number;
  kpFrom: number;
  kpTo: number;
  imdbFrom: number;
  imdbTo: number;
  ac3: boolean;
  subtitles: boolean;
}

export const YEAR_MIN = 1912;
export const YEAR_MAX = 2026;

export const defaultFilter = (): FilterState => ({
  category: "",
  genre: "",
  country: "",
  sort: "created-",
  yearFrom: YEAR_MIN,
  yearTo: YEAR_MAX,
  kpFrom: 0,
  kpTo: 10,
  imdbFrom: 0,
  imdbTo: 10,
  ac3: false,
  subtitles: false,
});

// The fields this panel owns. Category and genre are deliberately excluded: they
// have their own always-visible chip rows in the catalog, so resetting the panel
// narrows the user back to "this category, no extra conditions" instead of
// yanking them out of the category they are browsing.
const PANEL_DEFAULTS = (): Omit<FilterState, "category" | "genre"> => {
  const { category: _c, genre: _g, ...rest } = defaultFilter();
  return rest;
};

// Only the fields the API actually orders by: id, year, title, created, updated,
// rating, views, watchers. "kinopoisk-"/"imdb-" used to sit here too, but the API
// silently ignores unknown sort fields and falls back to "updated-", so those two
// chips did nothing — narrow by those ratings with the sliders below instead.
// "rating" is kino.watch's own score, not KP/IMDb.
//
// Note these are plain catalog orderings, NOT the site's charts: sorting
// everything by views yields an all-time hall of fame. The real Popular/Hot
// charts are the chips above the results (see Discover).
const SORTS = [
  { v: "updated-", label: "By update" },
  { v: "created-", label: "Fresh" },
  { v: "year-", label: "Year" },
  { v: "rating-", label: "By rating" },
  { v: "views-", label: "By views" },
  { v: "watchers-", label: "By watchers" },
];

// Decade shortcuts: dragging a 1912–2026 slider to "the 2010s" is fiddly, one tap
// is not. YEAR_MAX caps the open-ended ones so they never exceed the track.
const YEAR_PRESETS = [
  { label: "Last 2 years", from: YEAR_MAX - 2, to: YEAR_MAX },
  { label: "2020s", from: 2020, to: YEAR_MAX },
  { label: "2010s", from: 2010, to: 2019 },
  { label: "2000s", from: 2000, to: 2009 },
  { label: "1990s", from: 1990, to: 1999 },
];

const RATING_PRESETS = [6, 7, 8, 9];

// A summary of everything the panel currently narrows by, one removable chip per
// condition. This is what makes a collapsed panel honest: the user can see (and
// undo) each active condition without opening anything.
interface ActiveChip {
  key: string;
  label: string;
  clear: Partial<FilterState>;
}

// span renders a numeric range compactly: "2000–2010", "2000+", "≤2010".
const span = (from: number, to: number, min: number, max: number): string => {
  if (from > min && to < max) return `${from}–${to}`;
  if (from > min) return `${from}+`;
  return `≤${to}`;
};

function activeChips(f: FilterState, countries: NamedRef[], t: (k: string) => string): ActiveChip[] {
  const out: ActiveChip[] = [];
  if (f.country) {
    out.push({
      key: "country",
      label: countries.find((c) => c.id === f.country)?.title || t("Country"),
      clear: { country: "" },
    });
  }
  if (f.yearFrom > YEAR_MIN || f.yearTo < YEAR_MAX) {
    out.push({
      key: "year",
      label: span(f.yearFrom, f.yearTo, YEAR_MIN, YEAR_MAX),
      clear: { yearFrom: YEAR_MIN, yearTo: YEAR_MAX },
    });
  }
  if (f.kpFrom > 0 || f.kpTo < 10) {
    out.push({
      key: "kp",
      label: `${t("KP")} ${span(f.kpFrom, f.kpTo, 0, 10)}`,
      clear: { kpFrom: 0, kpTo: 10 },
    });
  }
  if (f.imdbFrom > 0 || f.imdbTo < 10) {
    out.push({
      key: "imdb",
      label: `IMDb ${span(f.imdbFrom, f.imdbTo, 0, 10)}`,
      clear: { imdbFrom: 0, imdbTo: 10 },
    });
  }
  if (f.ac3) out.push({ key: "ac3", label: "AC3", clear: { ac3: false } });
  if (f.subtitles) out.push({ key: "subtitles", label: t("Subtitles"), clear: { subtitles: false } });
  return out;
}

export function FilterPanel({
  value,
  onChange,
}: {
  value: FilterState;
  onChange: (f: FilterState) => void;
}) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [countries, setCountries] = useState<NamedRef[]>([]);

  // Type and genre live in the category bar / genre chips above (see Discover);
  // this panel covers the remaining cross-cutting filters.
  useEffect(() => {
    api.discoverCountries().then((r) => setCountries(r.items || [])).catch(() => {});
  }, []);

  const patch = (p: Partial<FilterState>) => onChange({ ...value, ...p });
  const chips = activeChips(value, countries, t);

  return (
    // `relative z-30` is load-bearing, not decoration: .card carries
    // backdrop-blur, and backdrop-filter opens a stacking context, so the
    // country popover's z-20 only ever competes *inside* this card. The poster
    // tiles below are position:relative, which paints them after a static
    // block — the whole card, popover included, would slide under the grid.
    // Lifting the card itself puts it above the tiles and below modals (z-50).
    <div className="card relative z-30 p-2 sm:p-2.5">
      {/* Toolbar: the filter toggle carries a live count, and sort — the control
          reached for most often — stays out here instead of hiding behind it. */}
      <div className="flex flex-wrap items-center gap-2">
        <button
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className={clsx(
            "flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition",
            open || chips.length
              ? "bg-gold-500/[0.12] text-gold-200 hover:bg-gold-500/[0.18]"
              : "text-slate-300 hover:bg-white/[0.05] hover:text-slate-100",
          )}
        >
          <Filter className="h-4 w-4" />
          {t("Filter")}
          {chips.length > 0 && (
            <span className="grid h-4 min-w-[1rem] place-items-center rounded-full bg-gold-500 px-1 text-[10px] font-bold text-ink-950">
              {chips.length}
            </span>
          )}
          <ChevronDown className={clsx("h-4 w-4 transition-transform", open && "rotate-180")} />
        </button>

        <div className="relative ml-auto">
          <ArrowUpDown className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
          <select
            aria-label={t("Sort")}
            value={value.sort}
            onChange={(e) => patch({ sort: e.target.value })}
            className="cursor-pointer appearance-none rounded-xl border border-white/[0.08] bg-ink-900/70 py-2 pl-8 pr-8 text-sm text-slate-200 outline-none transition hover:border-white/[0.14] focus:border-gold-500/60 focus:ring-2 focus:ring-gold-500/20"
          >
            {SORTS.map((s) => (
              <option key={s.v} value={s.v}>
                {t(s.label)}
              </option>
            ))}
          </select>
          <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
        </div>

        {chips.length > 0 && (
          <button
            onClick={() => patch(PANEL_DEFAULTS())}
            title={t("Reset filters")}
            className="flex items-center gap-1.5 rounded-xl px-2.5 py-2 text-xs text-slate-400 transition hover:bg-white/[0.05] hover:text-gold-300"
          >
            <RotateCcw className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{t("Reset filters")}</span>
          </button>
        )}
      </div>

      {/* Active conditions, removable one by one — visible whether or not the
          panel below is expanded. */}
      {chips.length > 0 && (
        <div className="flex flex-wrap gap-1.5 px-1 pb-1 pt-2">
          {chips.map((c) => (
            <button
              key={c.key}
              onClick={() => patch(c.clear)}
              title={t("Remove filter")}
              className="group flex items-center gap-1 rounded-full border border-gold-500/25 bg-gold-500/[0.10] py-1 pl-2.5 pr-1.5 text-xs font-medium text-gold-200 transition hover:border-gold-500/50 hover:bg-gold-500/[0.18]"
            >
              {c.label}
              <X className="h-3 w-3 text-gold-300/60 transition group-hover:text-gold-200" />
            </button>
          ))}
        </div>
      )}

      {open && (
        <div className="mt-2 grid gap-5 border-t border-white/[0.06] px-1 pb-1 pt-4 sm:grid-cols-2">
          <Combo
            label={t("Country")}
            value={value.country}
            options={[{ id: "", title: t("All countries") }, ...countries]}
            onChange={(v) => patch({ country: v })}
            placeholder={t("Search country…")}
          />

          <div>
            <span className="label">{t("Sound & subtitles")}</span>
            <div className="flex flex-wrap gap-2">
              <TogglePill active={value.ac3} onClick={() => patch({ ac3: !value.ac3 })} icon={Volume2} label={t("AC3 sound")} />
              <TogglePill
                active={value.subtitles}
                onClick={() => patch({ subtitles: !value.subtitles })}
                icon={Captions}
                label={t("With subtitles")}
              />
            </div>
          </div>

          <div className="sm:col-span-2">
            <Range
              label={t("Release year")}
              min={YEAR_MIN}
              max={YEAR_MAX}
              step={1}
              from={value.yearFrom}
              to={value.yearTo}
              editable
              onChange={(a, b) => patch({ yearFrom: a, yearTo: b })}
            >
              <Preset
                active={value.yearFrom === YEAR_MIN && value.yearTo === YEAR_MAX}
                onClick={() => patch({ yearFrom: YEAR_MIN, yearTo: YEAR_MAX })}
              >
                {t("Any")}
              </Preset>
              {YEAR_PRESETS.map((p) => (
                <Preset
                  key={p.label}
                  active={value.yearFrom === p.from && value.yearTo === p.to}
                  onClick={() => patch({ yearFrom: p.from, yearTo: p.to })}
                >
                  {t(p.label)}
                </Preset>
              ))}
            </Range>
          </div>

          <Range
            label={t("Kinopoisk rating")}
            min={0}
            max={10}
            step={0.5}
            from={value.kpFrom}
            to={value.kpTo}
            onChange={(a, b) => patch({ kpFrom: a, kpTo: b })}
          >
            <RatingPresets from={value.kpFrom} to={value.kpTo} onPick={(a, b) => patch({ kpFrom: a, kpTo: b })} />
          </Range>

          <Range
            label={t("IMDb rating")}
            min={0}
            max={10}
            step={0.5}
            from={value.imdbFrom}
            to={value.imdbTo}
            onChange={(a, b) => patch({ imdbFrom: a, imdbTo: b })}
          >
            <RatingPresets from={value.imdbFrom} to={value.imdbTo} onPick={(a, b) => patch({ imdbFrom: a, imdbTo: b })} />
          </Range>
        </div>
      )}
    </div>
  );
}

// Combo is a searchable single-select. kino.watch returns ~100 countries, which
// is more than a native <select> can be scanned through comfortably.
function Combo({
  label,
  value,
  options,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  options: NamedRef[];
  onChange: (v: string) => void;
  placeholder: string;
}) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const list = useMemo(() => {
    const s = q.trim().toLowerCase();
    return s ? options.filter((o) => o.title.toLowerCase().includes(s)) : options;
  }, [q, options]);

  const current = options.find((o) => o.id === value);

  return (
    <div ref={box} className="relative">
      <label className="label">{label}</label>
      <button
        type="button"
        onClick={() => {
          setQ("");
          setOpen((o) => !o);
        }}
        aria-expanded={open}
        className="input flex items-center justify-between gap-2 text-left"
      >
        <span className={clsx("truncate", !value && "text-slate-500")}>{current?.title || options[0]?.title}</span>
        <ChevronDown className={clsx("h-4 w-4 shrink-0 text-slate-500 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div className="absolute left-0 right-0 z-20 mt-1.5 overflow-hidden rounded-xl border border-white/[0.08] bg-ink-850 shadow-card">
          <div className="relative border-b border-white/[0.06]">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
            <input
              autoFocus
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder={placeholder}
              className="w-full bg-transparent py-2.5 pl-9 pr-3 text-sm text-slate-100 placeholder:text-slate-500 outline-none"
            />
          </div>
          <div className="max-h-56 overflow-y-auto py-1">
            {list.length === 0 ? (
              <p className="px-3 py-3 text-center text-xs text-slate-500">{t("Nothing found.")}</p>
            ) : (
              list.map((o) => (
                <button
                  key={o.id || "any"}
                  onClick={() => {
                    onChange(o.id);
                    setOpen(false);
                  }}
                  className={clsx(
                    "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition",
                    o.id === value ? "bg-gold-500/[0.12] text-gold-200" : "text-slate-300 hover:bg-white/[0.05]",
                  )}
                >
                  <Check className={clsx("h-3.5 w-3.5 shrink-0", o.id === value ? "opacity-100" : "opacity-0")} />
                  <span className="truncate">{o.title}</span>
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// Range is a two-slider min/max control. Two native sliders keep it robust and
// accessible; `children` render as a preset row beneath the track.
function Range({
  label,
  min,
  max,
  step,
  from,
  to,
  onChange,
  editable,
  children,
}: {
  label: string;
  min: number;
  max: number;
  step: number;
  from: number;
  to: number;
  onChange: (from: number, to: number) => void;
  /** Show the endpoints as typeable number boxes (years) instead of a badge. */
  editable?: boolean;
  children?: ReactNode;
}) {
  const { t } = useI18n();
  const pct = (v: number) => ((v - min) / (max - min)) * 100;
  // With both thumbs stacked at one end, whichever input sits on top wins every
  // drag — and at the top end that is the useless one. Hand the top layer to the
  // handle that still has room to move.
  const fromOnTop = (pct(from) + pct(to)) / 2 > 50;
  // The number boxes and the sliders set the same two values, so they need
  // distinct accessible names — "from/to" for the typed pair, "minimum/maximum"
  // for the handles — instead of two controls answering to one name.

  return (
    <div>
      <div className="mb-2.5 flex items-center justify-between gap-2">
        <label className="label mb-0">{label}</label>
        {editable ? (
          <span className="flex items-center gap-1">
            <NumBox value={from} min={min} max={to} onCommit={(v) => onChange(v, to)} label={`${label} — ${t("from")}`} />
            <span className="text-xs text-slate-500">–</span>
            <NumBox value={to} min={from} max={max} onCommit={(v) => onChange(from, v)} label={`${label} — ${t("to")}`} />
          </span>
        ) : (
          <span className="rounded bg-gold-500/15 px-1.5 py-0.5 font-mono text-xs text-gold-300">
            {from} – {to}
          </span>
        )}
      </div>
      <div className="relative h-4">
        <div className="absolute top-1/2 h-1.5 w-full -translate-y-1/2 rounded-full bg-white/10" />
        <div
          className="absolute top-1/2 h-1.5 -translate-y-1/2 rounded-full bg-gradient-to-r from-gold-500/70 to-gold-400"
          style={{ left: `${pct(from)}%`, right: `${100 - pct(to)}%` }}
        />
        <input
          type="range"
          aria-label={`${label} — ${t("Minimum")}`}
          className="range-dual"
          style={{ zIndex: fromOnTop ? 3 : 2 }}
          min={min}
          max={max}
          step={step}
          value={from}
          onChange={(e) => onChange(Math.min(Number(e.target.value), to), to)}
        />
        <input
          type="range"
          aria-label={`${label} — ${t("Maximum")}`}
          className="range-dual"
          style={{ zIndex: fromOnTop ? 2 : 3 }}
          min={min}
          max={max}
          step={step}
          value={to}
          onChange={(e) => onChange(from, Math.max(Number(e.target.value), from))}
        />
      </div>
      {children && <div className="mt-2.5 flex flex-wrap gap-1.5">{children}</div>}
    </div>
  );
}

// NumBox edits one endpoint by keyboard. The draft is local so a half-typed
// "19" isn't clamped to the minimum mid-keystroke; it commits on blur / Enter.
function NumBox({
  value,
  min,
  max,
  onCommit,
  label,
}: {
  value: number;
  min: number;
  max: number;
  onCommit: (v: number) => void;
  label: string;
}) {
  const [draft, setDraft] = useState(String(value));
  useEffect(() => setDraft(String(value)), [value]);

  const commit = () => {
    const n = Math.round(Number(draft));
    if (!draft.trim() || !Number.isFinite(n)) {
      setDraft(String(value));
      return;
    }
    const clamped = Math.min(max, Math.max(min, n));
    // Sync the draft ourselves: when the clamped result equals the current
    // value the commit changes no state, the value-effect never re-fires, and
    // the box would keep showing a number the filter does not apply (type
    // "3000" at max 2026 → filter 2026, box "3000" forever).
    setDraft(String(clamped));
    onCommit(clamped);
  };

  return (
    <input
      aria-label={label}
      inputMode="numeric"
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
      className="w-14 rounded-md bg-gold-500/15 px-1.5 py-0.5 text-center font-mono text-xs text-gold-300 outline-none transition focus:ring-2 focus:ring-gold-500/30"
    />
  );
}

function RatingPresets({
  from,
  to,
  onPick,
}: {
  from: number;
  to: number;
  onPick: (from: number, to: number) => void;
}) {
  const { t } = useI18n();
  return (
    <>
      <Preset active={from === 0 && to === 10} onClick={() => onPick(0, 10)}>
        {t("Any")}
      </Preset>
      {RATING_PRESETS.map((r) => (
        <Preset key={r} active={from === r && to === 10} onClick={() => onPick(r, 10)}>
          {r}+
        </Preset>
      ))}
    </>
  );
}

function Preset({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={clsx(
        "rounded-full px-2.5 py-1 text-xs transition",
        active
          ? "bg-gold-500/[0.16] text-gold-200"
          : "bg-white/[0.04] text-slate-400 hover:bg-white/[0.08] hover:text-slate-200",
      )}
    >
      {children}
    </button>
  );
}

function TogglePill({
  active,
  onClick,
  icon: Icon,
  label,
}: {
  active: boolean;
  onClick: () => void;
  icon: typeof Volume2;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={clsx(
        "flex items-center gap-1.5 rounded-xl border px-3 py-2 text-sm transition",
        active
          ? "border-gold-500/40 bg-gold-500/[0.12] text-gold-200"
          : "border-white/[0.08] bg-ink-900/40 text-slate-400 hover:border-white/[0.16] hover:text-slate-200",
      )}
    >
      <Icon className="h-4 w-4" />
      {label}
    </button>
  );
}
