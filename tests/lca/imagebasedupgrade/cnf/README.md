# LCA Image-Based Upgrade - CNF

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_LCA_IBU_CNF_WORKLOAD_IMAGE` | `registry.redhat.io/openshift4/ose-hello-openshift-rhel8@sha256:10dca31348f07e1bfb56ee93c324525cceefe27cb7076b23e42ac181e4d1863e` | Container image for IBU workload validation |
| `ECO_LCA_IBU_CNF_KUBECONFIG_TARGET_HUB` | _(empty)_ | Path to kubeconfig for the target hub cluster |
| `ECO_LCA_IBU_CNF_KUBECONFIG_TARGET_SNO` | _(empty)_ | Path to kubeconfig for the target SNO cluster |
| `ECO_LCA_IBU_CNF_WORKLOAD_NS` | `test` | Namespace for IBU workload validation |
| `ECO_LCA_IBU_CNF_WORKLOAD_PV_NS` | `test` | Namespace for IBU persistent volume workload validation |
| `ECO_LCA_IBU_CNF_WORKLOAD_PV_POD` | `test10-0` | Pod name for persistent volume workload validation |
| `ECO_LCA_IBU_CNF_WORKLOAD_PV_FILE` | `/data/test10-0` | File path inside the persistent volume pod to validate |
| `ECO_LCA_IBU_CNF_KCAT_IMAGE` | `quay.io/ocp-edge-qe/kcat` | Container image for kcat (Kafka cat) validation |
| `ECO_LCA_IBU_CNF_KCAT_BROKER` | `kafka.example.com:9092` | Kafka broker address for kcat validation |
| `ECO_LCA_IBU_CNF_KCAT_TOPIC` | `vran-qe` | Kafka topic for kcat validation |
| `ECO_LCA_IBGU_SEED_IMAGE` | `quay.io/ocp-edge-qe/seed-image` | Seed image for IBGU (Image Based Group Upgrade) |
| `ECO_LCA_IBGU_SEED_IMAGE_VERSION` | `4.17.0` | Seed image version for IBGU |
| `ECO_LCA_IBGU_ODAP_CM_NAME` | `oadp-cm` | OADP ConfigMap name for IBGU |
| `ECO_LCA_IBGU_ODAP_CM_NAMESPACE` | `openshift-adp` | OADP ConfigMap namespace for IBGU |
| `ECO_LCA_IBU_CNF_ACM_OPERATOR_NAMESPACE` | `rhacm` | Namespace of the ACM operator |
