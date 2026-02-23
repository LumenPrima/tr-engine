###############################################################################
# trunk-recorder + MQTT Status Plugin — multi-arch (amd64 / arm64)
#
# Based on TheGreatCodeholio/trunk-recorder-mqtt with multiarch fixes.
# Published as ghcr.io/lumenprima/trunk-recorder-mqtt
###############################################################################

###############################################################################
# Stage 1 – compile trunk-recorder + plugins                                  #
###############################################################################
FROM ubuntu:noble-20251001 AS builder

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get -y upgrade && \
    apt-get install --no-install-recommends -y \
    # ── toolchain ────────────────────────────────────────────────────────────
        build-essential cmake git curl pkg-config wget dpkg-dev \
    # ── SDR headers/libs ────────────────────────────────────────────────────
        ffmpeg gnuradio-dev gr-osmosdr libosmosdr-dev \
        libairspy-dev libairspyhf-dev libbladerf-dev libfreesrp-dev \
        libhackrf-dev libmirisdr-dev libuhd-dev libxtrx-dev librtlsdr-dev \
        libsoapysdr-dev \
    # ── generic libs ────────────────────────────────────────────────────────
        libboost-all-dev libcurl4-openssl-dev libgmp-dev liborc-0.4-dev \
        libpaho-mqtt-dev libpaho-mqttpp-dev libpthread-stubs0-dev libsndfile1-dev \
        libssl-dev python3-six openssh-client ca-certificates \
    # ── SoapyPlutoPAPR deps ─────────────────────────────────────────────────
        libiio-dev libad9361-dev libserialport-dev flex bison libxml2-dev soapysdr-tools \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# ───────────────────────── trunk-recorder ────────────────────────────────────
RUN git clone --depth 1 https://github.com/TrunkRecorder/trunk-recorder.git && \
    sed -i 's/^[[:space:]]*#\s*add_subdirectory(plugins\/simplestream)/add_subdirectory(plugins\/simplestream)/' \
        trunk-recorder/CMakeLists.txt && \
    mkdir -p trunk-recorder/build

# ───────────────────────── MQTT plugin ───────────────────────────────────────
RUN git -C trunk-recorder/user_plugins \
       clone --depth 1 https://github.com/TrunkRecorder/tr-plugin-mqtt.git

WORKDIR /src/trunk-recorder/build
RUN cmake .. \
 && make -j"$(nproc)" \
 && make DESTDIR=/newroot install

# ───────────────────────── SoapyPlutoPAPR driver ────────────────────────────
RUN cd /tmp && \
    git clone --depth 1 https://github.com/F5OEO/SoapyPlutoPAPR.git && \
    multiarch="$(dpkg-architecture -qDEB_HOST_MULTIARCH)" && \
    cmake -S SoapyPlutoPAPR -B SoapyPlutoPAPR/build \
          -DCMAKE_INSTALL_PREFIX=/usr \
          -DCMAKE_INSTALL_LIBDIR="lib/${multiarch}" && \
    cmake --build SoapyPlutoPAPR/build -- -j"$(nproc)" && \
    DESTDIR=/newroot cmake --install SoapyPlutoPAPR/build && \
    rm -rf SoapyPlutoPAPR

###############################################################################
# Stage 2 – lightweight runtime image                                         #
###############################################################################
FROM ubuntu:noble-20251001

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get -y upgrade && \
    apt-get install --no-install-recommends -y \
    # ── trunk-recorder runtime deps ──────────────────────────────────────────
        ca-certificates curl wget sox fdkaac \
        libboost-chrono1.83.0t64 libboost-log1.83.0 \
        libgnuradio-analog3.10.9t64 libgnuradio-digital3.10.9t64 \
        libgnuradio-filter3.10.9t64 libgnuradio-network3.10.9t64 \
        libgnuradio-osmosdr0.2.0t64 libgnuradio-uhd3.10.9t64 \
        libpaho-mqtt-dev libpaho-mqttpp-dev \
        libairspyhf1 libfreesrp0 librtlsdr2 libxtrx0 \
        libsoapysdr0.8 \
    # ── SoapyPlutoPAPR runtime deps ──────────────────────────────────────────
        libiio0 libad9361-0 \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /usr/share/{doc,man,info} /usr/local/share/{doc,man,info}

COPY --from=builder /newroot /

RUN mkdir -p /etc/gnuradio/conf.d && \
    echo 'log_level = info' > /etc/gnuradio/conf.d/gnuradio-runtime.conf && \
    ldconfig

WORKDIR /app
ENV HOME=/tmp

ENTRYPOINT ["trunk-recorder", "--config=/app/config.json"]
