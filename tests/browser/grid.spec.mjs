import { expect, test } from "@playwright/test";

test("mobile full-grid replay updates the race state, comparison, and finishers on seek", async ({ page }) => {
  await page.setViewportSize({width: 390, height: 844});
  const value = (elapsedMs, value) => ({elapsedMs, value});
  const timings = {
    "01": {
      positions: [value(0, "2"), value(2000, "1")],
      gaps: [value(0, "0.521")],
      intervals: [value(0, "0.412")],
      tyres: [{lapStart: 1, lapEnd: 2, startingAge: 0, compound: "MEDIUM"}],
      laps: [value(0, "1"), value(2000, "2")],
      completedLaps: [{elapsedMs: 8000, lap: 2, durationMs: 4000}],
      outcome: {elapsedMs: 8000, status: "finished", position: 1, gap: "0"},
    },
    "16": {
      positions: [value(0, "1"), value(2000, "2")],
      gaps: [value(0, "0"), value(2000, "+1 LAP")],
      laps: [value(0, "1")],
      outcome: {elapsedMs: 10_000, status: "finished", position: 2, gap: "+1 LAP"},
    },
    "99": {
      positions: [value(0, "3")],
      completedLaps: [{elapsedMs: 5000, lap: 1, durationMs: 5000}],
      outcome: {elapsedMs: 11_800, status: "dnf"},
    },
  };
  const raceStates = [value(0, "green"), value(1000, "vsc"), value(3000, "vsc-ending"),
    value(4000, "green"), value(8000, "chequered")];
  const metadata = {raceStates, raceLaps: [value(0, "1"), value(2000, "2")], totalLaps: 2};
  const events = [
    {elapsedMs: 1000, kind: "vsc", label: "Virtual Safety Car deployed"},
    {elapsedMs: 3000, kind: "vsc", label: "Virtual Safety Car ending"},
    {elapsedMs: 4000, kind: "vsc", label: "Track clear"},
    {elapsedMs: 8000, kind: "flag", label: "Chequered flag"},
    {elapsedMs: 10_000, kind: "flag", label: "Second crossing"},
  ];
  await page.route("**/api/session", async (route) => {
    const session = await (await route.fetch()).json();
    session.drivers = ["01", "16", "99"];
    session.sectorBreaks = [4, 8];
    session.grid = {"01": {code: "ONE"}, "16": {code: "TWO"}, "99": {code: "OUT"}};
    session.timeline = {...metadata, events, drivers: {"01": {laps: timings["01"].laps}}};
    await route.fulfill({json: session});
  });
  let releaseTiming;
  const gate = new Promise((resolve) => { releaseTiming = resolve; });
  await page.route("**/api/timing", async (route) => {
    await gate;
    await route.fulfill({json: {...metadata, events, drivers: timings}});
  });
  await page.goto("/");
  await expect(page.locator("#connection")).toHaveText("Local analysis");
  await expect(page.locator("#tower-message")).toBeVisible();
  const state = page.locator("#race-control");
  const lap = page.locator("#race-lap");
  await expect(state).toHaveText("Green flag");
  releaseTiming();

  const rows = page.locator("#tower-rows tr");
  const first = rows.filter({has: page.getByText("ONE", {exact: true})});
  const second = rows.filter({has: page.getByText("TWO", {exact: true})});
  const retired = rows.filter({has: page.getByText("OUT", {exact: true})});
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toContainText("TWO");
  await expect(lap).toHaveText("Lap 1 / 2");
  const comparison = page.locator("#comparison");
  await expect(comparison).toBeVisible();
  const standings = await page.locator("#timing-tower").boundingBox();
  expect(standings.y + standings.height).toBeLessThan((await comparison.boundingBox()).y);
  await expect(page.locator("#sequence")).toBeHidden();
  await expect(first).toContainText("+0.521");
  await expect(first).toContainText("M 0");
  await page.getByLabel("Time difference").selectOption("interval");
  await expect(first).toContainText("+0.412");
  await page.getByLabel("Time difference").selectOption("leader");
  const highlight = first.getByRole("button");
  await highlight.click();
  await expect(highlight).toHaveAttribute("aria-pressed", "true");
  const mapDriver = page.getByRole("combobox", {name: "Highlight driver"});
  await expect(mapDriver).toHaveValue("01");
  await expect(page.locator("#map-driver-stats")).toContainText("P2");
  await mapDriver.selectOption("16");
  await expect(highlight).toHaveAttribute("aria-pressed", "false");
  await expect(second.getByRole("button")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#map-sectors")).toBeVisible();

  await page.getByLabel("Go to lap").selectOption({label: "Lap 2"});
  await expect(page.locator("#elapsed")).toHaveText("0:02.0");
  await page.getByRole("link", {name: "Race events"}).click();
  await page.getByRole("button", {name: /Virtual Safety Car deployed/}).click();
  await expect(state).toHaveText("Virtual Safety Car");
  await page.getByRole("button", {name: /Virtual Safety Car ending/}).click();
  await expect(state).toHaveText("Virtual Safety Car ending");
  await expect(lap).toHaveText("Lap 2 / 2");
  await page.getByRole("button", {name: /Track clear/}).click();
  await expect(state).toHaveText("Green flag");

  await page.getByRole("button", {name: /Chequered flag/}).click();
  await expect(state).toHaveText("Chequered flag");
  await expect(rows.first()).toContainText("ONE");
  await expect(first.getByRole("img", {name: "Finished"})).toBeVisible();
  await expect(first).toContainText("0:04.000");
  await expect(second.getByRole("img", {name: "Finished"})).toBeHidden();
  await expect(second).toContainText("+1 lap");

  const firstPick = page.getByLabel("Driver 1");
  const secondPick = page.getByLabel("Driver 2");
  await expect(firstPick).toHaveValue("01");
  await expect(secondPick.locator('option[value="01"]')).toBeDisabled();
  await secondPick.selectOption("99");
  await expect(page.locator("#gap-legend")).toContainText("Car 99");
  await expect(page.locator("#lap-rows tr").filter({hasText: "Car 99"})).toContainText("0:05.000");
  await expect(page.locator("#elapsed")).toHaveText("0:08.0");
  await firstPick.selectOption("16");
  await expect(page.locator("#lap-rows tr")).toHaveCount(2);
  await expect(page.locator("#lap-rows")).not.toContainText("Car 01");

  await page.getByRole("button", {name: /Second crossing/}).click();
  await expect(second.getByRole("img", {name: "Finished"})).toBeVisible();
  const slider = page.getByRole("slider", {name: "Replay time"});
  await slider.press("End");
  await expect(retired).toContainText("DNF");
  await expect(retired.getByRole("img", {name: "Finished"})).toBeHidden();
  await slider.press("Home");
  await expect(rows.first()).toContainText("TWO");
  await expect(first.getByRole("img", {name: "Finished"})).toBeHidden();
  await expect(retired).not.toContainText("DNF");
  await expect(state).toHaveText("Green flag");
  await expect(lap).toHaveText("Lap 1 / 2");
  await expect(secondPick).toHaveValue("99");
  await expect(page.locator("#lap-rows")).not.toContainText("0:05.000");
});
