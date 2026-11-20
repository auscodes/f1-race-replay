# Learn F1 Race Replay

Start with the [demo](../../README.md#quick-start) if you want to see the app first. You can read these guides with Docker stopped.

1. [Technology walkthrough](stack.md) follows one race frame to the browser and one viewer through a pod failure. It introduces Go, HTTP, WebSockets, S3, Redis, Kubernetes, Prometheus, KEDA, Grafana, and k6 as they appear.
2. [Architecture and code map](architecture.md) shows how the importer, server, and browser fit together. Use it to trace a seek and see how the map, standings, and events stay aligned.
3. [Redis clock](redis-clock.md) explains how replicas share a start time and what happens when a replica misses a reset.
4. [Scaling and recovery](scaling.md) explains why the app scales on open sockets, what Grafana can show, and how k6 checks whether viewers return to the right race moment. It includes the passing and failing local runs.
5. [Milestones](milestones.md) traces the project from a one-server demo through the full race and the Kubernetes tests. It records where each feature was verified.
