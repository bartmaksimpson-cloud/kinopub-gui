import { useState } from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { FilterPanel, YEAR_MAX, YEAR_MIN, defaultFilter, type FilterState } from "./FilterPanel";

const discoverCountries = vi.fn();
vi.mock("../api", () => ({
  api: {
    discoverCountries: () => discoverCountries(),
  },
}));

// The panel is controlled, so a tiny stateful harness stands in for the Catalog
// page. It also dumps the live filter as JSON, which is the only way to observe
// fields the panel itself doesn't render (category/genre, sort).
function Harness({ initial }: { initial?: Partial<FilterState> }) {
  const [f, setF] = useState<FilterState>({ ...defaultFilter(), ...initial });
  return (
    <I18nProvider>
      <FilterPanel value={f} onChange={setF} />
      <pre data-testid="state">{JSON.stringify(f)}</pre>
    </I18nProvider>
  );
}

const state = (): FilterState => JSON.parse(screen.getByTestId("state").textContent || "{}");

// Renders and flushes the countries fetch so no state settles outside act().
async function setup(initial?: Partial<FilterState>) {
  render(<Harness initial={initial} />);
  await act(async () => {});
}

const chips = () => screen.queryAllByTitle("Remove filter");
const chipLabels = () => chips().map((c) => c.textContent);
const expand = () => fireEvent.click(screen.getByRole("button", { name: /Filter/ }));

beforeEach(() => {
  discoverCountries.mockResolvedValue({
    items: [
      { id: "1", title: "USA" },
      { id: "2", title: "United Kingdom" },
      { id: "3", title: "France" },
    ],
  });
});

describe("FilterPanel active-condition chips", () => {
  it("shows no chips for a default filter", async () => {
    await setup();
    expect(chips()).toHaveLength(0);
  });

  it("renders one chip per active condition", async () => {
    await setup({ country: "1", yearFrom: 2000, yearTo: 2010, kpFrom: 7, ac3: true });
    expect(chipLabels()).toEqual(["USA", "2000–2010", "KP 7+", "AC3"]);
  });

  it("renders open-ended ranges as `from+` and `≤to`", async () => {
    await setup({ yearFrom: 2000, imdbTo: 8 });
    expect(chipLabels()).toEqual(["2000+", "IMDb ≤8"]);
  });

  it("counts the active conditions on the toggle", async () => {
    await setup({ ac3: true, subtitles: true, kpFrom: 8 });
    expect(screen.getByRole("button", { name: /Filter/ })).toHaveTextContent("3");
  });

  it("clears only its own condition when a chip is removed", async () => {
    await setup({ yearFrom: 2000, yearTo: 2010, ac3: true });
    fireEvent.click(screen.getByRole("button", { name: /2000–2010/ }));

    expect(chipLabels()).toEqual(["AC3"]);
    const f = state();
    expect([f.yearFrom, f.yearTo]).toEqual([YEAR_MIN, YEAR_MAX]);
    expect(f.ac3).toBe(true);
  });
});

describe("FilterPanel reset", () => {
  it("clears the panel's own fields but keeps the category and genre", async () => {
    await setup({ category: "movie", genre: "5", country: "1", kpFrom: 8, subtitles: true });
    fireEvent.click(screen.getByTitle("Reset filters"));

    expect(chips()).toHaveLength(0);
    const f = state();
    expect(f.category).toBe("movie");
    expect(f.genre).toBe("5");
    expect(f.country).toBe("");
    expect(f.kpFrom).toBe(0);
    expect(f.subtitles).toBe(false);
  });

  it("is hidden while nothing is active", async () => {
    await setup();
    expect(screen.queryByTitle("Reset filters")).toBeNull();
  });
});

describe("FilterPanel controls", () => {
  it("exposes sort without expanding the panel", async () => {
    await setup();
    fireEvent.change(screen.getByLabelText("Sort"), { target: { value: "views-" } });
    expect(state().sort).toBe("views-");
  });

  it("picks a country through the searchable combobox", async () => {
    await setup();
    expand();
    fireEvent.click(screen.getByRole("button", { name: /All countries/ }));

    // Typing narrows the list to the single match.
    fireEvent.change(screen.getByPlaceholderText("Search country…"), { target: { value: "united" } });
    expect(screen.queryByRole("button", { name: /^USA$/ })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /United Kingdom/ }));
    expect(state().country).toBe("2");
    expect(chipLabels()).toEqual(["United Kingdom"]);
  });

  it("applies a decade preset to both year endpoints", async () => {
    await setup();
    expand();
    fireEvent.click(screen.getByRole("button", { name: "2010s" }));

    const f = state();
    expect([f.yearFrom, f.yearTo]).toEqual([2010, 2019]);
  });

  it("applies a rating preset to the matching rating only", async () => {
    await setup();
    expand();
    const imdb = screen.getByLabelText("IMDb rating — Minimum").closest("div")!.parentElement!;
    fireEvent.click(within(imdb).getByRole("button", { name: "8+" }));

    const f = state();
    expect([f.imdbFrom, f.imdbTo]).toEqual([8, 10]);
    expect([f.kpFrom, f.kpTo]).toEqual([0, 10]);
  });

  it("commits a typed year on blur, clamped to the track", async () => {
    await setup();
    expand();
    const box = screen.getByLabelText("Release year — from");
    fireEvent.change(box, { target: { value: "1800" } });
    fireEvent.blur(box);

    expect(state().yearFrom).toBe(YEAR_MIN);
  });

  it("toggles the AC3 and subtitles pills", async () => {
    await setup();
    expand();
    fireEvent.click(screen.getByRole("button", { name: "AC3 sound" }));
    fireEvent.click(screen.getByRole("button", { name: "With subtitles" }));

    const f = state();
    expect([f.ac3, f.subtitles]).toEqual([true, true]);
    expect(chipLabels()).toEqual(["AC3", "Subtitles"]);
  });
});
