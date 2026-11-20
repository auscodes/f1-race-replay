FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY web ./web
COPY testdata/demo.json ./testdata/demo.json
RUN CGO_ENABLED=0 go build -trimpath -o /f1-race-replay .

FROM scratch AS runtime
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /f1-race-replay /f1-race-replay
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/f1-race-replay"]
CMD ["-addr", ":8080"]

FROM runtime AS barcelona
COPY --chmod=0444 --from=recording /barcelona-2026-grid.json /barcelona-2026-grid.json

FROM runtime AS app
