import { fileURLToPath, URL } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

import { resolveBase } from "./src/lib/base";

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  // Third argument "" loads every environment variable, not only VITE_-
  // prefixed ones, so this also sees a plain shell-exported VITE_BASE_PATH
  // (as the pages.yml workflow and `VITE_BASE_PATH=/cordvba/ pnpm build`
  // set it) and not only one from a .env file.
  const env = loadEnv(mode, process.cwd(), "");

  return {
    base: resolveBase(env),
    plugins: [react()],
    resolve: {
      alias: {
        "@contracts/environment/v1": fileURLToPath(
          new URL("../../packages/contracts/environment/v1", import.meta.url),
        ),
      },
    },
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: ["./src/test/setup.ts"],
      css: false,
    },
  };
});
