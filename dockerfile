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

# Install the dashboard, released next to the CLI.
RUN curl -fsSL "https://github.com/cronitorio/cronitor-cli/releases/latest/download/crontab-dashboard_linux_${TARGETARCH}.tar.gz" -o /tmp/dashboard.tar.gz && \
    tar -xzf /tmp/dashboard.tar.gz -C /usr/local/bin crontab-dashboard && \
    chmod +x /usr/local/bin/crontab-dashboard && \
    rm /tmp/dashboard.tar.gz

# Supply CRONITOR_DASH_USER and CRONITOR_DASH_PASS at runtime.

EXPOSE 9000

CMD ["crontab-dashboard", "--port", "9000"]
