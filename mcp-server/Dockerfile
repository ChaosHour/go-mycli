FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum go.work ./
COPY mcp-server/go.mod mcp-server/
COPY sqlbot/go.mod sqlbot/
RUN go mod download
COPY . .
RUN go build -o /bin/mcp-server ./mcp-server
RUN go build -o /bin/sqlbot ./sqlbot

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/mcp-server /bin/mcp-server
COPY --from=builder /bin/sqlbot /bin/sqlbot
EXPOSE 8800
CMD ["/bin/mcp-server", "--listen", ":8800", "--mcp-command", "/bin/sqlbot"]

