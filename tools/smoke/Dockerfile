FROM golang:1.17.6-buster as build
COPY . /src/agent
WORKDIR /src/agent
ARG RELEASE_BUILD=true
ARG IMAGE_TAG

# Backports repo required to get a libsystemd version 246 or newer which is required to handle journal +ZSTD compression
RUN echo "deb http://deb.debian.org/debian buster-backports main" >> /etc/apt/sources.list
RUN apt-get update && apt-get install -t buster-backports -qy libsystemd-dev

RUN make clean && make IMAGE_TAG=${IMAGE_TAG} RELEASE_BUILD=${RELEASE_BUILD} BUILD_IN_CONTAINER=false agent-smoke

FROM debian:buster-slim

# Backports repo required to get a libsystemd version 246 or newer which is required to handle journal +ZSTD compression
RUN echo "deb http://deb.debian.org/debian buster-backports main" >> /etc/apt/sources.list
RUN apt-get update && apt-get install -t buster-backports -qy libsystemd-dev && \
  apt-get install -qy tzdata ca-certificates && \
  rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY --from=build /src/agent/tools/smoke/grafana-agent-smoke /bin/grafana-agent-smoke

ENTRYPOINT ["/bin/grafana-agent-smoke"]