# Build the Discord bot binary in a small distroless-ish image.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/bot ./cmd/bot

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bot /bot
ENTRYPOINT ["/bot"]
