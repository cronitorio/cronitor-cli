FROM alpine:latest

# Build arguments - MUST be provided during build
ARG DASHBOARD_USERNAME
ARG DASHBOARD_PASSWORD

# Install dependencies
RUN apk add --no-cache \
    curl \
    bash \
    ca-certificates \
    dcron

# Install cronitor CLI
RUN curl -sL https://cronitor.io/dl/linux_amd64.tar.gz -o /usr/local/bin/cronitor && \
    chmod +x /usr/local/bin/cronitor

# Configure authentication
RUN cronitor configure --auth-username ${DASHBOARD_USERNAME} --auth-password ${DASHBOARD_PASSWORD}

EXPOSE 9000

CMD ["cronitor", "dash", "--port", "9000"]
