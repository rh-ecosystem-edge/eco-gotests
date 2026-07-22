package kmmparams

const (
	// Label represents kmm that can be used for test cases selection.
	Label = "kmm"

	// KmmLogLevel custom loglevel of KMM related functions.
	KmmLogLevel = 90

	// McoStateDone represents the Machine Config Operator state when a node configuration is applied.
	McoStateDone = "Done"

	// ArchArm64 represents the arm64 architecture identifier.
	ArchArm64 = "arm64"
	// ArchAarch64 represents the aarch64 architecture identifier.
	ArchAarch64 = "aarch64"
	// ArchS390x represents the s390x architecture identifier.
	ArchS390x = "s390x"
	// ArchPpc64le represents the ppc64le architecture identifier.
	ArchPpc64le = "ppc64le"

	// MultistageContents represents the Dockerfile contents for multi stage build.
	MultistageContents = `ARG DTK_AUTO
FROM ${DTK_AUTO} as builder
ARG KERNEL_VERSION
ARG MY_MODULE
WORKDIR /build
RUN git clone https://github.com/cdvultur/kmm-kmod.git
WORKDIR /build/kmm-kmod
RUN cp kmm_ci_a.c {{.Module}}.c
RUN make KVER=${KERNEL_VERSION}

FROM registry.redhat.io/ubi9/ubi-minimal
ARG KERNEL_VERSION
ARG MY_MODULE
RUN microdnf -y install kmod

COPY --from=builder /etc/driver-toolkit-release.json /etc/
COPY --from=builder /build/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_VERSION}/
RUN depmod -b /opt ${KERNEL_VERSION}
`

	// UserDTKContents represents the Dockerfile where user specifies DTK image and Module.
	UserDTKContents = `FROM {{.DTKImage}} as builder
ARG KERNEL_VERSION
ARG MY_MODULE
WORKDIR /build
RUN git clone https://github.com/cdvultur/kmm-kmod.git
WORKDIR /build/kmm-kmod
RUN cp kmm_ci_a.c {{.Module}}.c
RUN make KVER=${KERNEL_VERSION}

FROM registry.redhat.io/ubi9/ubi-minimal
ARG KERNEL_VERSION
ARG MY_MODULE
RUN microdnf -y install kmod

COPY --from=builder /etc/driver-toolkit-release.json /etc/
COPY --from=builder /build/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_VERSION}/
RUN depmod -b /opt ${KERNEL_VERSION}
`
	// SimpleKmodContents represents the Dockerfile contents for simple-kmod build.
	SimpleKmodContents = `ARG DTK_AUTO
FROM ${DTK_AUTO} as builder
ARG KERNEL_VERSION
ARG KMODVER
WORKDIR /build/

RUN git clone https://github.com/cdvultur/simple-kmod.git && \
	cd simple-kmod && \
    make all       KVER=$KERNEL_VERSION KMODVER=$KMODVER && \
    make install   KVER=$KERNEL_VERSION KMODVER=$KMODVER

FROM registry.redhat.io/ubi9/ubi-minimal
ARG KERNEL_VERSION
ARG MY_MODULE
RUN microdnf -y install kmod

COPY --from=builder /etc/driver-toolkit-release.json /etc/
COPY --from=builder /lib/modules/$KERNEL_VERSION/simple-*.ko /opt/lib/modules/${KERNEL_VERSION}/
COPY --from=builder /lib/modules/$KERNEL_VERSION/modules.* /opt/lib/modules/${KERNEL_VERSION}/
RUN depmod -b /opt ${KERNEL_VERSION}
`

	// SecretContents template.
	SecretContents = `
{
  "auths": {
    "{{.Registry}}": {
      "auth": "{{.PullSecret}}",
      "email": ""
    }
  }
}
`
	// SimpleKmodFirmwareContents represents the Dockerfile contents for simple-kmod-firmware build.
	SimpleKmodFirmwareContents = `ARG DTK_AUTO
FROM ${DTK_AUTO} as builder
ARG KERNEL_FULL_VERSION
ARG MOD_NAME
ARG MOD_NAMESPACE
ARG KMODVER

WORKDIR /build/ 
RUN git clone https://github.com/cdvultur/simple-kmod.git && \
   cd simple-kmod && \
   make all       KVER=$KERNEL_FULL_VERSION KMODVER=$KMODVER && \
   make install   KVER=$KERNEL_FULL_VERSION KMODVER=$KMODVER

FROM registry.redhat.io/ubi9/ubi-minimal
ARG KERNEL_FULL_VERSION
ARG MOD_NAME
ARG MOD_NAMESPACE
RUN microdnf -y install kmod

COPY --from=builder /etc/driver-toolkit-release.json /etc/
COPY --from=builder /lib/modules/$KERNEL_FULL_VERSION/simple-*.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/
COPY --from=builder /lib/modules/$KERNEL_FULL_VERSION/modules.* /opt/lib/modules/${KERNEL_FULL_VERSION}/
RUN depmod -b /opt ${KERNEL_FULL_VERSION}

RUN mkdir /firmware
RUN echo -n "simple_kmod_firmware validation string" >> /firmware/simple_kmod_firmware.bin
`
	// LocalMultiStageContents represents the Dockerfile contents for multi stage build using local registry.
	LocalMultiStageContents = `FROM image-registry.openshift-image-registry.svc:5000/openshift/driver-toolkit as builder
ARG KERNEL_FULL_VERSION
ARG MOD_NAME
WORKDIR /build
RUN git clone https://github.com/cdvultur/kmm-kmod.git
WORKDIR /build/kmm-kmod
RUN cp kmm_ci_a.c $MOD_NAME.c
RUN echo "obj-m += $MOD_NAME.o" >> Makefile
RUN make KVER=${KERNEL_FULL_VERSION}

FROM registry.redhat.io/ubi9/ubi-minimal
ARG KERNEL_FULL_VERSION
RUN microdnf -y install kmod

COPY --from=builder /etc/driver-toolkit-release.json /etc/
COPY --from=builder /build/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/
RUN depmod -b /opt ${KERNEL_FULL_VERSION}
`

	// MultiKoContents represents the Dockerfile for building 3 kernel modules for glob signing tests.
	MultiKoContents = `ARG DTK_AUTO
FROM ${DTK_AUTO} as builder
ARG KERNEL_VERSION
WORKDIR /build
RUN git clone https://github.com/cdvultur/kmm-kmod.git
WORKDIR /build/kmm-kmod
RUN cp kmm_ci_a.c test_mod.c
RUN echo "obj-m += test_mod.o" >> Makefile
RUN make KVER=${KERNEL_VERSION}

FROM registry.redhat.io/ubi9/ubi-minimal
ARG KERNEL_VERSION
RUN microdnf -y install kmod

COPY --from=builder /etc/driver-toolkit-release.json /etc/
COPY --from=builder /build/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_VERSION}/
RUN depmod -b /opt ${KERNEL_VERSION}
`

	// MultiKoCustomDirContents represents the Dockerfile for building 3 kernel modules under /custom dir.
	MultiKoCustomDirContents = `ARG DTK_AUTO
FROM ${DTK_AUTO} as builder
ARG KERNEL_VERSION
WORKDIR /build
RUN git clone https://github.com/cdvultur/kmm-kmod.git
WORKDIR /build/kmm-kmod
RUN cp kmm_ci_a.c test_mod.c
RUN echo "obj-m += test_mod.o" >> Makefile
RUN make KVER=${KERNEL_VERSION}

FROM registry.redhat.io/ubi9/ubi-minimal
ARG KERNEL_VERSION
RUN microdnf -y install kmod

COPY --from=builder /etc/driver-toolkit-release.json /etc/
RUN mkdir -p /custom/lib/modules/${KERNEL_VERSION}
COPY --from=builder /build/kmm-kmod/*.ko /custom/lib/modules/${KERNEL_VERSION}/
RUN depmod -b /custom ${KERNEL_VERSION}
`

	// SigningCertCN is the Common Name of the signing certificate used to verify module signatures.
	SigningCertCN = "KMM Secure Boot Signing CA"

	//nolint:lll
	// SigningCertBase64 represents cert used for module signing.
	SigningCertBase64 = `MIIFXzCCA0egAwIBAgIUWAIGkfuVm+MhaciUbKhaYm/vr5gwDQYJKoZIhvcNAQELBQAwNjEPMA0GA1UECgwGS01NLVFFMSMwIQYDVQQDDBpLTU0gU2VjdXJlIEJvb3QgU2lnbmluZyBDQTAgFw0yNjA3MjExMTI2MDVaGA8yMTI2MDYyNzExMjYwNVowNjEPMA0GA1UECgwGS01NLVFFMSMwIQYDVQQDDBpLTU0gU2VjdXJlIEJvb3QgU2lnbmluZyBDQTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBANAbs93H5JuZi5kazv8ciPajuIHG5WqzbIyzXGNOPf5HZWea0sK9dbqUxMaedq/t+U92UNpeYzcCFd3Jb9Hio4qcbdkudAE+ensJGsQddACDJZDxO+egEdpO2yFVAwn/wSRcdBNyvMVHSMGQZBF5hJU6GR3YlsPB/+l6yK3KUSd+lULuZmlAbjVagdep1Q2QEJxQDWFg/9GpnsuPUuMjuj1lz6UW6TucQM/fe8WnNgb/OUOguOIb37NQVfHRLk8cGzsCXxH6ea7SY4d0cBDUayMnbgwOObl6BGDdoJgS5wnDClaJj026W3OkxhyJUbjauaWV8wXMO17o210u9vJaxxtARK2TG+il62xkbN0QFcGenVMfxDxWNKLLPVKCZCWTrgdEIknqH8swks80S8pQeOikveoGtGe0903gPAYqrVr0vc71wcc1d5uHcoodrznW0ZoCQvN+BJmzRe6aKNJt6C4AKVStmjsSNTwWxQ7Ja8rhrAei0LwiAxIUhULyVRDUtc2yCMkBHhKVpQSBP3GJHdI0HjrT1gbPGoi4m4xKwxjO0AEwCdgbWKZForodZmENHOW++lYQBbC8/ifoLO+vHIHwIliQDnBoPendyFXzcN/Ta+jF2zgTdUydyxxOL2ygnA41A2spxrhZLbDseOQ7aVvluiImoLE9j4LEIhH9QPQNAgMBAAGjYzBhMB8GA1UdIwQYMBaAFFA1AU7fU0EqIFiPRWMonL1yJgQ6MA8GA1UdEwEB/wQFMAMBAf8wDgYDVR0PAQH/BAQDAgKEMB0GA1UdDgQWBBRQNQFO31NBKiBYj0VjKJy9ciYEOjANBgkqhkiG9w0BAQsFAAOCAgEAAfIjOymqeNWq+nxQc2MzbHqzUBqAbTEemdkAsB/wqfXmy0JkKeZkdizwBCkisDkR9eimlY9wKkf2OWQQlTK69JCeu+TfGcY5OPeMfIa81yjgsUVxZPJ9DXmrfsLQIfsQwclJXYxGuuFx5aKHAvjMJcrXSpScP+PPEMvFGNULK0lqLkX//LWL9plcToNL6/auPxno5wbilebdWhdvmSfpowyg9VLS0L2fHkFODVlrCX2SJlgphRoFOjA6XfwTra/EdFDCNIgEiqazmEZpqLFPsaqJ3TB6t6OF87oJCZREZk7Ufq6WYsINLefZGo4rbUY2Llm0vmhNwAUdjiWBrWF7oiIqghZpriqNBXbuA3uCJ+nWJLEtphfuoAufkDsb98C1gJ9IbFOVnWA42LVn9zinTtMsCdXQJYedRdvFRvhJqkeKu7J1O/eeuWVBZ/WhQ81q2sjOvxowxJN9QzJF4IXLAsS3elHHyr5EUaVelaKnv1lF6PSfqQKChBvUgBqQOdxjMS2kzblCv0HRaqtp4u7+YsKnT+zoA+cgq6Sasr4dqUDfTq+BglmPfmk0NIXX20conYVGhIs/IiofcezbYUM1mYVzEeb/vmZeSeO1deYdT0Xsdtr69ma4rc+v1sHU+TDgo+vdR3DGZgt0gR1ODgNxKNxUjnYEpBrJr6eoxLaNzk0=`

	//nolint:lll
	// SigningKeyBase64 represents key used for module signing.
	SigningKeyBase64 = `LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JSUpRZ0lCQURBTkJna3Foa2lHOXcwQkFRRUZBQVNDQ1N3d2dna29BZ0VBQW9JQ0FRRFFHN1BkeCtTYm1ZdVoKR3M3L0hJajJvN2lCeHVWcXMyeU1zMXhqVGozK1IyVm5tdExDdlhXNmxNVEdubmF2N2ZsUGRsRGFYbU0zQWhYZAp5Vy9SNHFPS25HM1pMblFCUG5wN0NSckVIWFFBZ3lXUThUdm5vQkhhVHRzaFZRTUovOEVrWEhRVGNyekZSMGpCCmtHUVJlWVNWT2hrZDJKYkR3Zi9wZXNpdHlsRW5mcFZDN21acFFHNDFXb0hYcWRVTmtCQ2NVQTFoWVAvUnFaN0wKajFMakk3bzlaYytsRnVrN25FRFAzM3ZGcHpZRy96bERvTGppRzkrelVGWHgwUzVQSEJzN0FsOFIrbm11MG1PSApkSEFRMUdzakoyNE1Eam01ZWdSZzNhQ1lFdWNKd3dwV2lZOU51bHR6cE1ZY2lWRzQycm1sbGZNRnpEdGU2TnRkCkx2YnlXc2NiUUVTdGt4dm9wZXRzWkd6ZEVCWEJucDFUSDhROFZqU2l5ejFTZ21RbGs2NEhSQ0pKNmgvTE1KTFAKTkV2S1VIam9wTDNxQnJSbnRQZE40RHdHS3ExYTlMM085Y0hITlhlYmgzS0tIYTg1MXRHYUFrTHpmZ1NaczBYdQptaWpTYmVndUFDbFVyWm83RWpVOEZzVU95V3ZLNGF3SG90QzhJZ01TRklWQzhsVVExTFhOc2dqSkFSNFNsYVVFCmdUOXhpUjNTTkI0NjA5WUd6eHFJdUp1TVNzTVl6dEFCTUFuWUcxaW1SYUs2SFdaaERSemx2dnBXRUFXd3ZQNG4KNkN6dnJ4eUI4Q0pZa0E1d2FEM3AzY2hWODNEZjAydm94ZHM0RTNWTW5jc2NUaTlzb0p3T05RTnJLY2E0V1Mydwo3SGprTzJsYjVib2lKcUN4UFkrQ3hDSVIvVUQwRFFJREFRQUJBb0lDQUFyd3JlRzJtTExDWFdlWTFIRnVXVmZuCjd2THBseFZuSmNsMncyQllGR081OHArQjhOcjZkVVl2L0hFNWt1RTRTQzh3ZDlzbTg1M0lid2hZQnRnWWkwTGwKNnROc1FQNXd2NlVZcXk3TW9wVEZVRFFBcm9iRWNEUFRDZXVFYWRLOGZuV1FJNjBERHQvckdhek5UNmxvZ3pzSwpFUmh6MHFjclcyMzFmYmUvSjBtRmVzaklPaHVGM3RWUXN2czRxeUhacFRubWJuR2tWUVo3WFlUemZhYkJzZEI0CitmUENrdFBHcU5TVXVjKy85TlBrMW1pUytpMHV6SEdEZzJVaE8wYys0Y0tXSXhQUm9GUjdyRzVLUlhycEE5SFgKQ0k2N0w3Y3JyaSsrYnVzWHNVd2UwL3dOSXJxeUZZMHNpQnFuY0dPZ29mcE51ZmNmejY0elhSYld1WFl2cGdCVgpLYm9QSU04UW9WR05lVkhHaXMzQVpoRDR2TFAzV2hCbm9SUDJwZzdhTW1BeFNZendkRUI4Z3ptT1lGZDJ1MGtyCkpBWElHK3k0QWVKaTV2bDI2YUMwZVA5REcyNVJZa2xTMWJ6NFNIRmJGd2JBZi9WOXV5Ymg2ekVqUko0bFBKN0IKT0R3RkJIWVlaRnNWRUsvQWxaRk5tOUlWbllYREVCZHUrYm94U0lYTEhjbjBoazdoNGlZZ3NuNlFHTVdLZW96NAozWDMzK3BWcitCZmxGYUVWU0pZbkNmL1BsdEhveWpjUlozT3dsaStMTFVlVlVBTFNnc3VwTFVxYklNYUVMTmJRCnc5ei8zT0tLQlhIUmI1QmY2RjVKNElaMy8ybFpidCtoNElhYkNENGtYaUpqNHVMbEFxSGxZVzV5d29IWk45OTMKMDNVZDBpdE5PTzcvVWJCMTFsTXBBb0lCQVFEMytrUXpGYkJEM2x4S1lLZC90em95NlJ0QzJPVTJYNWxKTmZmcQpreUhWWUJ4RVZud21iL3dEWXEyQWJ5bDFhYTNSZkJUQmVWaVNvWmpOMlU4UkxtOUZBWDRSOUFDUk0weDVlcTM3CjN4R25HaU43M1J4R3ByeDZ6NVFZTWhVOGVRWS9oL0o0ci9lTlZZSVNHR0lHeHFWQlZub0hieHRCcFZncTFxVnYKSmV3bXdzV1pOZktWUEhicjh1NklMVWV4WGFnSkt0U3gyNlk2QWg5L21JV3dreWRJUTVKU2VzQ0RsTHBJdlhyMwpwbWJBVmNEY3JIUXlSUy9WNVNHaXJ3NFdWZUdKNXpCUUZYbEVYQlFMcW9YRWcwZnZOVGJST0JqUXc2TDJyQldRClBXQzNVNEs0TUMxYlArNTBGTzhBOE9acFEwK1BwWktRemlzcTI2aFg3UWJhYU1hbEFvSUJBUURXMXoyWi9mN0QKakZESUFYUWU0WGhYSGN3R0NNVEdzMm5lRHBkODVkYXRTa1JEeVMrdUF6aW0xQmw3L2UzUDF1VXhwZEVHZHZGOQp5LzJTaVFlTEdnRmxDenF1OVY0YnBpSi81RjZKUUtCTVhyeFo4cGdQc2hTbG9UUnlvMUc5Y1UrT0I0UlV5eDhhCkVGZThweDJTbWdKeXdlQXpzdmJ2SUdmM3dFQVNwc21icnNib2N5MHlGb3k0YXVhVCtQSDNHY01KMVBiNWJBMlUKWHFoNitYY1IxOXdCUUpqZzVKbFhFc250dkZwdTVPRVFuMVdWanZ0eHRCVHdkcmlOMzdGODRwRG1JbHRXSklQNQpYTHE2RWFRbzJxc0dnREhIQmMrUHFGbmkyYy8weG9KaCtMMlJXWm45Y3NCMUNlaS90b2tnVzJLdXhPUXl1OVU4CkYzWXFpM0NQak9OSkFvSUJBRWExSHNNdU9QOXhKUUcwUTY2QUVXRTQ1S1FQOG5PcG1LeXViYzErUEpSNS9qVmkKTDY1S0dpTGcvVFgwL0c1VlB0SFB2cDlZT1hBK0ViTUlkcW9nZ211N3ZEWGtURkVhdm9DWkFZa2pGd1o5eG9abwpDc3IrZWhuT25KeTFreWFXSDdqM3k2U3NFRHRGbVh3TlpjNG0wRElzTlVsdlhlYysvVC9oeW5SQjdPODBkR0YyClo0QTBLSGlvNFdrRStEOVR4RGt3OUVydXBadyt4THArUkFpUVBSQTI3RDhHYjJSVmdUU0NpdENZcXczR3BlS0cKYmU5em1PczFsWHlzeTZpRXZuV3k3U3k0b3NaaU52a0ltM0hvT1JlMUpiZE1kbzc0YWJ4S1h5K3N3S29sYnMyVQpOeXFndFI1MlpadndGTk1JOVdPeFEzUjB4UXhJYTl6K1lnamUvVjBDZ2dFQUJpMFIzRWRzOTNvbEtGckNWOURyCmdMV2VrOTNVcWxHbURYZ2w2clZBd3FLTDY5Ynd3L1Bid1EvLzI0eWxOWUJiL2wxaDhPbWliNWRacHNqQnFzSUkKb0RyK2FPRXMzMmFiUDBuMVhjUzUxdmd5T2ZPN1kxZ2ZOOU8yaWtVZnFHNkZkdlBJWGhlb3dUc1BDcUZUUEUrcApHbHR3Y293R2hVRk5POVlQQVhwQitYUEF6QlhqaXJrVE9vbjRMdkROSStsZHJmNnRTdVpNVGFPYS9FNEhtaisyClYyTGdBamNRSVd5czJXUzh4TXRYblA2M215RU5JV3VuM25ITFJHalYxVjArQlIrZnVJNDhMUUw1YXpmdW5DNE4KRkpiQzE1SFhNYTRKUjNnemZqbm1yYUpVOE1TK3BJYVQrY2xiSTRlLzBXcFBIKzhSRUZST0FQZzdzV1ZveXZlbAo0UUtDQVFFQTczcWhkZDVZMmV5TlpET0VUZTZOTFlidkNTbElRN2pXdWhpZm14OE8wVWVaNk04UDNIOXVMZFRYCnNjSzVvWjZ5RFU4azRCMW5ueDhwd1JhNTZoNzc0NnVnUTJkUTRBUjNraHlBN1M3Y21VRVUycXJEazZ0ZmVBTVIKTXNQajVvbHFYVVBJbUxoMkhXWTVDWHplVDZaWUY4NzVpc2hEMmwzRExBVkhQY2NwV0ZDV3dtNGN4RVNjc21ZQwpiRGJINkFuNC9Ia3VGc01kN1UxTnF5UGlYS0dCOHFET2FDcXlmR0VtanBoYW5UT3ZWNWN2NVo4UzVudXArdTd5CjByMXZlTU5lRlEzTUVYc0VXbElENEtxaGNaV1JkQmIzTW5sMUVaVXdabGZaNmFNUGVZZjUzZUZKWGI1WkJlM2EKT0YxVXh3VS9RVVZLa1dkbnU3SFNCOXg2eUdwSDhnPT0KLS0tLS1FTkQgUFJJVkFURSBLRVktLS0tLQo=`

	// KmmScannerDockerfile represents dockerfile used to run clamav on KMM images.
	KmmScannerDockerfile = `ARG OPERATOR_IMAGE
ARG MUST_GATHER
ARG SIGN
ARG WORKER
ARG RBAC_IMAGE

FROM ${OPERATOR_IMAGE} as operator
FROM ${MUST_GATHER} as must-gather
FROM ${SIGN} as sign
FROM ${WORKER} as worker
FROM ${RBAC_IMAGE} as rbac

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.8-1072
RUN rpm -ivh https://dl.fedoraproject.org/pub/epel/epel-release-latest-8.noarch.rpm && \
    microdnf -y --setopt=tsflags=nodocs install \
    clamav \
    clamd \
    clamav-update && \
    microdnf clean all

WORKDIR /operator
COPY --from=operator . .

WORKDIR /must-gather
COPY --from=must-gather . .

WORKDIR /sign
COPY --from=sign . .

WORKDIR /worker
COPY --from=worker . .

WORKDIR /rbac
COPY --from=rbac . .

WORKDIR /
RUN freshclam
RUN clamscan -v -a --bell --log=/operator.log -r -z /operator
RUN clamscan -v -a --bell --log=/must-gather.log -r -z /must-gather
RUN clamscan -v -a --bell --log=/sign.log -r -z /sign
RUN clamscan -v -a --bell --log=/worker.log -r -z /worker
RUN clamscan -v -a --bell --log=/rbac.log -r -z /rbac
RUN chmod o+r *.log

`
)

