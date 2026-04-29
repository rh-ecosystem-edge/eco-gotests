module github.com/rh-ecosystem-edge/eco-gotests

go 1.26

toolchain go1.26.0

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/Juniper/go-netconf v0.3.1
	github.com/Masterminds/semver/v3 v3.4.0
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/NVIDIA/gpu-operator v1.11.1
	github.com/cavaliergopher/cpio v1.0.1
	github.com/cavaliergopher/grab/v3 v3.0.1
	github.com/containers/image/v5 v5.36.2
	github.com/coreos/ignition/v2 v2.26.0
	github.com/go-git/go-git/v5 v5.17.0
	github.com/go-logr/logr v1.4.3
	github.com/go-openapi/runtime v0.29.2
	github.com/go-openapi/strfmt v0.25.0
	github.com/google/uuid v1.6.0
	github.com/grafana/loki/operator/apis/loki v0.0.0-20241021105923-5e970e50b166
	github.com/hashicorp/go-version v1.8.0
	github.com/k8snetworkplumbingwg/multi-networkpolicy v1.0.1
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.7
	github.com/k8snetworkplumbingwg/sriov-network-operator v1.6.0
	github.com/kedacore/keda-olm-operator v0.0.0-20260226211224-331db27583d3
	github.com/kedacore/keda/v2 v2.18.1-0.20260128140547-07f4f705abe2
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/klauspost/compress v1.18.4
	github.com/metal3-io/baremetal-operator/apis v0.12.2
	github.com/nmstate/kubernetes-nmstate/api v0.0.0-20260303115241-f7eb8708765a
	github.com/onsi/ginkgo/v2 v2.28.1
	github.com/onsi/gomega v1.39.1
	github.com/openshift-kni/cluster-group-upgrades-operator v0.0.0-20260303165917-19490b7b0e65
	github.com/openshift-kni/k8sreporter v1.0.7
	github.com/openshift-kni/lifecycle-agent v0.0.0-20260303165314-4c7e2e331845 // release-4.21
	github.com/openshift-kni/numaresources-operator v0.4.18-0.2024100201.0.20260303161347-c598bf331b95 // release-4.21
	github.com/openshift-kni/oran-o2ims v0.0.0-20260303201028-2a10788e098f // release-4.21
	github.com/openshift/api v0.0.0-20260302175216-a591881943ae // release-4.21
	github.com/openshift/client-go v0.0.0-20251205093018-96a6cbc1420c // release-4.21
	github.com/openshift/cluster-nfd-operator v0.0.0-20251208202434-7ee083174821 // release-4.21
	github.com/openshift/cluster-node-tuning-operator v0.0.0-20260114201201-0e275839ec18 // release-4.21
	github.com/openshift/installer v0.0.0-00010101000000-000000000000
	github.com/openshift/local-storage-operator v0.0.0-20251219063308-d463337f1468 // release-4.21
	github.com/operator-framework/api v0.38.0 // aligned with k8s v0.34
	github.com/povsister/scp v0.0.0-20250701154629-777cf82de5df
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.89.0 // aligned with k8s v0.33
	github.com/prometheus/alertmanager v0.31.1
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/common v1.20.99
	github.com/redhat-cne/sdk-go v1.23.6
	github.com/stmcginnis/gofish v0.20.0
	github.com/stretchr/testify v1.11.1
	github.com/vmware-tanzu/velero v1.17.2
	github.com/walle/targz v0.0.0-20140417120357-57fe4206da5a
	github.com/wk8/go-ordered-map/v2 v2.1.8
	golang.org/x/crypto v0.48.0
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa
	golang.org/x/oauth2 v0.35.0
	gopkg.in/k8snetworkplumbingwg/multus-cni.v4 v4.2.4
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.34.5
	k8s.io/apimachinery v0.35.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog/v2 v2.130.1
	k8s.io/kubelet v0.34.5
	k8s.io/utils v0.0.0-20260210185600-b8788abfbbc2
	open-cluster-management.io/config-policy-controller v0.17.0
	open-cluster-management.io/governance-policy-propagator v0.17.0
	open-cluster-management.io/multicloud-operators-subscription v0.16.0
	sigs.k8s.io/controller-runtime v0.22.5
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.20.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/PaesslerAG/gval v1.0.0 // indirect
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/ProtonMail/go-crypto v1.4.0 // indirect
	github.com/ajeddeloh/go-json v0.0.0-20200220154158-5ae607161559 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/asaskevich/govalidator/v11 v11.0.2-0.20250122183457-e11347878e23 // indirect
	github.com/aws/aws-sdk-go-v2 v1.41.1 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/clarketm/json v1.17.1 // indirect
	github.com/cloudevents/sdk-go/v2 v2.16.2 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/containernetworking/cni v1.3.0 // indirect
	github.com/containers/storage v1.59.1 // indirect
	github.com/coreos/fcct v0.5.0 // indirect
	github.com/coreos/go-json v0.0.0-20230131223807-18775e0fb4fb // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/coreos/ign-converter v0.0.0-20230417193809-cee89ea7d8ff // indirect
	github.com/coreos/ignition v0.35.0 // indirect
	github.com/coreos/vcontext v0.0.0-20231102161604-685dc7299dc5 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/expr-lang/expr v1.17.7 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/ghodss/yaml v1.0.1-0.20220118164431-d8423dcdf344 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.8.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-openapi/analysis v0.24.2 // indirect
	github.com/go-openapi/errors v0.22.6 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/jsonreference v0.21.5 // indirect
	github.com/go-openapi/loads v0.23.2 // indirect
	github.com/go-openapi/spec v0.22.4 // indirect
	github.com/go-openapi/swag v0.25.5 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.5 // indirect
	github.com/go-openapi/swag/conv v0.25.5 // indirect
	github.com/go-openapi/swag/fileutils v0.25.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.5 // indirect
	github.com/go-openapi/swag/loading v0.25.5 // indirect
	github.com/go-openapi/swag/mangling v0.25.5 // indirect
	github.com/go-openapi/swag/netutils v0.25.5 // indirect
	github.com/go-openapi/swag/stringutils v0.25.5 // indirect
	github.com/go-openapi/swag/typeutils v0.25.5 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.5 // indirect
	github.com/go-openapi/validate v0.25.1 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20260302011040-a15ffb7f9dcc // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grafana/regexp v0.0.0-20240518133315-a468a5bfb3bc // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-7 // indirect
	github.com/hashicorp/vault/api v1.22.0 // indirect
	github.com/hashicorp/vault/api/auth/approle v0.11.0 // indirect
	github.com/hashicorp/vault/api/auth/kubernetes v0.10.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/imdario/mergo v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kdomanski/iso9660 v0.2.1 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kube-object-storage/lib-bucket-provisioner v0.0.0-20221122204822-d1a8c34382f1 // indirect
	github.com/lib/pq v1.11.2 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mailru/easyjson v0.9.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/moby/sys/capability v0.4.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nutanix-cloud-native/prism-go-client v0.5.0 // indirect
	github.com/oapi-codegen/runtime v1.1.2 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/opencontainers/runtime-spec v1.2.1 // indirect
	github.com/openshift/cluster-logging-operator/api/observability v0.0.0-20250422180113-5bae4ccfc5ef // indirect
	github.com/openshift/custom-resource-status v1.1.3-0.20220503160415-f2fdb4999d87 // indirect
	github.com/openshift/elasticsearch-operator v0.0.0-20241202223819-cc1a232913d6 // indirect
	github.com/openshift/library-go v0.0.0-20251120164824-14a789e09884 // indirect
	github.com/openshift/machine-config-operator v0.0.1-0.20250320230514-53e78f3692ee // indirect
	github.com/otiai10/copy v1.14.0 // indirect
	github.com/ovn-kubernetes/ovn-kubernetes/go-controller v0.0.0-20260303063950-da86b2aa2ff0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pjbgf/sha1cd v0.5.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/r3labs/diff/v3 v3.0.2 // indirect
	github.com/red-hat-storage/odf-operator v0.0.0-20260226164309-08c71191d483 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/skeema/knownhosts v1.3.2 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stolostron/kubernetes-dependency-watches v0.10.2 // indirect
	github.com/thoas/go-funk v0.9.3 // indirect
	github.com/vincent-petithory/dataurl v1.0.0 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	go.mongodb.org/mongo-driver v1.17.9 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	go4.org v0.0.0-20230225012048-214862532bf5 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/term v0.40.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gorm.io/gorm v1.31.1 // indirect
	k8s.io/apiserver v0.34.5 // indirect
	k8s.io/cli-runtime v0.34.5 // indirect
	k8s.io/component-base v0.34.5 // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/kube-aggregator v0.34.2 // indirect
	k8s.io/kube-openapi v0.34.2 // indirect
	k8s.io/kubectl v0.34.5 // indirect
	knative.dev/pkg v0.0.0-20260104220430-51874fe86fcb // indirect
	maistra.io/api v0.0.0-20240319144440-ffa91c765143 // indirect
	open-cluster-management.io/api v1.2.0 // indirect
	sigs.k8s.io/cluster-api v1.11.2 // indirect
	sigs.k8s.io/cluster-api-provider-azure v1.21.1-0.20250929163617-2c4eaa611a39 // indirect
	sigs.k8s.io/container-object-storage-interface-api v0.1.0 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/kube-storage-version-migrator v0.0.6-0.20230721195810-5c8923c5ff96 // indirect
	sigs.k8s.io/kustomize/api v0.21.0 // indirect
	sigs.k8s.io/kustomize/kyaml v0.21.0 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

