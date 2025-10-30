FROM gcr.io/distroless/static:nonroot

ARG TARGETOS
ARG TARGETARCH

ADD bin/${TARGETOS}_${TARGETARCH}/provider /usr/local/bin/provider
ADD package.yaml /package.yaml

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/provider"]