FROM node:24-alpine AS web
WORKDIR /src/admin
RUN corepack enable && corepack prepare pnpm@10.18.0 --activate
COPY admin/package.json admin/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY admin/ ./
RUN pnpm build

FROM golang:1.27.3-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY server/ ./server/
RUN CGO_ENABLED=0 go build -trimpath -o /duoyu ./server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S duoyu && adduser -S -G duoyu duoyu && mkdir /data && chown duoyu:duoyu /data
WORKDIR /app
COPY --from=backend /duoyu /app/duoyu
COPY --from=web /src/admin/dist /app/admin/dist
USER duoyu
EXPOSE 8080
CMD ["/app/duoyu"]
