import { defineConfig } from "@playwright/test";

const address = "127.0.0.1:18080";
const baseURL = `http://${address}`;

export default defineConfig({
  testDir: "./tests/browser",
  use: { baseURL },
  webServer: {
    command: `go run . -addr ${address}`,
    url: baseURL,
    reuseExistingServer: false,
  },
});
