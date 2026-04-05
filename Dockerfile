FROM golang:1.23 AS builder
WORKDIR /src

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/prototypehub ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /out/prototypehub /app/prototypehub
EXPOSE 8080
ENTRYPOINT ["/app/prototypehub"]
