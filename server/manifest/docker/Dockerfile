FROM debian:bookworm-slim

###############################################################################
#                                INSTALLATION
###############################################################################

ENV WORKDIR                 /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*
ADD hack                $WORKDIR/hack/
ADD manifest/config                $WORKDIR/manifest/config/
ADD resource                $WORKDIR/resource/
ADD storage                $WORKDIR/storage/
ADD ./temp/linux_amd64/hotgo $WORKDIR/hotgo
ADD ./manifest/docker/entrypoint.sh $WORKDIR/entrypoint.sh
RUN chmod +x $WORKDIR/hotgo
RUN chmod +x $WORKDIR/entrypoint.sh


###############################################################################
#                                   START
###############################################################################

WORKDIR $WORKDIR
CMD ./entrypoint.sh
