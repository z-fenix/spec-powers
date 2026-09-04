# spec-powers 服务端构建镜像
# 阶段 1：构建前端（React + Vite 产物 dist/）
FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# 阶段 2：构建后端二进制（spd 服务端 + sp 命令行）
FROM golang:1.27-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/spd ./cmd/spd \
 && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/sp ./cmd/sp

# 阶段 3：运行时镜像——单二进制 + 内置前端静态资源
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 spd
COPY --from=backend /out/spd /usr/local/bin/spd
COPY --from=backend /out/sp /usr/local/bin/sp
COPY --from=frontend /app/dist /srv/static
ENV SP_STATIC_DIR=/srv/static \
    SP_ADDR=:8080
USER spd
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/spd"]
