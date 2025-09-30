# syntax=docker/dockerfile:1

FROM docker.io/library/python:3.12-alpine3.22

ENV PACKAGES_DIR=/packages

USER root

COPY requirements.txt pyproject.toml jamfsync.py /app/
WORKDIR /app

RUN \
	pip3 install --no-cache-dir -r \
		requirements.txt \
	&& \
	pip3 install --no-cache-dir \
		. \
	&& \
	mkdir -p /packages \
	&& \
	rm -rf /tmp/*

USER nobody:nogroup

CMD ["jcdssync"]