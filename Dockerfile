FROM registry.access.redhat.com/ubi9/ubi:latest

ARG GO_VER=go1.25.0
ARG GINKGO_VER=ginkgo@v2.23.4
ARG CONTAINERUSER=testuser

LABEL description="eco-gotests development image"
LABEL go.version=${GO_VER}
LABEL ginkgo.version=${GINKGO_VER}
LABEL container.user=${CONTAINERUSER}

ENV PATH "$PATH:/usr/local/go/bin:/root/go/bin"
RUN dnf install -y tar gcc make && \
    dnf clean metadata packages && \
    arch=$(arch | sed s/aarch64/arm64/ | sed s/x86_64/amd64/) && \
    curl -Ls https://go.dev/dl/${GO_VER}.linux-${arch}.tar.gz |tar -C /usr/local -xzf -  && \
    useradd -U -u 1000 -m -d /home/${CONTAINERUSER} -s /usr/bin/bash ${CONTAINERUSER}

USER ${CONTAINERUSER}
WORKDIR /home/${CONTAINERUSER}
RUN go install github.com/onsi/ginkgo/v2/${GINKGO_VER}
COPY --chown=${CONTAINERUSER}:${CONTAINERUSER} . .

ENTRYPOINT ["scripts/test-runner.sh"]
