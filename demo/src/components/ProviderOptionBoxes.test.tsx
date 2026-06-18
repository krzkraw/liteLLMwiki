import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import { ProviderOptionBoxes } from "./ProviderOptionBoxes";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

let root: Root | null = null;
let container: HTMLDivElement | null = null;

function getByTestId<T extends Element = Element>(testId: string): T {
  const element = container?.querySelector(`[data-testid="${testId}"]`);

  if (!element) {
    throw new Error(`Unable to find element with data-testid="${testId}".`);
  }

  return element as T;
}

async function renderOptionBoxes() {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);

  await act(async () => {
    root?.render(<ProviderOptionBoxes provider="web" />);
  });
}

describe("ProviderOptionBoxes", () => {
  afterEach(() => {
    act(() => {
      root?.unmount();
    });
    container?.remove();
    root = null;
    container = null;
  });

  it("renders compact option pills with title tooltips", async () => {
    await renderOptionBoxes();

    const pill = getByTestId("provider-option-pill-temperature");
    const input = getByTestId<HTMLInputElement>("provider-option-input-temperature");

    expect(pill.className).toContain("provider-option-pill");
    expect(pill.getAttribute("title")).toContain("Sampling temperature");
    expect(input.value).toBe("0.7");
  });

  it("shows a reset button for changed values and restores defaults", async () => {
    await renderOptionBoxes();

    const input = getByTestId<HTMLInputElement>("provider-option-input-temperature");

    await act(async () => {
      input.value = "0.25";
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });

    expect(getByTestId("provider-option-reset-temperature").textContent).toBe(
      "Reset",
    );

    await act(async () => {
      getByTestId<HTMLButtonElement>("provider-option-reset-temperature").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(getByTestId<HTMLInputElement>("provider-option-input-temperature").value).toBe(
      "0.7",
    );
  });

  it("renders secret options as password inputs", async () => {
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);

    await act(async () => {
      root?.render(<ProviderOptionBoxes provider="executable" />);
    });

    expect(
      getByTestId<HTMLInputElement>("provider-option-input-huggingfaceToken").type,
    ).toBe("password");
  });
});
