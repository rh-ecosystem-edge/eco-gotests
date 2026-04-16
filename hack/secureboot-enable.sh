#!/usr/bin/env bash
# Enables UEFI Secure Boot on all KVM cluster VMs and enrolls the KMM signing key
# into worker nodes' UEFI signature database.
#
# IMPORTANT: The SIGNING_CERT_DER must be the DER-encoded version of the same
# certificate used by the test framework for module signing (kmmparams.SigningCertBase64).
# If these don't match, signed modules will be rejected by the kernel at load time.
#
# To extract the test framework's cert as DER:
#   grep -A1 'SigningCertBase64 =' tests/hw-accel/kmm/internal/kmmparams/const.go | \
#     tail -1 | sed "s/.*\`//" | sed "s/\`.*//" | base64 -d > cdvtest_signing_cert.der
#
# Usage: ./secureboot-enable.sh
#
# This script can run in two modes:
#   1. Locally on the hypervisor (CI mode via job-runner-bash) - omit HYPERVISOR_HOST
#   2. Remotely via SSH - set HYPERVISOR_HOST to the SSH-reachable hostname
#
# Required environment variables:
#   KUBECONFIG         - Path to kubeconfig for the OCP cluster
#   SIGNING_CERT_DER   - Path to signing cert DER file (on the hypervisor). Must match the
#                        cert in kmmparams.SigningCertBase64 (CN=cdvtest signing key).
#
# Optional environment variables:
#   HYPERVISOR_HOST    - SSH-reachable hypervisor hostname (e.g. root@hypervisor.example.com).
#                        If unset, commands run locally (for use on the hypervisor itself).
#   NVRAM_DIR          - NVRAM directory on hypervisor (default: /var/lib/libvirt/qemu/nvram)
#   SECBOOT_TEMPLATE   - Secure boot NVRAM template (default: /usr/share/OVMF/OVMF_VARS.secboot.fd)
#   UEFI_DB_GUID       - GUID for UEFI db enrollment (default: 77fa9abd-0359-4d32-bd60-28f4e78f784b)

set -euo pipefail

: "${KUBECONFIG:?KUBECONFIG must be set}"
: "${SIGNING_CERT_DER:?SIGNING_CERT_DER must be set}"

NVRAM_DIR="${NVRAM_DIR:-/var/lib/libvirt/qemu/nvram}"
SECBOOT_TEMPLATE="${SECBOOT_TEMPLATE:-/usr/share/OVMF/OVMF_VARS.secboot.fd}"
UEFI_DB_GUID="${UEFI_DB_GUID:-77fa9abd-0359-4d32-bd60-28f4e78f784b}"

# Run a command on the hypervisor (locally or via SSH).
run_on_hyp() {
    if [ -n "${HYPERVISOR_HOST:-}" ]; then
        ssh -o StrictHostKeyChecking=no "${HYPERVISOR_HOST}" "$@"
    else
        eval "$@"
    fi
}

wait_node_ready() {
    local node="$1"
    local timeout="${2:-300}"
    echo "Waiting for ${node} to become Ready (timeout ${timeout}s)..."
    oc wait --for=condition=Ready "node/${node}" --timeout="${timeout}s"
}

wait_etcd_healthy() {
    echo "Waiting for etcd to report all members available..."
    local attempts=0
    while [ $attempts -lt 60 ]; do
        local msg
        msg=$(oc get etcd -o=jsonpath='{range .items[0].status.conditions[?(@.type=="EtcdMembersAvailable")]}{.message}{end}' 2>/dev/null || true)
        if echo "${msg}" | grep -q "have a revision"; then
            echo "etcd healthy: ${msg}"
            return 0
        fi
        sleep 5
        attempts=$((attempts + 1))
    done
    echo "WARNING: etcd health check timed out"
    return 1
}

echo "=== Phase 1: Enable Secure Boot on worker nodes ==="

WORKERS=$(oc get nodes -l 'node-role.kubernetes.io/worker=' -o jsonpath='{.items[*].metadata.name}')

for worker in ${WORKERS}; do
    echo "--- Processing worker: ${worker} ---"

    echo "Cordoning ${worker}..."
    oc adm cordon "${worker}"

    echo "Draining ${worker}..."
    oc adm drain "${worker}" --ignore-daemonsets --delete-emptydir-data --force --timeout=120s || true

    echo "Stopping VM ${worker}..."
    run_on_hyp "sudo virsh destroy ${worker}" || true

    echo "Replacing NVRAM with secure boot template..."
    run_on_hyp "sudo cp ${SECBOOT_TEMPLATE} ${NVRAM_DIR}/${worker}.fd"

    echo "Enrolling KMM signing key into UEFI db..."
    run_on_hyp "virt-fw-vars --input ${NVRAM_DIR}/${worker}.fd --output ${NVRAM_DIR}/${worker}.fd --add-db ${UEFI_DB_GUID} ${SIGNING_CERT_DER}"

    echo "Starting VM ${worker}..."
    run_on_hyp "sudo virsh start ${worker}"

    wait_node_ready "${worker}" 300

    echo "Uncordoning ${worker}..."
    oc adm uncordon "${worker}"

    echo "Verifying secure boot on ${worker}..."
    oc debug "node/${worker}" -- chroot /host mokutil --sb-state 2>/dev/null || true

    echo "--- Worker ${worker} done ---"
done

echo ""
echo "=== Phase 2: Enable Secure Boot on master nodes ==="

MASTERS=$(oc get nodes -l 'node-role.kubernetes.io/master=' -o jsonpath='{.items[*].metadata.name}')
if [ -z "${MASTERS}" ]; then
    MASTERS=$(oc get nodes -l 'node-role.kubernetes.io/control-plane=' -o jsonpath='{.items[*].metadata.name}')
fi

for master in ${MASTERS}; do
    echo "--- Processing master: ${master} ---"

    echo "Stopping VM ${master}..."
    run_on_hyp "sudo virsh destroy ${master}" || true

    echo "Replacing NVRAM with secure boot template..."
    run_on_hyp "sudo cp ${SECBOOT_TEMPLATE} ${NVRAM_DIR}/${master}.fd"

    echo "Starting VM ${master}..."
    run_on_hyp "sudo virsh start ${master}"

    wait_node_ready "${master}" 300
    wait_etcd_healthy

    echo "--- Master ${master} done ---"
done

echo ""
echo "=== Phase 3: Verification ==="

ALL_NODES=$(oc get nodes -o jsonpath='{.items[*].metadata.name}')
for node in ${ALL_NODES}; do
    echo "--- ${node} ---"
    oc debug "node/${node}" -- chroot /host mokutil --sb-state 2>/dev/null || echo "  (could not verify)"
done

echo ""
echo "=== Secure Boot enablement complete ==="
