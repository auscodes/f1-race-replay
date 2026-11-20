import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

test("personal replay opens one segment and jumps with matching driver timing", async ({ page }) => {
  const { frames } = JSON.parse(readFileSync(new URL("../../testdata/demo.json", import.meta.url)));
  const requested = [];
  await page.route("**/api/session", async (route) => {
    const session = await (await route.fetch()).json();
    session.segmentSize = 30;
    session.timeline = {
      events: [{elapsedMs: 8001, kind: "pit", label: "Pit stop"}],
      drivers: {"01": {laps: [{elapsedMs: 0, value: "1"}, {elapsedMs: 6001, value: "2"}]}},
    };
    await route.fulfill({json: session});
  });
  await page.route("**/api/segments/*", async (route) => {
    const segment = Number(new URL(route.request().url()).pathname.split("/").at(-1));
    requested.push(segment);
    const startFrame = segment * 30;
    await route.fulfill({json: {startFrame, frames: frames.slice(startFrame, startFrame + 30)}});
  });
  await page.goto("/");
  await expect(page.getByRole("status")).toHaveText("Local analysis");
  expect(requested).toEqual([0]);
  await expect(page.locator("#drivers")).toContainText("Lap 1");

  await page.getByLabel("Hamilton lap").selectOption({label: "Lap 2"});
  await expect(page.locator("#elapsed")).toHaveText("0:06.2");
  await expect(page.locator("#drivers")).toContainText("Lap 2");
  expect(requested).toEqual([0, 1]);

  await page.getByLabel("Race event").selectOption({label: "Pit stop · 0:08"});
  await expect(page.locator("#elapsed")).toHaveText("0:08.2");
  expect(requested).toEqual([0, 1]);
  await page.getByRole("button", {name: "Play", exact: true}).click();
  await expect.poll(async () => Number(await page.locator("#sequence").innerText())).toBeGreaterThan(41);
  await page.getByRole("button", {name: "Pause", exact: true}).click();
  await expect(page.getByRole("button", {name: "Play", exact: true})).toBeVisible();
});
