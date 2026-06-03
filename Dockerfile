FROM golang:1.24-bookworm AS builder

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        gcc \
        libc6-dev \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN GOBIN=/out go install github.com/ffuf/ffuf/v2@v2.1.0
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/cyberstrike-ai ./cmd/server

FROM archlinux:base

ENV PYTHONUNBUFFERED=1 \
    CYBERSTRIKE_HTTPS=1 \
    JAVA_HOME="/usr/lib/jvm/default" \
    PATH="/opt/dirsearch/bin:${PATH}"

WORKDIR /app

RUN pacman -Syu --disable-download-timeout --noconfirm --needed \
        bash \
        ca-certificates \
        curl \
        git \
        hydra \
        iproute2 \
        iputils \
        jdk8-openjdk \
        masscan \
        nmap \
        openssl \
        python \
        python-pip \
        python-requests \
        python-setuptools \
        rustscan \
    && python -m venv /opt/dirsearch \
    && /opt/dirsearch/bin/pip install --no-cache-dir --upgrade pip \
    && /opt/dirsearch/bin/pip install --no-cache-dir 'setuptools<81' dirsearch 'httpx>=0.27.0' \
    && archlinux-java set java-8-openjdk \
    && mkdir -p /usr/share/wordlists/dirb \
    && printf '%s\n' \
        admin \
        api \
        assets \
        backup \
        config \
        css \
        dashboard \
        docs \
        images \
        index \
        js \
        login \
        logout \
        robots.txt \
        static \
        upload \
        uploads \
        user \
        users \
        wp-admin \
        wp-content \
        > /usr/share/wordlists/dirb/common.txt \
    && pacman -Scc --noconfirm

COPY --from=builder /out/cyberstrike-ai /app/cyberstrike-ai
COPY --from=builder /out/ffuf /usr/local/bin/ffuf
COPY web /app/web
COPY tools /app/tools
COPY skills /app/skills
COPY agents /app/agents
COPY roles /app/roles
COPY knowledge_base /app/knowledge_base
COPY internal/c2/payload_templates /app/internal/c2/payload_templates

RUN mkdir -p /app/data /app/tmp /app/reports /app/chat_uploads \
    && chmod +x /app/cyberstrike-ai

EXPOSE 8080

ENTRYPOINT ["/app/cyberstrike-ai"]
CMD ["--https", "-config", "/app/config.yaml"]
