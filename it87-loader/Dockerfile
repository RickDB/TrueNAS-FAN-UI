FROM debian:bookworm-slim

ARG IT87_REF=master

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bc \
        bison \
        build-essential \
        ca-certificates \
        flex \
        git \
        kmod \
        libelf-dev \
        libssl-dev \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 --branch "${IT87_REF}" https://github.com/frankcrawford/it87.git /opt/it87 \
    && git -C /opt/it87 rev-parse HEAD > /opt/it87/.source-rev

COPY it87-loader/entrypoint.sh /usr/local/bin/it87-loader
RUN chmod +x /usr/local/bin/it87-loader

ENTRYPOINT ["/usr/local/bin/it87-loader"]
