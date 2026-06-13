FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 纯 Go 静态二进制（modernc.org/sqlite + glebarez/sqlite）
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/geokeep ./cmd/geokeep
RUN mkdir -p /out/data

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/geokeep /geokeep
COPY --from=build --chown=65532:65532 /out/data /data
ENV GEOKEEP_DATA_DIR=/data \
    GEOKEEP_LISTEN=:8080
EXPOSE 8080
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/geokeep"]
CMD ["serve"]
