# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build

ARG VERSION=dev
ARG REVISION=none
ARG CREATED=unknown
ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-s -w -X github.com/Simplarix/dropwatch/internal/version.Version=${VERSION} -X github.com/Simplarix/dropwatch/internal/version.Commit=${REVISION} -X github.com/Simplarix/dropwatch/internal/version.BuildTime=${CREATED}" \
    -o /out/dropwatch ./cmd/dropwatch
RUN install -d -m 0755 /rootfs/etc/ssl/certs /rootfs/data \
    && install -d -m 1777 /rootfs/tmp \
    && cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ \
    && printf 'dropwatch:x:10001:10001:DropWatch:/data:/sbin/nologin\n' > /rootfs/etc/passwd \
    && printf 'dropwatch:x:10001:\n' > /rootfs/etc/group \
    && chown 10001:10001 /rootfs/data

FROM scratch

ARG VERSION=dev
ARG REVISION=none
ARG CREATED=unknown
ARG SOURCE=https://github.com/Simplarix/dropwatch

LABEL org.opencontainers.image.title="DropWatch" \
      org.opencontainers.image.description="RDAP and DNS domain monitoring worker" \
      org.opencontainers.image.source="${SOURCE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

COPY --from=build /rootfs /
COPY --from=build --chown=10001:10001 /out/dropwatch /usr/local/bin/dropwatch

USER 10001:10001
WORKDIR /data

ENV DROPWATCH_DATABASE_PATH=/data/dropwatch.db \
    DROPWATCH_LOG_FORMAT=json \
    DROPWATCH_TIMEZONE=UTC

VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/dropwatch"]
CMD ["watch"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=20s --retries=3 \
    CMD ["/usr/local/bin/dropwatch", "health", "--quiet"]
