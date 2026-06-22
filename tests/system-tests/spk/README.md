# SPK

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_SYSTEM_SPK_WORKLOAD_NS` | `spk-test` | Namespace for SPK workload deployment |
| `ECO_SYSTEM_SPK_INGRESS_TCP_IPV4_URL` | _(empty)_ | IPv4 URL for TCP ingress testing |
| `ECO_SYSTEM_SPK_INGRESS_UDP_IPV4_URL` | _(empty)_ | IPv4 URL for UDP ingress testing |
| `ECO_SYSTEM_SPK_INGRESS_TCP_IPV6_URL` | _(empty)_ | IPv6 URL for TCP ingress testing |
| `ECO_SYSTEM_SPK_INGRESS_UDP_IPV6_URL` | _(empty)_ | IPv6 URL for UDP ingress testing |
| `ECO_SYSTEM_SPK_WORKLOAD_DCI_DEPLOYEMNT_NAME` | _(empty)_ | Name of the DCI workload deployment |
| `ECO_SYSTEM_SPK_NODES_CREDENTIALS_MAP` | _(empty)_ | BMC credentials map per node (format: `node,user,pass,bmc;...`) |
| `ECO_SYSTEM_SPK_WORKLOAD_DEPLOYMENT_IMAGE` | _(empty)_ | Container image for the SPK workload deployment |
| `ECO_SYSTEM_SPK_BACKEND_DEPLOYMENT_IMAGE` | _(empty)_ | Container image for the SPK backend deployment |
| `ECO_SYSTEM_SPK_WORKLOAD_DEPLOYMENT_NAME` | _(empty)_ | Name of the SPK workload deployment |
| `ECO_SYSTEM_SPK_WORKLOAD_TEST_URL` | _(empty)_ | URL used for workload connectivity tests |
| `ECO_SYSTEM_SPK_WORKLOAD_TEST_PORT` | _(empty)_ | Port used for workload connectivity tests |
| `ECO_SYSTEM_SPK_DATA_NS` | _(empty)_ | Namespace for SPK data plane components |
| `ECO_SYSTEM_SPK_DNS_NS` | _(empty)_ | Namespace for SPK DNS components |
| `ECO_SYSTEM_SPK_UTILITIES_NS` | _(empty)_ | Namespace for SPK utilities |
| `ECO_SYSTEM_COREDNS_NS` | _(empty)_ | Namespace for CoreDNS |
| `ECO_SYSTEM_SPK_DATA_TMM_DEPLOY_NAME` | _(empty)_ | Deployment name for data plane TMM |
| `ECO_SYSTEM_SPK_DNS_TMM_DEPLOY_NAME` | _(empty)_ | Deployment name for DNS TMM |
| `ECO_SYSTEM_SPK_DATA_INGRESS_DEPLOY_NAME` | _(empty)_ | Deployment name for data plane ingress controller |
| `ECO_SYSTEM_SPK_DNS_INGRESS_DEPLOY_NAME` | _(empty)_ | Deployment name for DNS ingress controller |
| `ECO_SYSTEM_SPK_BACKEND_UDP_DEPLOYMENT_IMAGE` | _(empty)_ | Container image for the SPK UDP backend deployment |
