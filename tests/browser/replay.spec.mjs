import { expect, test } from "@playwright/test";

test("inspecting a frame freezes the display while the live stream continues", async ({ page }) => {
  let liveSequence = -1;
  page.on("websocket", (socket) => {
    if (new URL(socket.url()).pathname !== "/ws") return;
    socket.on("framereceived", ({ payload }) => {
      liveSequence = JSON.parse(payload.toString()).sequence;
    });
  });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Club Circuit" })).toBeVisible();
  const canvas = page.getByRole("img", { name: "Circuit map with car positions" });
  await expect(canvas).toBeVisible();
  await expect(page.getByRole("status")).toHaveText("Receiving replay");

  const inspector = page.getByText("Inspect a frame", { exact: true });
  const payload = page.locator("#payload");
  const sequence = page.locator("#sequence");
  await inspector.click();
  await expect(payload).toContainText('"positions"');
  const captured = JSON.parse(await payload.innerText());
  expect(captured.positions.map(({ driver }) => driver)).toEqual(["01", "07"]);
  const frozenMap = await canvas.evaluate((element) => element.toDataURL());

  await expect.poll(() => liveSequence).toBeGreaterThan(captured.sequence);
  await expect(sequence).toHaveText(String(captured.sequence));
  expect(JSON.parse(await payload.innerText())).toEqual(captured);
  const currentMap = await canvas.evaluate((element) => element.toDataURL());
  expect(currentMap === frozenMap, "the map should keep showing the captured frame").toBe(true);

  await page.getByRole("button", { name: "Capture latest frame" }).click();
  const refreshed = JSON.parse(await payload.innerText());
  expect(refreshed.sequence).toBeGreaterThan(captured.sequence);
  await expect(sequence).toHaveText(String(refreshed.sequence));

  await inspector.click();
  await expect(payload).toBeHidden();
  await expect.poll(async () => Number(await sequence.innerText())).toBeGreaterThan(refreshed.sequence);
});

test("the viewer reconnects and displays a newer frame after a socket closes", async ({ page }) => {
  const connections = [];
  await page.routeWebSocket("**/ws", (socket) => {
    connections.push(socket);
    socket.connectToServer();
  });

  await page.goto("/");
  const status = page.getByRole("status");
  const sequence = page.locator("#sequence");
  await expect(status).toHaveText("Receiving replay");
  const beforeDisconnect = Number(await sequence.innerText());

  await connections[0].close({ code: 1001 });
  await expect.poll(() => connections.length).toBeGreaterThan(1);
  await expect(status).toHaveText("Receiving replay");
  await expect.poll(async () => Number(await sequence.innerText())).toBeGreaterThan(beforeDisconnect);
});

test("the viewer shows a useful error when session metadata is unavailable", async ({ page }) => {
  await page.route("**/api/session", (route) => route.fulfill({ status: 503 }));
  await page.goto("/");
  await expect(page.getByRole("status")).toHaveText("Could not load the recording. Refresh to retry.");
});
