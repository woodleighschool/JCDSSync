FROM ghcr.io/linuxserver/baseimage-alpine:3.20

# set version label
LABEL maintainer="hydazz"

RUN \
  echo "**** install runtime packages ****" && \
  apk add --no-cache \
    python3 \
	py3-apscheduler \
	py3-requests

# copy local files
COPY root/ /

# ports and volumes
VOLUME /packages
