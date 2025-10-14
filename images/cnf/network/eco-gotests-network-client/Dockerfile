# Build testcmd bin
FROM docker.io/library/golang:1.23 AS builder
RUN git clone https://github.com/kononovn/testcmd.git
WORKDIR ./testcmd
ENV CGO_ENABLED=0
RUN make build

# Create bbdev_builder with DPDK project in order to further take bbdev tool from there
FROM quay.io/centos/centos:stream9 AS bbdev_builder
ENV BUILDER_VERSION 0.1
ENV DPDK_VERSION=24.11
ENV DPDK_DOWNLOAD=https://fast.dpdk.org/rel/dpdk-${DPDK_VERSION}.tar.xz
RUN dnf install -y wget python3.11 python3.11-pip \
 numactl \
 numactl-devel \
 make \
 logrotate \
 ethtool \
 patch \
 which \
 readline-devel \
 iproute \
 libibverbs \
 lua \
 git \
 gcc \
 pciutils \
 xz \
 libibverbs-devel \
 expect && dnf clean all
RUN pip3.11 install meson ninja pyelftools
WORKDIR /dpdk-prepare
RUN wget ${DPDK_DOWNLOAD} && tar xf dpdk-${DPDK_VERSION}.tar.xz && cd dpdk-${DPDK_VERSION} && meson setup build && cd build  && meson configure -Denable_docs=false -Dplatform=generic -Dmax_ethports=32 -Dmax_numa_nodes=8 -Dtests=false && ninja



FROM registry.access.redhat.com/ubi9/ubi:latest

LABEL description="eco-gotests cnf network test client image"
RUN dnf install -y python3 numactl-libs nginx iproute iputils procps-ng ethtool shadow-utils libpcap net-tools nmap
RUN curl -s https://mirror.stream.centos.org/9-stream/AppStream/x86_64/os/Packages/tcpdump-4.99.0-9.el9.x86_64.rpm -o tcpdump-4.99.0-9.el9.x86_64.rpm
RUN rpm -i tcpdump-4.99.0-9.el9.x86_64.rpm
RUN dnf clean all
RUN echo 'server {location /clientip {default_type text/plain;return 200 "Client IP Address: $remote_addr\n";} location /hostname {default_type text/plain;return 200 "You reached host: $hostname\n";} location /serverip {default_type text/plain;return 200 "Server IP Address: $server_addr\n";} location /checknet {default_type text/plain;return 200 "Client IP: $remote_addr\nServer Hostname: $hostname\nServer IP: $server_addr\n";}}' > /etc/nginx/conf.d/custom.conf
COPY --from=builder /go/testcmd/bin/testcmd /usr/bin/testcmd
ENV DPDK_VERSION=24.11
ENV LD_LIBRARY_PATH=/usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_bbdev.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_eal.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_kvargs.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_telemetry.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_mbuf.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_mempool.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_ring.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/lib/librte_pci.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/drivers/librte_bus_vdev.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/drivers/librte_baseband_fpga_lte_fec.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/drivers/librte_bus_pci.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/drivers/librte_baseband_fpga_5gnr_fec.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/drivers/librte_baseband_acc.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/drivers/librte_mempool_ring.so.25 /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/app/test-bbdev/test-bbdev.py /usr/bbdev/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/app/test-bbdev/test_vectors/* /usr/bbdev/test_vectors/
COPY --from=bbdev_builder /dpdk-prepare/dpdk-${DPDK_VERSION}/build/app/dpdk-test-bbdev /usr/bbdev/
