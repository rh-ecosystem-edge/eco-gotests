#!/usr/bin/env bash
# Lab helper for MNO O-RAN provision (77394): watch hub/spoke install progress and
# optionally apply the machine-api finalize workaround before the O2IMS ~1h30m install timeout.
#
# Usage:
#   export ECO_CNF_RAN_KUBECONFIG_HUB=/path/to/hub.kubeconfig
#   export ECO_CNF_RAN_SPOKE1_NAME=kni-qe-26
#   ./oran-mno-install-watch.sh              # poll every 60s, report only
#   ./oran-mno-install-watch.sh --fix        # apply workaround when trap is detected
#   ./oran-mno-install-watch.sh --once       # single snapshot
#
# Spoke kubeconfig defaults to /tmp/<spoke>-admin.kubeconfig and is pulled from the hub
# secret when missing.

set -uo pipefail

SPOKE_NAME="${ECO_CNF_RAN_SPOKE1_NAME:-kni-qe-26}"
PR_NAME="${ORAN_TEST_PR_NAME:-9c5372f3-ea1d-4a96-8157-b3b874a55cf9}"
HUB_KUBECONFIG="${ECO_CNF_RAN_KUBECONFIG_HUB:-${KUBECONFIG:-}}"
SPOKE_KUBECONFIG="${ORAN_SPOKE_KUBECONFIG:-/tmp/${SPOKE_NAME}-admin.kubeconfig}"
INTERVAL="${ORAN_WATCH_INTERVAL:-60}"
EXPECTED_NODES="${ORAN_EXPECTED_NODE_COUNT:-5}"
FIX_MODE=false
ONCE=false

usage() {
	sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
}

log() {
	printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*"
}

warn() {
	printf '[%s] WARNING: %s\n' "$(date -u +%H:%M:%SZ)" "$*" >&2
}

hub_oc() {
	KUBECONFIG="${HUB_KUBECONFIG}" oc "$@"
}

spoke_oc() {
	KUBECONFIG="${SPOKE_KUBECONFIG}" oc "$@"
}

parse_args() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--fix) FIX_MODE=true ;;
		--once) ONCE=true ;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			echo "unknown argument: $1" >&2
			usage
			exit 2
			;;
		esac
		shift
	done
}

require_hub_kubeconfig() {
	if [[ -z "${HUB_KUBECONFIG}" ]]; then
		echo "set ECO_CNF_RAN_KUBECONFIG_HUB or KUBECONFIG to the hub kubeconfig" >&2
		exit 1
	fi
	if ! hub_oc whoami >/dev/null 2>&1; then
		echo "hub kubeconfig is not usable: ${HUB_KUBECONFIG}" >&2
		exit 1
	fi
}

ensure_spoke_kubeconfig() {
	if [[ -f "${SPOKE_KUBECONFIG}" ]] && spoke_oc whoami >/dev/null 2>&1; then
		return 0
	fi

	if ! hub_oc get secret "${SPOKE_NAME}-admin-kubeconfig" -n "${SPOKE_NAME}" >/dev/null 2>&1; then
		return 1
	fi

	log "pulling spoke admin kubeconfig to ${SPOKE_KUBECONFIG}"
	hub_oc get secret "${SPOKE_NAME}-admin-kubeconfig" -n "${SPOKE_NAME}" \
		-o jsonpath='{.data.kubeconfig}' | base64 -d >"${SPOKE_KUBECONFIG}"
	chmod 600 "${SPOKE_KUBECONFIG}"
}

