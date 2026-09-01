FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server .

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY templates /app/templates
COPY static /app/static
EXPOSE 8080
CMD ["/app/server"]
