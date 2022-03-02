#!/bin/bash

#Basic build for exporter and server, and push to gcr
# docker build . -f Dockerfile.exporter -t gcr.io/lively-video/goexporter-stats-exporter
docker build . -f Dockerfile.server -t gcr.io/lively-video/goexporter-stats-server
# docker push gcr.io/lively-video/goexporter-stats-exporter
docker push gcr.io/lively-video/goexporter-stats-server
