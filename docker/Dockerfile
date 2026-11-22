FROM node:26-alpine AS fe
WORKDIR /src
COPY Taiwan-Dashboard-FE/package.json Taiwan-Dashboard-FE/package-lock.json ./
RUN npm ci
COPY Taiwan-Dashboard-FE/ ./
RUN npm run build

FROM golang:1.27-alpine AS be
WORKDIR /src
COPY Taiwan-Dashboard-BE/go.mod Taiwan-Dashboard-BE/go.sum ./
RUN go mod download
COPY Taiwan-Dashboard-BE/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/dashboardtw ./cmd/dashboardtw

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata postgresql16-client \
    && adduser -D -H dashboardtw \
    && mkdir -p /data/images /data/downloads \
    && chown dashboardtw /data/images /data/downloads
COPY --from=be /out/dashboardtw /usr/local/bin/dashboardtw
COPY --from=fe /src/dist /web
COPY db-sample-data/ /sample/
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
USER dashboardtw
ENV TZ=Asia/Taipei \
    HTTP_ADDR=0.0.0.0:8080 \
    WEB_DIR=/web \
    IMAGE_DIR=/data/images \
    DOWNLOAD_DIR=/data/downloads
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
CMD ["serve"]
