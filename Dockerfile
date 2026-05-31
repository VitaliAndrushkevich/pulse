FROM golang:1.22-alpine AS backend-builder

WORKDIR /src/backend
COPY backend/go.mod ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/pulse ./cmd/pulse

FROM gcr.io/distroless/static-debian12 AS runtime

WORKDIR /app
COPY --from=backend-builder /out/pulse /app/pulse

EXPOSE 8080
ENTRYPOINT ["/app/pulse"]