print_hub_status() {
	local aci_state aci_progress pr_phase pr_details agents_done agents_total mc_import

	if ! hub_oc get namespace "${SPOKE_NAME}" >/dev/null 2>&1; then
		log "hub: namespace ${SPOKE_NAME} not found yet"
		return 0
	fi

	aci_state="$(hub_oc get aci "${SPOKE_NAME}" -n "${SPOKE_NAME}" -o jsonpath='{.status.debugInfo.stateInfo}' 2>/dev/null || true)"
	aci_progress="$(hub_oc get aci "${SPOKE_NAME}" -n "${SPOKE_NAME}" -o jsonpath='{.status.progress.totalPercentage}' 2>/dev/null || true)"
	pr_phase="$(hub_oc get pr "${PR_NAME}" -o jsonpath='{.status.provisioningStatus.provisioningPhase}' 2>/dev/null || true)"
	pr_details="$(hub_oc get pr "${PR_NAME}" -o jsonpath='{.status.provisioningStatus.provisioningDetails}' 2>/dev/null || true)"
	agents_done="$(hub_oc get agents -A --no-headers 2>/dev/null | awk -v c="${SPOKE_NAME}" '$3==c && toupper($6)=="DONE" {n++} END {print n+0}')"
	agents_total="$(hub_oc get agents -A --no-headers 2>/dev/null | awk -v c="${SPOKE_NAME}" '$3==c {n++} END {print n+0}')"
	mc_import="$(hub_oc get managedcluster "${SPOKE_NAME}" -o jsonpath='{.status.conditions[?(@.type=="ManagedClusterImportSucceeded")].status}' 2>/dev/null || echo "missing")"

	log "hub: ACI state=${aci_state:-n/a} progress=${aci_progress:-n/a}% agents=${agents_done}/${agents_total} MC import=${mc_import}"
	log "hub: PR phase=${pr_phase:-n/a} details=${pr_details:-n/a}"
}

print_spoke_status() {
	local ready_nodes machine_api_avail cvo_avail

	if ! ensure_spoke_kubeconfig; then
		log "spoke: admin kubeconfig not available yet"
		return 0
	fi

	ready_nodes="$(spoke_oc get nodes --no-headers 2>/dev/null | awk '$2=="Ready"{c++} END{print c+0}')"
	machine_api_avail="$(spoke_oc get co machine-api -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || true)"
	cvo_avail="$(spoke_oc get clusterversion version -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || true)"

	log "spoke: ready_nodes=${ready_nodes}/${EXPECTED_NODES} machine-api=${machine_api_avail:-n/a} clusterversion=${cvo_avail:-n/a}"
	log "spoke: machines:"
	spoke_oc get machines -n openshift-machine-api -o custom-columns=\
NAME:.metadata.name,PHASE:.status.phase,NODE:.status.nodeRef.name 2>/dev/null || true
	log "spoke: bmhs:"
	spoke_oc get bmh -n openshift-machine-api -o custom-columns=\
NAME:.metadata.name,STATE:.status.provisioning.state,CONSUMER:.spec.consumerRef.name 2>/dev/null || true
}

trap_detected() {
	local trap=false

	if ! ensure_spoke_kubeconfig; then
		return 1
	fi

	local ready_nodes provisioning provisioned empty_state need_link
	ready_nodes="$(spoke_oc get nodes --no-headers 2>/dev/null | awk '$2=="Ready"{c++} END{print c+0}')"
	provisioning="$(spoke_oc get machines -n openshift-machine-api --no-headers 2>/dev/null | awk '$2=="Provisioning"{c++} END{print c+0}')"
	provisioned="$(spoke_oc get machines -n openshift-machine-api --no-headers 2>/dev/null | awk '$2=="Provisioned"{c++} END{print c+0}')"
	empty_state="$(spoke_oc get bmh -n openshift-machine-api -o json 2>/dev/null | jq -r '[.items[] | select((.status.provisioning.state // "") == "")] | length')"
	need_link="$(spoke_oc get machines -n openshift-machine-api -o json 2>/dev/null | jq -r '[.items[] | select(.status.phase=="Provisioned" and (.status.nodeRef.name // "") == "")] | length')"

	if [[ "${ready_nodes}" -ge "${EXPECTED_NODES}" && "${provisioning}" -gt 0 && "${empty_state}" -gt 0 ]]; then
		trap=true
		log "trap: nodes Ready but Machines Provisioning with empty BMH provisioning.state"
	fi

	if [[ "${ready_nodes}" -ge "${EXPECTED_NODES}" && "${need_link}" -gt 0 ]]; then
		trap=true
		log "trap: Machines Provisioned without nodeRef while nodes are Ready"
	fi

	if [[ "${trap}" == true ]]; then
		return 0
	fi
	return 1
}

