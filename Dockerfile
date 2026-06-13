FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 纯 Go 静态二进制（modernc.org/sqlite + glebarez/sqlite）
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/geokeep ./cmd/geokeep
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/geokeep-entrypoint ./cmd/geokeep-entrypoint
RUN mkdir -p /out/data

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/geokeep /geokeep
COPY --from=build /out/geokeep-entrypoint /geokeep-entrypoint
COPY --from=build --chown=65532:65532 /out/data /data
ENV GEOKEEP_DATA_DIR=/data \
    GEOKEEP_LISTEN=:8080
EXPOSE 8080
VOLUME ["/data"]
# 入口先以 root 修正 bind mount 权限，然后降权到非 root UID/GID 再执行 geokeep。
USER 0:0
ENTRYPOINT ["/geokeep-entrypoint"]
CMD ["serve"]
