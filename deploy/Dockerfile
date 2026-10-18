FROM scratch
ARG REVISION
LABEL org.opencontainers.image.title="hyjs" org.opencontainers.image.revision=$REVISION
COPY app/ /app/
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
USER 65532:65532
ENTRYPOINT ["/app/hyjs-server"]