require (
	github.com/rh-ecosystem-edge/eco-goinfra v0.0.0-20260428185709-3aaf0a41ccdc
	k8s.io/apiextensions-apiserver v0.34.5
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.16
	github.com/k8snetworkplumbingwg/sriov-network-operator => github.com/openshift/sriov-network-operator v0.0.0-20251223102349-9cac901c5313 // release-4.21
	github.com/kubernetes-incubator/external-storage => github.com/libopenstorage/external-storage v5.2.1-0.20190425001840-d5e6a33a1729+incompatible
	github.com/metal3-io/baremetal-operator/pkg/hardwareutils => github.com/metal3-io/baremetal-operator/pkg/hardwareutils v0.9.0
	github.com/openshift/api => github.com/openshift/api v0.0.0-20260302175216-a591881943ae // release-4.21
	github.com/openshift/assisted-service/api => github.com/openshift/assisted-service/api v0.0.0-20260204223219-1574e1fa7ee0 // release-4.21
	github.com/openshift/assisted-service/models => github.com/openshift/assisted-service/models v0.0.0-20260204223219-1574e1fa7ee0 // release-4.21
	github.com/openshift/installer => github.com/openshift/installer v0.0.0-20260211082425-e973ac44d688 // release-4.21
	github.com/portworx/sched-ops => github.com/portworx/sched-ops v1.20.4-rc1
	k8s.io/client-go => k8s.io/client-go v0.34.5
	// The cluster-node-tuning-operator release-4.21 uses version k8s.io/kube-openapi v0.34.2, which does not exist.
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20260127142750-a19766b6e2d4
	// Required by openshift/installer release-4.21. Temporary.
	sigs.k8s.io/cluster-api-provider-azure => github.com/mboersma/cluster-api-provider-azure v0.3.1-0.20251030205607-3161b9cc8d3e
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.19.7
)
