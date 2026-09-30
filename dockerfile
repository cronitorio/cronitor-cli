FROM alpine:latest

# BuildKit supplies the target architecture, including when cross-building.
ARG TARGETARCH

# Install dependencies
RUN apk add --no-cache \
    curl \
    bash \
    ca-certificates \
    dcron

# Install the CLI for the container's architecture, extracting the release archive.
RUN case "${TARGETARCH}" in amd64|arm64|arm) ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; esac && \
    curl -fsSL "https://cronitor.io/dl/linux_${TARGETARCH}.tar.gz" -o /tmp/cronitor.tar.gz && \
    tar -xzf /tmp/cronitor.tar.gz -C /usr/local/bin cronitor && \
    chmod +x /usr/local/bin/cronitor && \
    rm /tmp/cronitor.tar.gz

# Supply CRONITOR_DASH_USER and CRONITOR_DASH_PASS at runtime.

EXPOSE 9000

CMD ["cronitor", "dash", "--port", "9000"]