apply_bmh_status_fix() {
	log "applying BMH status patch on spoke"
	spoke_oc get bmh -n openshift-machine-api -o json | jq -r '.items[].metadata.name' | while read -r bmh; do
		[[ -z "${bmh}" ]] && continue
		state="$(spoke_oc get bmh "${bmh}" -n openshift-machine-api -o jsonpath='{.status.provisioning.state}')"
		if [[ -n "${state}" ]]; then
			log "  skip ${bmh}: state already ${state}"
			continue
		fi
		uid="$(spoke_oc get bmh "${bmh}" -n openshift-machine-api -o jsonpath='{.metadata.uid}')"
		spoke_oc patch bmh "${bmh}" -n openshift-machine-api --type=merge --subresource=status -p "{
			\"status\": {
				\"operationalStatus\": \"OK\",
				\"errorMessage\": \"\",
				\"poweredOn\": true,
				\"provisioning\": {
					\"state\": \"externally provisioned\",
					\"ID\": \"${uid}\"
				}
			}
		}"
		log "  patched ${bmh}"
	done
}

apply_machine_node_link_fix() {
	log "linking Machines to Nodes on spoke"
	spoke_oc get bmh -n openshift-machine-api -o json | jq -r '.items[] | [.spec.consumerRef.name, .metadata.name] | @tsv' |
		while IFS=$'\t' read -r machine node; do
			[[ -z "${machine}" || -z "${node}" ]] && continue

			phase="$(spoke_oc get machine "${machine}" -n openshift-machine-api -o jsonpath='{.status.phase}')"
			node_ref="$(spoke_oc get machine "${machine}" -n openshift-machine-api -o jsonpath='{.status.nodeRef.name}')"
			if [[ "${phase}" == "Running" && -n "${node_ref}" ]]; then
				log "  skip ${machine}: already Running on ${node_ref}"
				continue
			fi

			provider_id="$(spoke_oc get machine "${machine}" -n openshift-machine-api -o jsonpath='{.spec.providerID}')"
			node_uid="$(spoke_oc get node "${node}" -o jsonpath='{.metadata.uid}')"
			if [[ -z "${provider_id}" || -z "${node_uid}" ]]; then
				warn "  skip ${machine}: missing providerID or node uid for ${node}"
				continue
			fi

			spoke_oc patch node "${node}" --type=merge -p "{
				\"metadata\": {
					\"annotations\": {
						\"machine.openshift.io/machine\": \"openshift-machine-api/${machine}\"
					}
				},
				\"spec\": {
					\"providerID\": \"${provider_id}\"
				}
			}"

			spoke_oc patch machine "${machine}" -n openshift-machine-api --type=merge --subresource=status -p "{
				\"status\": {
					\"phase\": \"Running\",
					\"nodeRef\": {
						\"kind\": \"Node\",
						\"name\": \"${node}\",
						\"uid\": \"${node_uid}\"
					}
				}
			}"
			log "  linked ${machine} -> ${node}"
		done
}

maybe_apply_fix() {
	if ! trap_detected; then
		return 0
	fi

	if [[ "${FIX_MODE}" != true ]]; then
		warn "trap detected; rerun with --fix to apply the lab workaround"
		return 0
	fi

	apply_bmh_status_fix
	sleep 10
	apply_machine_node_link_fix
	log "workaround applied; waiting for operators to reconcile"
}

snapshot() {
	print_hub_status
	print_spoke_status
	maybe_apply_fix
}

main() {
	parse_args "$@"
	require_hub_kubeconfig

	if ! command -v jq >/dev/null 2>&1; then
		echo "jq is required" >&2
		exit 1
	fi

	log "watching spoke=${SPOKE_NAME} pr=${PR_NAME} fix_mode=${FIX_MODE}"

	if [[ "${ONCE}" == true ]]; then
		snapshot
		exit 0
	fi

	while true; do
		snapshot

		pr_phase="$(hub_oc get pr "${PR_NAME}" -o jsonpath='{.status.provisioningStatus.provisioningPhase}' 2>/dev/null || true)"
		if [[ "${pr_phase}" == "fulfilled" ]]; then
			log "PR fulfilled; exiting"
			exit 0
		fi
		if [[ "${pr_phase}" == "failed" ]]; then
			warn "PR failed; exiting"
			exit 1
		fi

		sleep "${INTERVAL}"
	done
}

main "$@"
