FROM debian:trixie-slim

###############################################################################
#                                INSTALLATION
###############################################################################

ENV WORKDIR=/app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*
COPY hack $WORKDIR/hack/
COPY manifest/config $WORKDIR/manifest/config/
RUN cp $WORKDIR/manifest/config/config.example.yaml $WORKDIR/manifest/config/config.yaml
COPY resource $WORKDIR/resource/
COPY storage $WORKDIR/storage/
COPY addons $WORKDIR/addons/
COPY --chmod=755 ./temp/linux_amd64/hotgo $WORKDIR/hotgo
COPY --chmod=755 ./manifest/docker/entrypoint.sh $WORKDIR/entrypoint.sh


###############################################################################
#                                   START
###############################################################################

WORKDIR $WORKDIR
ENTRYPOINT ["./entrypoint.sh"]
