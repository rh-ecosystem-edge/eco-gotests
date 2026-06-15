# Telco Verification IPSec System Test Troubleshooting

## Overview
This document is intended to be a brief troubleshooting guide if there are problems
with the Telco Verification IPSec System Tests. Normally, if these tests fail, it
will be because the IPSec tunnel is not correctly established.

IPSec tunnels will have 2 endpoints, and in the context of this testing, one endpoint
will be the SNO cluster, and the other end is usually a Security Gateway. For these
test cases, the Security Gateway is usually a simple RHEL VM running in the lab.

## Telco Verification IPSec System Test cases
Currently, for the SNO use case, these are the test cases:

* ipsec_tunnel_deployment.go - verify the tunnel is established after deployment
* ipsec_packets_snoegress.go - verify packets egress the cluster via the IPSec tunnel
* ipsec_packets_snoingress.go - verify packets ingress the cluster via the IPSec tunnel

### Troubleshooting ipsec_tunnel_deployment test failure
This will most likely be the most common failure and will most likely fail because
the tunnel is not established.

If the tunnel is not established, it will most likely be for one of the following
reasons:

1. The Security Gateway is down, either the server or the tunnel on the server
2. There is no connectivity between the SNO cluster and the Security Gateway 
3. The IP addresses have changed either on the SNO cluster or the Security Gateway

Find details on how to check these in the following sections.

#### Checking the Security Gateway

The remote end of the SNO IPSec tunnel is often-times called a Security Gateway.
Currently the Security Gateway is running on a RHEL VM which can be accessed using
the IP address stored statically in the `secgw_host_ip` parameter in the
`eco-gotests/tests/system-tests/ipsec/internal/ipsecconfig/default.yaml` file.
Set the `ECO_IPSEC_SECGW_HOST_IP` environment variable to override this IP
address in CI.

If it is not possible to connect to the Security Gateway, then it may be down or 
have other issues that need to be fixed. The SNO IPSec tunnel cannot work if the
Security Gateway is not running correctly.

Once connected to the Security Gateway, verify the tunnel is up as explained below
in Appendix A1.

Additionally, check the IP Addresses are correct in the Security Gateway IPSec
configuration file:

```
sudo more /etc/ipsec.d/secgw.conf
conn secgw
    left=10.1.28.190
    leftid=%fromcert
    leftrsasigkey=%cert
    leftsubnet=172.16.123.0/24
    leftcert=north
    rightrsasigkey=%cert
    right=10.1.101.129
    rightid=%fromcert
    authby=rsasig
    auto=add
    ikelifetime=24h
    salifetime=1h
    ikev2=insist
    phase2=esp
    fragmentation=yes
    esp=aes256-sha1
    ike=aes256-sha1
```

The `left` and `leftsubnet` configuration parameters refer to the local configuration,
which in this case is the Security Gateway configuration. The IP in `left` should
be the IP used to ssh to the Security Gateway, and `leftsubnet` should be the subnet
on the Security Gateway `ipsec` interface. The `right` configuration parameter should
be the IP address of the `br-ex` interface on the SNO cluster.

#### Checking connectivity between the Security Gateway and the SNO cluster

It must be possible to ping from the Security Gateway to the SNO cluster using the
SNO IP address configured in the `right` parameter in the Security Gateway IPSec
config file. If not, then it will be impossible to start the IPSec tunnel.

If there is a firewall running on the Security Gateway, it needs to be configured
to open ports for IPSec, as explained in Appendix A4.

#### Checking the SNO cluster IPSec configuration

If the SNO cluster used for IPSec testing is changed, then most likely the IP addresses
will have to be changed.

To change the SNO cluster IP, first update the `iperf3_server_ocp_ips` parameter
in the `eco-gotests/tests/system-tests/ipsec/internal/ipsecconfig/default.yaml`
file. This parameter can be overridden in CI by setting the `ECO_IPSEC_IPERF3_SERVER_OCP_IPS`
environment variable.

Additionally, the SNO cluster Machine Config must be created again and stored in
the SNO cluster ZTP git repo, as explained in Appendix A2.

### Troubleshooting ipsec_packets_snoegress test failure

This test case consists of first running an ssh command on the Security Gateway
to run the `iperf3` command in server mode, waiting for an iperf3 client connection
from the SNO cluster. The test case then creates a workload on the SNO cluster which
runs the `iperf3` application in client mode.

The test case could fail for one of the following reasons:

1. The `iperf3` binary is not available on the Security Gateway
2. A firewall on the Security Gateway is blocking the `iperf3` ports. Refer to Appendix A4.
3. The iperf3 image for the SNO cluster is not available. Refer to Appendix A3.
4. One of the IPs in `eco-gotests/tests/system-tests/ipsec/internal/ipsecconfig/default.yaml`
   is incorrect.

### Troubleshooting ipsec_packets_snoingress test failure

This test case consists of first creating a workload on the SNO cluster which
runs the `iperf3` command in server mode, waiting for an iperf3 client connection
from the Security Gateway. The test case then runs an ssh command on the Security
Gateway to run the `iperf3` application in client mode.

The test case could fail for one of the following reasons:

1. The `iperf3` binary is not available on the Security Gateway
2. A firewall on the Security Gateway is blocking the `iperf3` ports. Refer to Appendix A4.
3. The iperf3 image for the SNO cluster is not available. Refer to Appendix A3.
4. One of the IPs in `eco-gotests/tests/system-tests/ipsec/internal/ipsecconfig/default.yaml`
   is incorrect.

