FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/ubi:latest AS fetcher

ARG GO_VER=go1.24.1
RUN dnf install -y tar

ARG TARGETARCH
RUN arch=$(echo ${TARGETARCH} | sed s/aarch64/arm64/ | sed s/x86_64/amd64/) && \
    curl -Ls https://go.dev/dl/${GO_VER}.linux-${arch}.tar.gz | tar -C /usr/local -xzf -

FROM registry.access.redhat.com/ubi9/ubi:latest

# We had issues with extracting the Go tarball using emulation, so this way we
# extract it natively and then copy to the final image with the target
# architecture.
COPY --from=fetcher /usr/local/go /usr/local/go

ARG GO_VER=go1.24.1
ARG GINKGO_VER=ginkgo@v2.22.2
ARG CONTAINERUSER=testuser

LABEL description="eco-gotests development image"
LABEL go.version=${GO_VER}
LABEL ginkgo.version=${GINKGO_VER}
LABEL container.user=${CONTAINERUSER}

ENV PATH "$PATH:/usr/local/go/bin:/root/go/bin"
RUN dnf install -y tar gcc make && \
    dnf clean metadata packages && \
    useradd -U -u 1000 -m -d /home/${CONTAINERUSER} -s /usr/bin/bash ${CONTAINERUSER}

USER ${CONTAINERUSER}
WORKDIR /home/${CONTAINERUSER}
RUN go install github.com/onsi/ginkgo/v2/${GINKGO_VER}
COPY --chown=${CONTAINERUSER}:${CONTAINERUSER} . .

ENTRYPOINT ["scripts/test-runner.sh"]
