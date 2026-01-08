ARG BASE_IMG=quay.io/ocp-edge-qe/eco-gotests
ARG BASE_TAG=latest

FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/ubi:latest AS fetcher

RUN dnf install -y tar

ARG TARGETARCH

ADD https://get.helm.sh/helm-v3.13.2-linux-${TARGETARCH}.tar.gz /tmp
ADD https://mirror.openshift.com/pub/openshift-v4/${TARGETARCH}/clients/ocp/latest/openshift-client-linux.tar.gz /tmp
ADD https://mirror.openshift.com/pub/openshift-v4/${TARGETARCH}/clients/ocp/stable/openshift-install-linux.tar.gz /tmp

RUN tar -C /tmp/ -zxvf /tmp/helm-v3.13.2-linux-${TARGETARCH}.tar.gz && \
    mv /tmp/linux-${TARGETARCH}/helm /usr/local/bin && \
    tar -C /usr/local/bin -xzf /tmp/openshift-client-linux.tar.gz && \
    tar -C /usr/local/bin -xzf /tmp/openshift-install-linux.tar.gz

FROM ${BASE_IMG}:${BASE_TAG}

# We had issues with extracting tarballs using emulation, so this way we
# extract them natively and then copy to the final image with the target
# architecture.
COPY --from=fetcher /usr/local/bin/helm /usr/local/bin/helm
COPY --from=fetcher /usr/local/bin/oc /usr/local/bin/oc
COPY --from=fetcher /usr/local/bin/kubectl /usr/local/bin/kubectl
COPY --from=fetcher /usr/local/bin/openshift-install /usr/local/bin/openshift-install

USER root
RUN dnf install -y python3-pip jq iputils skopeo && \
    python3 -m pip install https://github.com/ansible/ansible/archive/refs/tags/v2.15.9.tar.gz && \
    dnf clean all && rm -rf /var/cache/yum

USER testuser
RUN pip3 install -r images/system-tests/ran-du/requirements.txt &&\
    ansible-galaxy install -r images/system-tests/ran-du/requirements.yml