## Environment Variables

The following environment variables configure the IPSec test suite. See
`tests/system-tests/ipsec/internal/ipsecconfig/default.yaml` for default values.

* `ECO_IPSEC_TESTS_IPERF3_IMAGE`: Container image for the iperf3 tool.
* `ECO_IPSEC_SECGW_HOST_IP`: Host IP address of the Security Gateway (for SSH).
* `ECO_IPSEC_SECGW_SERVER_IP`: IPSec tunnel IP of the Security Gateway.
* `ECO_IPSEC_IPERF3_SERVER_OCP_IPS`: OCP cluster IP addresses for iperf3 server.
* `ECO_IPSEC_IPERF3_CLIENT_TX_BYTES`: Number of bytes to transmit in iperf3 client tests.
* `ECO_IPSEC_NODE_PORT`: NodePort used for iperf3 tests.
* `ECO_IPSEC_NODE_PORT_INCREMENT`: Increment applied to the node port between tests.
* `ECO_IPSEC_TESTWORKLOAD_NAMESPACE`: Namespace for the test workload.
* `ECO_IPSEC_TESTWORKLOAD_CREATE_METHOD`: Method used to create the test workload.
* `ECO_IPSEC_TESTWORKLOAD_CREATE_SHELLCMD`: Shell command to create the test workload.
* `ECO_IPSEC_TESTWORKLOAD_DELETE_SHELLCMD`: Shell command to delete the test workload.
* `ECO_SSH_USER`: SSH username for the Security Gateway.
* `ECO_SSH_PRIVATE_KEY`: Path to SSH private key for the Security Gateway.
* `ECO_SSH_PORT`: SSH port for the Security Gateway.

## Appendix

### A1 Checking the IPSec tunnel

The following 3 commands show different information about the IPSec tunnel:

```
sudo ipsec briefstatus
sudo ipsec showstates
sudo ipsec traffic
```

The following command shows much more information, and usually will not be necessary
unless more involved debugging is needed:

```
sudo ipsec briefstatus
```

It must be possible to ping the Security Gateway VPN IP address from the SNO cluster.
Notice this ping must be performed with the correct source IP, as follows:

```
# On the Security Gateway, get the VPN IP Address:
ip a show ipsec
5: ipsec: <NO-CARRIER,POINTOPOINT,MULTICAST,NOARP,UP> mtu 1500 qdisc fq_codel state DOWN group default qlen 500
    link/none 
    inet 172.16.123.10/24 brd 172.16.123.255 scope global noprefixroute ipsec
       valid_lft forever preferred_lft forever
```

```
# On the SNO cluster, get the source IP address:
ip a show br-ex noprefixroute
14: br-ex: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UNKNOWN group default qlen 1000
    link/ether 6c:fe:54:58:ee:e4 brd ff:ff:ff:ff:ff:ff
    inet 10.1.101.129/27 brd 10.1.101.159 scope global noprefixroute br-ex
       valid_lft forever preferred_lft forever

# On the SNO cluster Ping the Security Gateway, via the IPSec tunnel
ping -I 10.1.101.129 172.16.123.10
PING 172.16.123.10 (172.16.123.10) from 10.1.101.129 : 56(84) bytes of data.
64 bytes from 172.16.123.10: icmp_seq=1 ttl=64 time=0.635 ms
64 bytes from 172.16.123.10: icmp_seq=2 ttl=64 time=0.381 ms
64 bytes from 172.16.123.10: icmp_seq=3 ttl=64 time=0.277 ms
```

### A2 Creating the OCP IPSec Machine Config

Complete instructions can be found here:

```
cnf-features-deploy/ztp/source-crs/optional-extra-manifest/README.md
```

It is best to use butane version `0.20.0`.

The most important parts of this is to place the latest certificates in this
directory, and to correctly configure the IP Addresses in the `ipsec-endpoint-config.yml`
file.

Once the Machine Configurations have been generated, store the updated `99-ipsec-master-endpoint-config.yaml`
file in the SNO cluster ZTP git repo.

### A3 The iperf3 ubi image

Since the `iperf3` application is not available on RHCOS, an image was created so
the application can be used for these test cases. This image was created with the
following Dockerfile: `eco-gotests/images/system-tests/ipsec-iperf3/Dockerfile`.

The image url can be found in the `iperf3tool_image` parameter in the
`eco-gotests/tests/system-tests/ipsec/internal/ipsecconfig/default.yaml` file.
This parameter can be overriden with the `ECO_IPSEC_TESTS_IPERF3_IMAGE`
environment variable.

### A4 The Security Gateway firewall

If the `firewall-cmd` command is not found, or there is an error saying that
`firewalld` is not running, then this part can be skipped.

To check the `firewalld` configuration, execute the following command:

```
sudo firewall-cmd --list-all
```

The following command will open ports for ipsec and iperf3:

```
firewall-cmd --add-service="ipsec"
firewall-cmd --add-port=30000/tcp
firewall-cmd --runtime-to-permanent
```

Notice that the port used for iperf3 is configured in the `node_port` parameter
in the `eco-gotests/tests/system-tests/ipsec/internal/ipsecconfig/default.yaml` file.
This parameter can be overridden with the `ECO_IPSEC_NODE_PORT` environment variable.
