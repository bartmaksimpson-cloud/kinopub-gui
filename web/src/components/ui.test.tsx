import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Modal } from "./ui";

// The page behind an open modal must not scroll: a wheel gesture that runs past
// the end of the card would otherwise drag the catalog underneath it.
describe("Modal body scroll lock", () => {
  afterEach(() => {
    document.body.style.overflow = "";
    document.body.style.paddingRight = "";
  });

  it("locks the body while open and restores the previous style on close", () => {
    document.body.style.overflow = "auto";
    const { unmount } = render(
      <Modal open onClose={() => {}} title="t">
        body
      </Modal>,
    );
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("auto");
  });

  it("never locks while closed", () => {
    render(
      <Modal open={false} onClose={() => {}} title="t">
        body
      </Modal>,
    );
    expect(document.body.style.overflow).toBe("");
  });

  it("keeps the lock until the last stacked modal closes", () => {
    const outer = render(
      <Modal open onClose={() => {}} title="outer">
        outer
      </Modal>,
    );
    const inner = render(
      <Modal open onClose={() => {}} title="inner">
        inner
      </Modal>,
    );
    expect(document.body.style.overflow).toBe("hidden");
    inner.unmount();
    expect(document.body.style.overflow).toBe("hidden");
    outer.unmount();
    expect(document.body.style.overflow).toBe("");
  });

  it("drops its own header in bare mode so the content can draw a hero", () => {
    const { rerender } = render(
      <Modal open onClose={() => {}} title="Heading">
        body
      </Modal>,
    );
    expect(screen.getByRole("heading", { name: "Heading" })).toBeInTheDocument();
    rerender(
      <Modal open onClose={() => {}} title="Heading" bare>
        body
      </Modal>,
    );
    expect(screen.queryByRole("heading", { name: "Heading" })).not.toBeInTheDocument();
  });
});
