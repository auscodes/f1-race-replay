import { expect, test } from "@playwright/test";

test("full-grid timing follows seeks and marks each finisher independently", async ({ page }) => {
  const value = (elapsedMs, value) => ({elapsedMs, value});
  const timings = {
    "01": {
      positions: [value(0, "2"), value(2000, "1")],
      gaps: [value(0, "0.521")],
      intervals: [value(0, "0.412")],
      tyres: [{lapStart: 1, lapEnd: 2, startingAge: 0, compound: "MEDIUM"}],
      laps: [value(0, "1")],
      completedLaps: [{elapsedMs: 8000, lap: 2, durationMs: 4000}],
      outcome: {elapsedMs: 8000, status: "finished", position: 1, gap: "0"},
    },
    "16": {
      positions: [value(0, "1"), value(2000, "2")],
      gaps: [value(0, "0"), value(2000, "+1 LAP")],
      outcome: {elapsedMs: 10_000, status: "finished", position: 2, gap: "+1 LAP"},
    },
    "99": {positions: [value(0, "3")], outcome: {elapsedMs: 11_800, status: "dnf"}},
  };
  const events = [
    {elapsedMs: 8000, kind: "flag", label: "Chequered flag"},
    {elapsedMs: 10_000, kind: "flag", label: "Second crossing"},
  ];
  await page.route("**/api/session", async (route) => {
    const session = await (await route.fetch()).json();
    session.drivers = Object.keys(timings);
    session.grid = {"01": {code: "ONE"}, "16": {code: "TWO"}, "99": {code: "OUT"}};
    session.timeline = {events, drivers: {"01": {laps: timings["01"].laps}}};
    await route.fulfill({json: session});
  });
  let releaseTiming;
  const gate = new Promise((resolve) => { releaseTiming = resolve; });
  await page.route("**/api/timing", async (route) => {
    await gate;
    await route.fulfill({json: {events, drivers: timings}});
  });
  await page.goto("/");
  await expect(page.locator("#connection")).toHaveText("Local analysis");
  await expect(page.locator("#tower-message")).toBeVisible();
  releaseTiming();

  const rows = page.locator("#tower-rows tr");
  const first = rows.filter({has: page.getByText("ONE", {exact: true})});
  const second = rows.filter({has: page.getByText("TWO", {exact: true})});
  const retired = rows.filter({has: page.getByText("OUT", {exact: true})});
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toContainText("TWO");
  await expect(first).toContainText("+0.521");
  await expect(first).toContainText("M 0");
  await page.getByLabel("Time difference").selectOption("interval");
  await expect(first).toContainText("+0.412");
  await page.getByLabel("Time difference").selectOption("leader");
  const highlight = first.getByRole("button");
  await highlight.click();
  await expect(highlight).toHaveAttribute("aria-pressed", "true");

  await page.locator("#event-jump").selectOption("8000");
  await expect(rows.first()).toContainText("ONE");
  await expect(first.getByRole("img", {name: "Finished"})).toBeVisible();
  await expect(first).toContainText("0:04.000");
  await expect(second.getByRole("img", {name: "Finished"})).toBeHidden();
  await expect(second).toContainText("+1 lap");

  await page.locator("#event-jump").selectOption("10000");
  await expect(second.getByRole("img", {name: "Finished"})).toBeVisible();
  const slider = page.getByRole("slider", {name: "Replay time"});
  await slider.press("End");
  await expect(retired).toContainText("DNF");
  await expect(retired.getByRole("img", {name: "Finished"})).toBeHidden();
  await slider.press("Home");
  await expect(rows.first()).toContainText("TWO");
  await expect(first.getByRole("img", {name: "Finished"})).toBeHidden();
  await expect(retired).not.toContainText("DNF");
});
