import { load } from "cheerio";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const platform = { platform: "unknown", mobileOS: null as string | null };

vi.mock("../hooks/useOS", () => ({
  Platform: { Mobile: "mobile", Unknown: "unknown" },
  usePlatform: () => platform,
}));

import { AndroidAppCTA } from "./AndroidAppCTA";
import { MobileAppCTA } from "./MobileAppCTA";

beforeEach(() => {
  platform.platform = "unknown";
  platform.mobileOS = null;
});

describe("store CTAs", () => {
  it("does not expose an unprovisioned iOS store URL", () => {
    platform.platform = "mobile";
    platform.mobileOS = "ios";

    const $ = load(renderToStaticMarkup(<MobileAppCTA />));

    expect($("a")).toHaveLength(0);
    expect($("button")).toHaveLength(0);
    expect($("span").text()).toContain("iOS app coming soon");
  });

  it("does not expose an unprovisioned Android store URL", () => {
    platform.platform = "mobile";
    platform.mobileOS = "android";

    const $ = load(renderToStaticMarkup(<AndroidAppCTA />));

    expect($("a")).toHaveLength(0);
    expect($("button")).toHaveLength(0);
    expect($("span").text()).toContain("Android app coming soon");
  });
});