const (
	// LabelSuite represents kmm label that can be used for test cases selection.
	LabelSuite = "module"
	// LabelSanity represents kmm label for short-running tests used for test case selection.
	LabelSanity = "kmm-sanity"
	// LabelLongRun represent kmm label for long-running tests used for test case selection.
	LabelLongRun = "kmm-longrun"
	// KmmOperatorNamespace represents the namespace where KMM is installed.
	KmmOperatorNamespace = "openshift-kmm"
	// KmmHubOperatorNamespace represents namespace of the operator.
	KmmHubOperatorNamespace = "openshift-kmm-hub"
	// DeploymentName represents the name of the KMM operator deployment.
	DeploymentName = "kmm-operator-controller"
	// WebhookDeploymentName represents the name of the Webhook server deployment.
	WebhookDeploymentName = "kmm-operator-webhook-server"
	// HubDeploymentName represents the name of the KMM HUB deployment.
	HubDeploymentName = "kmm-operator-hub-controller"
	// HubWebhookDeploymentName represents the name of the HUB Webhook server deployment.
	HubWebhookDeploymentName = "kmm-operator-hub-webhook-server"
	// BuildArgName represents kmod key passed to kmm-ci example.
	BuildArgName = "MY_MODULE"
	// RelImgMustGather represents identifier for must-gather image in operator environment variables.
	RelImgMustGather = "MUST_GATHER"
	// RelImgSign represents identifier for sign image in operator environment variables.
	RelImgSign = "SIGN"
	// RelImgWorker represents identifier for worker image in operator environment variables.
	RelImgWorker = "WORKER"
	// ModuleNodeLabelTemplate represents template of the label set on a node for a Module.
	ModuleNodeLabelTemplate = "kmm.node.kubernetes.io/%s.%s.ready"
	// ModuleVersionNodeLabelTemplate represents template of the label set on a node for a Module Version.
	ModuleVersionNodeLabelTemplate = "kmm.node.kubernetes.io/%s.%s.version.ready"
	// DevicePluginNodeLabelTemplate represents template label set by KMM on a node for a Device Plugin.
	DevicePluginNodeLabelTemplate = "kmm.node.kubernetes.io/%s.%s.device-plugin-ready"
	// UseDtkModuleTestNamespace represents test case namespace name.
	UseDtkModuleTestNamespace = "54283-use-dtk"
	// UseLocalMultiStageTestNamespace represents test case namespace name.
	UseLocalMultiStageTestNamespace = "53651-multi-stage"
	// WebhookModuleTestNamespace represents test case namespace name.
	WebhookModuleTestNamespace = "webhook"
	// SimpleKmodModuleTestNamespace represents test case namespace name.
	SimpleKmodModuleTestNamespace = "simple-kmod"
	// DevicePluginTestNamespace represents test case namespace name.
	DevicePluginTestNamespace = "53678-devplug"
	// RealtimeKernelNamespace represents test case namespace name.
	RealtimeKernelNamespace = "53656-rtkernel"
	// FirmwareTestNamespace represents test case namespace name.
	FirmwareTestNamespace = "simple-kmod-firmware"
	// ModuleBuildAndSignNamespace represents test case namespace name.
	ModuleBuildAndSignNamespace = "56252"
	// InTreeReplacementNamespace represents test case namespace name.
	InTreeReplacementNamespace = "62745"
	// MultipleModuleTestNamespace represents test case namespace name.
	MultipleModuleTestNamespace = "multiple-modules"
	// VersionModuleTestNamespace represents test case namespace name.
	VersionModuleTestNamespace = "modver"
	// TolerationModuleTestNamespace represents test case namespace name.
	TolerationModuleTestNamespace = "79205-tol"
	// RebuildTriggerBasicNamespace represents test case namespace name.
	RebuildTriggerBasicNamespace = "rt-basic"
	// RebuildTriggerNoopNamespace represents test case namespace name.
	RebuildTriggerNoopNamespace = "rt-noop"
	// AutomountSATokenTestNamespace represents test case namespace name for automount SA token tests.
	AutomountSATokenTestNamespace = "automount-satoken"
	// FilesToSignGlobTestNamespace represents test case namespace name for filesToSign glob tests.
	FilesToSignGlobTestNamespace = "filestosign-glob"
	// DefaultNodesNamespace represents namespace of the nodes events.
	DefaultNodesNamespace = "default"
	// SimpleKmodImage represents the pre-built simple-kmod kernel module image.
	SimpleKmodImage = "quay.io/ocp-edge-qe/simple-kmod"
	// SimpleKmodModuleName represents the simple-kmod kernel module name.
	SimpleKmodModuleName = "simple-kmod"
	// DefaultWorkerMCPName represents the default worker MachineConfigPool name.
	DefaultWorkerMCPName = "worker"
	// PreflightDTKImageX86 represents x86_64 DTK image for KMM 2.4 preflightvalidationocp.
	// Compatible with OpenShift Container Platform 4.18.
	PreflightDTKImageX86 = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:" +
		"7bfeb4d93b12a70c561de0d104d21c1898dac65d96808ff2d2f772134b4261e8"
	// PreflightDTKImageARM64 represents ARM64 DTK image for KMM 2.4 preflightvalidationocp.
	// Compatible with OpenShift Container Platform 4.18.
	PreflightDTKImageARM64 = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:" +
		"ada767898092f36e8d965292843f9a772b2df449aeda06580430162696bd5ddf"
	// PreflightDTKImageS390X represents S390X DTK image for KMM 2.4 preflightvalidationocp.
	// Compatible with OpenShift Container Platform 4.18.
	PreflightDTKImageS390X = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:" +
		"a61237a389ac9e52841468a5540f810b50f33e9106d9817eb1e1e04cf6064ce8"
	// PreflightDTKImagePPC64LE represents PPC64LE DTK image for KMM 2.4 preflightvalidationocp.
	PreflightDTKImagePPC64LE = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:" +
		"7785b2a16b2a2c7443cfc59164c74acb341237b09a9f87c5f0747c9140d21b92"
	// KernelForDTKPpc64le represents kernel string for ppc64le dtk image.
	KernelForDTKPpc64le = "5.14.0-570.41.1.el9_6.ppc64le"
	// KernelForDTKPpc64leRealtime represents kernel string for ppc64le realtime dtk image.
	KernelForDTKPpc64leRealtime = "not supported"
	// KernelForDTKArm64 represents kernel string for arm64 dtk image.
	KernelForDTKArm64 = "5.14.0-284.52.1.el9_2.aarch64"
	// KernelForDTKArm64Realtime represents kernel string for arm64 realtime dtk image.
	KernelForDTKArm64Realtime = "not supported"
	// KernelForDTKX86 represents kernel string for x86 dtk image.
	KernelForDTKX86 = "5.14.0-427.81.1.el9_4.x86_64"
	// KernelForDTKX86Realtime represents kernel string for x86 realtime dtk image.
	KernelForDTKX86Realtime = "5.14.0-427.81.1.el9_4.x86_64+rt"
	// KernelForDTKS390x represents kernel string for s390x dtk image.
	KernelForDTKS390x = "5.14.0-427.65.1.el9_4.s390x"
	// KernelForDTKS390xRealtime represents kernel string for s390x realtime dtk image.
	KernelForDTKS390xRealtime = "not supported"
	// PreflightName represents preflightvalidation ocp object name.
	PreflightName = "preflight"
	// ScannerTestNamespace represents test case namespace name.
	ScannerTestNamespace = "kmm-scanner"
	// ReasonBuildCompleted represents event reason for a build completed.
	ReasonBuildCompleted = "BuildCompleted"
	// ReasonBuildCreated represents event reason for a build created.
	ReasonBuildCreated = "BuildimageCreated"
	// ReasonBuildStarted represents event reason for a build started.
	ReasonBuildStarted = "BuildStarted"
	// ReasonBuildSucceeded represents event reason for a build succeeded.
	ReasonBuildSucceeded = "BuildimageSucceeded"
	// ReasonSignCreated represents event reason for a sign created.
	ReasonSignCreated = "SignimageCreated"
	// ReasonSignSucceeded represents event reason for a sign succeeded.
	ReasonSignSucceeded = "SignimageSucceeded"
	// ReasonModuleLoaded represents event reason for a module loaded.
	ReasonModuleLoaded = "ModuleLoaded"
	// ReasonModuleUnloaded represents event reason for a module unloaded.
	ReasonModuleUnloaded = "ModuleUnloaded"

	// InTreeRemoveModuleX86 represents an in-tree kernel module for removal testing on x86.
	InTreeRemoveModuleX86 = "ib_ipoib"
	// InTreeRemoveModuleArm64 represents an in-tree kernel module for removal testing on arm64.
	InTreeRemoveModuleArm64 = "ib_ipoib"
	// InTreeRemoveModuleS390x represents an in-tree kernel module for removal testing on s390x.
	InTreeRemoveModuleS390x = "ib_ipoib"
	// InTreeRemoveModulePpc64le represents an in-tree kernel module for removal testing on ppc64le.
	InTreeRemoveModulePpc64le = "ib_ipoib"
)
