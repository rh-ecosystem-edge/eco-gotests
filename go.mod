module github.com/rh-ecosystem-edge/eco-gotests

go 1.24.1

require (
	github.com/BurntSushi/toml v1.4.0
	github.com/Juniper/go-netconf v0.3.0
	github.com/NVIDIA/gpu-operator v1.11.1
	github.com/cavaliergopher/cpio v1.0.1
	github.com/cavaliergopher/grab/v3 v3.0.1
	github.com/containers/image/v5 v5.31.0
	github.com/golang/glog v1.2.4
	github.com/grafana/loki/operator/apis/loki v0.0.0-20241021105923-5e970e50b166
	github.com/hashicorp/go-version v1.7.0
	github.com/k8snetworkplumbingwg/multi-networkpolicy v1.0.1
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.5
	github.com/k8snetworkplumbingwg/sriov-network-operator v1.4.0
	github.com/kedacore/keda-olm-operator v0.0.0-20241108231742-8cb33dda32d3
	github.com/kedacore/keda/v2 v2.16.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/klauspost/compress v1.17.11
	github.com/metal3-io/baremetal-operator/apis v0.9.0
	github.com/nmstate/kubernetes-nmstate/api v0.0.0-20250114063637-129b149f6ce9
	github.com/onsi/ginkgo/v2 v2.22.2
	github.com/onsi/gomega v1.36.2
	github.com/openshift-kni/k8sreporter v1.0.6
	github.com/openshift-kni/lifecycle-agent v0.0.0-20250120220331-9547280df193 // release-4.18
	github.com/openshift-kni/numaresources-operator v0.4.18-0.2024100201.0.20250114093602-01c00730991d // release-4.18
	github.com/openshift-kni/oran-hwmgr-plugin/api/hwmgr-plugin v0.0.0-20250128160241-57fbcf565b32
	github.com/openshift-kni/oran-o2ims/api/hardwaremanagement v0.0.0-20250130153422-72163e75c543
	github.com/openshift-kni/oran-o2ims/api/provisioning v0.0.0-20250123151805-c935b06062f9
	github.com/openshift/api v3.9.1-0.20191111211345-a27ff30ebf09+incompatible
	github.com/openshift/client-go v0.0.0-20241107164952-923091dd2b1a
	github.com/openshift/cluster-logging-operator v0.0.0-20241003210634-afb65cea19d1 // release-5.9
	github.com/openshift/cluster-nfd-operator v0.0.0-20250116132220-e601a6278a42 // release-4.18
	github.com/openshift/cluster-node-tuning-operator v0.0.0-20250116110730-90fc6ab88c08 // release-4.18
	github.com/openshift/installer v0.0.0-00010101000000-000000000000
	github.com/openshift/local-storage-operator v0.0.0-20241213132210-fa575fb5f229 // release-4.18
	github.com/operator-framework/api v0.28.0
	github.com/povsister/scp v0.0.0-20240802064259-28781e87b246
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.78.2
	github.com/stretchr/testify v1.10.0
	github.com/vmware-tanzu/velero v1.13.2
	github.com/walle/targz v0.0.0-20140417120357-57fe4206da5a
	github.com/wk8/go-ordered-map/v2 v2.1.8
	golang.org/x/exp v0.0.0-20250106191152-7588d65b2ba8
	gopkg.in/k8snetworkplumbingwg/multus-cni.v4 v4.1.4
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.31.5
	k8s.io/apimachinery v0.31.5
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/utils v0.0.0-20241210054802-24370beab758
	open-cluster-management.io/config-policy-controller v0.15.0
	open-cluster-management.io/governance-policy-propagator v0.15.0
	open-cluster-management.io/multicloud-operators-subscription v0.15.0
	sigs.k8s.io/controller-runtime v0.19.5
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.1 // indirect
	github.com/Masterminds/sprig/v3 v3.2.3 // indirect
	github.com/PaesslerAG/gval v1.0.0 // indirect
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/ajeddeloh/go-json v0.0.0-20200220154158-5ae607161559 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go v1.55.5 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/clarketm/json v1.17.1 // indirect
	github.com/containernetworking/cni v1.2.3 // indirect
	github.com/containers/storage v1.54.0 // indirect
	github.com/coreos/fcct v0.5.0 // indirect
	github.com/coreos/go-json v0.0.0-20230131223807-18775e0fb4fb // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/coreos/ign-converter v0.0.0-20230417193809-cee89ea7d8ff // indirect
	github.com/coreos/ignition v0.35.0 // indirect
	github.com/coreos/ignition/v2 v2.20.0
	github.com/coreos/vcontext v0.0.0-20231102161604-685dc7299dc5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/evanphx/json-patch v5.9.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/expr-lang/expr v1.16.9 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/ghodss/yaml v1.0.1-0.20220118164431-d8423dcdf344 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-jose/go-jose/v4 v4.0.4 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-openapi/analysis v0.23.0 // indirect
	github.com/go-openapi/errors v0.22.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/loads v0.22.0 // indirect
	github.com/go-openapi/spec v0.21.0 // indirect
	github.com/go-openapi/strfmt v0.23.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-openapi/validate v0.24.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-collections/collections v0.0.0-20130729185459-604e922904d3 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/gnostic-models v0.6.9-0.20230804172637-c7be7c783f49 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.8 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.6 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hashicorp/vault/api v1.15.0 // indirect
	github.com/hashicorp/vault/api/auth/approle v0.8.0 // indirect
	github.com/hashicorp/vault/api/auth/kubernetes v0.8.0 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/imdario/mergo v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kdomanski/iso9660 v0.2.1 // indirect
	github.com/kube-object-storage/lib-bucket-provisioner v0.0.0-20221122204822-d1a8c34382f1 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/metal3-io/baremetal-operator/pkg/hardwareutils v0.5.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/spdystream v0.4.0 // indirect
	github.com/moby/sys/mountinfo v0.7.1 // indirect
	github.com/moby/sys/user v0.1.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nutanix-cloud-native/prism-go-client v0.3.4 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/openshift-kni/cluster-group-upgrades-operator v0.0.0-20241213003211-a57a58a5c4f2
	github.com/openshift/custom-resource-status v1.1.3-0.20220503160415-f2fdb4999d87 // indirect
	github.com/openshift/elasticsearch-operator v0.0.0-20241202183904-81cd6e70c15e // indirect
	github.com/openshift/library-go v0.0.0-20240903143724-7c5c5d305ac1 // indirect
	github.com/openshift/machine-config-operator v0.0.1-0.20231024085435-7e1fb719c1ba
	github.com/otiai10/copy v1.14.0 // indirect
	github.com/ovn-org/ovn-kubernetes/go-controller v0.0.0-20250122211335-e07ba60f758a // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.20.5 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.61.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/r3labs/diff/v3 v3.0.1 // indirect
	github.com/red-hat-storage/odf-operator v0.0.0-20241124050455-02681e6c10df // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/samber/lo v1.47.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.6-0.20210604193023-d5e0c0615ace // indirect
	github.com/stmcginnis/gofish v0.20.0
	github.com/stolostron/kubernetes-dependency-watches v0.10.0 // indirect
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	github.com/thoas/go-funk v0.9.3 // indirect
	github.com/vincent-petithory/dataurl v1.0.0 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	go.mongodb.org/mongo-driver v1.17.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	go4.org v0.0.0-20230225012048-214862532bf5 // indirect
	golang.org/x/crypto v0.32.0
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/oauth2 v0.25.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/term v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	golang.org/x/tools v0.29.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/protobuf v1.36.1 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gorm.io/gorm v1.25.12 // indirect
	k8s.io/apiextensions-apiserver v0.31.5 // indirect
	k8s.io/apiserver v0.31.5 // indirect
	k8s.io/cli-runtime v0.31.5 // indirect
	k8s.io/component-base v0.31.5 // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-aggregator v0.31.5 // indirect
	k8s.io/kube-openapi v0.0.0-20241009091222-67ed5848f094 // indirect
	k8s.io/kubectl v0.31.5 // indirect
	k8s.io/kubelet v0.31.5
	knative.dev/pkg v0.0.0-20241218051509-40afb7c5436e // indirect
	maistra.io/api v0.0.0-20240319144440-ffa91c765143 // indirect
	open-cluster-management.io/api v0.15.0 // indirect
	sigs.k8s.io/container-object-storage-interface-api v0.1.0 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/kube-storage-version-migrator v0.0.6-0.20230721195810-5c8923c5ff96 // indirect
	sigs.k8s.io/kustomize/api v0.18.0 // indirect
	sigs.k8s.io/kustomize/kyaml v0.18.1 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

require gopkg.in/yaml.v3 v3.0.1

require github.com/rh-ecosystem-edge/eco-goinfra v0.0.0-20250918072008-53bbc8540dca

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.16
	github.com/k8snetworkplumbingwg/sriov-network-operator => github.com/openshift/sriov-network-operator v0.0.0-20250102160645-f496851fbd0d // release-4.18
	github.com/kubernetes-incubator/external-storage => github.com/libopenstorage/external-storage v5.2.1-0.20190425001840-d5e6a33a1729+incompatible
	github.com/metal3-io/baremetal-operator/pkg/hardwareutils => github.com/metal3-io/baremetal-operator/pkg/hardwareutils v0.9.0
	github.com/openshift/api => github.com/openshift/api v0.0.0-20250123044544-55c9ecba501f // release-4.18
	github.com/openshift/assisted-service/api => github.com/openshift/assisted-service/api v0.0.0-20250106212530-e5a400590fed // release-4.18
	github.com/openshift/assisted-service/models => github.com/openshift/assisted-service/models v0.0.0-20250106212530-e5a400590fed // release-4.18
	github.com/openshift/installer => github.com/openshift/installer v0.91.0 // master
	github.com/portworx/sched-ops => github.com/portworx/sched-ops v1.20.4-rc1
	k8s.io/client-go => k8s.io/client-go v0.31.5
)
