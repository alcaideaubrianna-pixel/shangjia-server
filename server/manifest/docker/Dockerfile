FROM debian:bookworm-slim

###############################################################################
#                                INSTALLATION
###############################################################################

ENV WORKDIR=/app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*
COPY hack $WORKDIR/hack/
COPY manifest/config $WORKDIR/manifest/config/
COPY resource $WORKDIR/resource/
COPY storage $WORKDIR/storage/
COPY --chmod=755 ./temp/linux_amd64/hotgo $WORKDIR/hotgo
COPY --chmod=755 ./manifest/docker/entrypoint.sh $WORKDIR/entrypoint.sh


###############################################################################
#                                   START
###############################################################################

WORKDIR $WORKDIR
ENTRYPOINT ["./entrypoint.sh"]
