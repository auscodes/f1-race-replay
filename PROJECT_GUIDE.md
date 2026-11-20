# F1 Race Replay project guide

F1 Race Replay lets people study a race moment by replaying car positions on a track map. A shared timeline lets two viewers discuss the same moment, even if one joins midway. Milestone 1 proves that flow locally with two fictional cars and one Go process.

## How it works

```text
Recording → Go picks the current frame → WebSocket → browser draws the cars
```

The browser fetches the track outline, driver IDs, and clip duration once from `/api/session`. It then opens `/ws` and receives the latest positions. The server's start time is shared by all viewers. The browser renders the frames it receives and can freeze its own view in the inspector.

## Where to look in the code

| Start here | What to understand |
| --- | --- |
| [testdata/demo.json](testdata/demo.json) | The recording contains the track outline and 60 sets of car positions, sampled 200 ms apart. |
| [main.go](main.go) | `main` loads the recording and starts the clock. `replayRoutes` wires up the metadata and WebSocket endpoints. |
| [replay.go](replay.go) | `frameAt` calculates the sequence and uses the remainder after dividing by the frame count to select a row in the clip. |
| [stream.go](stream.go) | `serve` sends a full frame as soon as a viewer connects. It schedules later sends against the shared clock and skips ahead if a write falls behind. |
| [web/viewer.js](web/viewer.js) | `showFrame` updates the timer and passes the positions to the canvas. The `ctx.arc` call places each car at its recorded coordinates. |

## Trace one frame

At **11,800 ms** since startup, `frameAt` divides by 200 to get sequence **59**. With 60 saved frames, that picks index **59**, the last row in the recording. At **12,000 ms**, the sequence reaches **60**, the index wraps to **0**, and the browser shows **0:00.0**. The sequence counts frame intervals since startup. The displayed time is the position within the clip.

In the running demo, open **Inspect a frame** and take its sequence modulo 60. Find that row in the `frames` array and compare car `01`'s coordinates with the snapshot.

## Decisions and limits

- **One clock on the server.** A late joiner receives the current frame. Viewers share the same replay time, with small differences in when each browser displays a frame.
- **Full snapshots over WebSocket.** The server can push updates on one connection. A single message contains every car's current position, which makes joining and catching up simple. The trade-off is repeated data on the wire.
- **One process for Milestone 1.** The clock lives in memory. Browsers can reconnect after a server restart, and the replay starts a new timeline. Synchronizing independent servers is a later milestone.

[Replay tests](replay_test.go) check the timing boundaries and wraparound. [WebSocket tests](stream_test.go) verify that two late joiners get the current frame.

**Next milestone.** Import one historical race clip into the same frame format.

Update this guide when a milestone ships. Revise the flow and file list in place, and keep the decisions and limits that still apply.
