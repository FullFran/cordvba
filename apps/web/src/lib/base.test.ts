import { describe, expect, it } from "vitest";

import { resolveBase } from "./base";

describe("resolveBase", () => {
  it("defaults to '/' when VITE_BASE_PATH is not set", () => {
    expect(resolveBase({})).toBe("/");
  });

  it("defaults to '/' when VITE_BASE_PATH is an empty string", () => {
    expect(resolveBase({ VITE_BASE_PATH: "" })).toBe("/");
  });

  it("uses VITE_BASE_PATH when set, e.g. for a GitHub Pages sub-path", () => {
    expect(resolveBase({ VITE_BASE_PATH: "/cordvba/" })).toBe("/cordvba/");
  });

  it("passes through a bare '/' unchanged", () => {
    expect(resolveBase({ VITE_BASE_PATH: "/" })).toBe("/");
  });
});
