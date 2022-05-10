BASE_BRANCH ?= devel
# Denotes the default operator image version, exposed as a variable for the automated release
DEFAULT_IMAGE_VERSION ?= $(BASE_BRANCH)
export BASE_BRANCH
export DEFAULT_IMAGE_VERSION

# Define LOCAL_BUILD to build directly on the host and not inside a Dapper container
ifdef LOCAL_BUILD
DAPPER_HOST_ARCH ?= $(shell go env GOHOSTARCH)
SHIPYARD_DIR ?= ../shipyard
SCRIPTS_DIR ?= $(SHIPYARD_DIR)/scripts/shared

export DAPPER_HOST_ARCH
export SHIPYARD_DIR
export SCRIPTS_DIR
endif

ifneq (,$(DAPPER_HOST_ARCH))

CONTROLLER_GEN := $(CURDIR)/bin/controller-gen

# Running in Dapper

IMAGES = subctl
undefine SKIP
undefine FOCUS
undefine E2E_TESTDIR

ifneq (,$(filter ovn,$(_using)))
SETTINGS = $(DAPPER_SOURCE)/.shipyard.e2e.ovn.yml
else
SETTINGS = $(DAPPER_SOURCE)/.shipyard.e2e.yml
endif

include $(SHIPYARD_DIR)/Makefile.inc

CROSS_TARGETS := linux-amd64 linux-arm64 linux-arm linux-s390x linux-ppc64le windows-amd64.exe darwin-amd64
BINARIES := bin/subctl
CROSS_BINARIES := $(foreach cross,$(CROSS_TARGETS),$(patsubst %,bin/subctl-$(VERSION)-%,$(cross)))
CROSS_TARBALLS := $(foreach cross,$(CROSS_TARGETS),$(patsubst %,dist/subctl-$(VERSION)-%.tar.xz,$(cross)))

override E2E_ARGS += --settings $(SETTINGS) cluster1 cluster2
export DEPLOY_ARGS
override UNIT_TEST_ARGS += test internal/env
override VALIDATE_ARGS += --skip-dirs pkg/client

# Process extra flags from the `using=a,b,c` optional flag

ifneq (,$(filter lighthouse,$(_using)))
override DEPLOY_ARGS += --deploytool_broker_args '--components service-discovery,connectivity'
endif

GO ?= go
GOARCH = $(shell $(GO) env GOARCH)
GOEXE = $(shell $(GO) env GOEXE)
GOOS = $(shell $(GO) env GOOS)

# Produce v1 CRDs, requiring Kubernetes 1.16 or later
CRD_OPTIONS ?= "crd:crdVersions=v1,trivialVersions=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell $(GO) env GOBIN))
GOBIN=$(shell $(GO) env GOPATH)/bin
else
GOBIN=$(shell $(GO) env GOBIN)
endif

# Ensure we prefer binaries we build
export PATH := $(CURDIR)/bin:$(PATH)

# Targets to make

images: build

# Build subctl before deploying to ensure we use that
# (with the PATH set above)
deploy: bin/subctl

e2e: deploy
	scripts/kind-e2e/e2e.sh $(E2E_ARGS)

clean:
	rm -f $(BINARIES) $(CROSS_BINARIES) $(CROSS_TARBALLS)

build: $(BINARIES)

build-cross: $(CROSS_TARBALLS)

licensecheck: BUILD_ARGS=--noupx
licensecheck: build | bin/lichen
	bin/lichen -c .lichen.yaml $(BINARIES)

bin/lichen: $(VENDOR_MODULES)
	mkdir -p $(@D)
	$(GO) build -o $@ github.com/uw-labs/lichen

package/Dockerfile.subctl: bin/subctl

# Generate deep-copy code
CONTROLLER_DEEPCOPY := api/submariner/v1alpha1/zz_generated.deepcopy.go
$(CONTROLLER_DEEPCOPY): $(VENDOR_MODULES) | $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) object:headerFile="$(CURDIR)/hack/boilerplate.go.txt,year=$(shell date +"%Y")" paths="./..."

# Generate embedded YAMLs
EMBEDDED_YAMLS := pkg/embeddedyamls/yamls.go
$(EMBEDDED_YAMLS): pkg/embeddedyamls/generators/yamls2go.go deploy/crds/submariner.io_servicediscoveries.yaml deploy/crds/submariner.io_brokers.yaml deploy/crds/submariner.io_submariners.yaml deploy/submariner/crds/submariner.io_clusters.yaml deploy/submariner/crds/submariner.io_endpoints.yaml deploy/submariner/crds/submariner.io_gateways.yaml $(shell find deploy/ -name "*.yaml") $(shell find config/rbac/ -name "*.yaml") $(VENDOR_MODULES) $(CONTROLLER_DEEPCOPY)
	$(GO) generate pkg/embeddedyamls/generate.go

bin/subctl: bin/subctl-$(VERSION)-$(GOOS)-$(GOARCH)$(GOEXE)
	ln -sf $(<F) $@

cmd/bin/subctl: cmd/bin/subctl-$(VERSION)-$(GOOS)-$(GOARCH)$(GOEXE)
	ln -sf $(<F) $@

dist/subctl-%.tar.xz: bin/subctl-%
	mkdir -p dist
	tar -cJf $@ --transform "s/^bin/subctl-$(VERSION)/" $<

# Versions may include hyphens so it's easier to use $(VERSION) than to extract them from the target
bin/subctl-%: $(EMBEDDED_YAMLS) $(shell find pkg/subctl/ -name "*.go") $(VENDOR_MODULES)
	mkdir -p $(@D)
	target=$@; \
	target=$${target%.exe}; \
	components=($$(echo $${target//-/ })); \
	GOOS=$${components[-2]}; \
	GOARCH=$${components[-1]}; \
	export GOARCH GOOS; \
	$(SCRIPTS_DIR)/compile.sh \
		--ldflags "-X 'github.com/submariner-io/submariner-operator/pkg/version.Version=$(VERSION)' \
			   -X 'github.com/submariner-io/submariner-operator/api/submariner/v1alpha1.DefaultSubmarinerOperatorVersion=$${DEFAULT_IMAGE_VERSION#v}'" \
		--noupx $@ ./pkg/subctl $(BUILD_ARGS)

cmd/bin/subctl-%: $(shell find cmd/ -name "*.go") $(VENDOR_MODULES)
	mkdir -p cmd/bin
	target=$@; \
	target=$${target%.exe}; \
	components=($$(echo $${target//-/ })); \
	GOOS=$${components[-2]}; \
	GOARCH=$${components[-1]}; \
	export GOARCH GOOS; \
	$(SCRIPTS_DIR)/compile.sh \
		--ldflags "-X 'github.com/submariner-io/submariner-operator/pkg/version.Version=$(VERSION)' \
		       -X 'github.com/submariner-io/submariner-operator/api/submariner/v1alpha1.DefaultSubmarinerOperatorVersion=$${DEFAULT_IMAGE_VERSION#v}'" \
        --noupx $@ ./cmd $(BUILD_ARGS)

ci: $(EMBEDDED_YAMLS) golangci-lint markdownlint unit build images

# Operator CRDs
$(CONTROLLER_GEN): $(VENDOR_MODULES)
	mkdir -p $(@D)
	$(GO) build -o $@ sigs.k8s.io/controller-tools/cmd/controller-gen

deploy/crds/submariner.io_servicediscoveries.yaml: ./api/submariner/v1alpha1/servicediscovery_types.go $(VENDOR_MODULES) | $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=deploy/crds
	test -f $@

deploy/crds/submariner.io_brokers.yaml deploy/crds/submariner.io_submariners.yaml: ./api/submariner/v1alpha1/submariner_types.go $(VENDOR_MODULES) | $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=deploy/crds
	test -f $@

# Submariner CRDs
deploy/submariner/crds/submariner.io_clusters.yaml deploy/submariner/crds/submariner.io_endpoints.yaml deploy/submariner/crds/submariner.io_gateways.yaml: $(VENDOR_MODULES) | $(CONTROLLER_GEN)
	cd vendor/github.com/submariner-io/submariner && $(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=../../../../deploy/submariner/crds
	test -f $@

# Generate the clientset for the Submariner APIs
# It needs to be run when the Submariner APIs change
generate-clientset: $(VENDOR_MODULES)
	git clone https://github.com/kubernetes/code-generator -b kubernetes-1.19.10 $${GOPATH}/src/k8s.io/code-generator
	cd $${GOPATH}/src/k8s.io/code-generator && $(GO) mod vendor
	GO111MODULE=on $${GOPATH}/src/k8s.io/code-generator/generate-groups.sh \
		client,deepcopy \
		github.com/submariner-io/submariner-operator/pkg/client \
		github.com/submariner-io/submariner-operator/api \
		submariner:v1alpha1

golangci-lint: $(EMBEDDED_YAMLS)

unit: $(EMBEDDED_YAMLS)

# Test as many of the config/context-dependent subctl commands as possible
test-subctl: bin/subctl deploy
# benchmark
	bin/subctl benchmark latency --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1:$(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster2 \
		--kubecontexts cluster1,cluster2
	bin/subctl benchmark latency --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 \
		--kubecontexts cluster1 --intra-cluster
	bin/subctl benchmark throughput --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1:$(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster2 \
		--kubecontexts cluster1,cluster2
	bin/subctl benchmark throughput --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 \
		--kubecontexts cluster1 --intra-cluster
# cloud
	bin/subctl cloud prepare generic --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 --kubecontext cluster1
# deploy-broker is tested by the deploy target
# diagnose
	bin/subctl diagnose all --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1
	bin/subctl diagnose firewall inter-cluster $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster2
# export TBD
# gather
	bin/subctl gather $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1
# join is tested by the deploy target
# show
	bin/subctl show all --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1
# verify is tested by the e2e target (run elsewhere)
	bin/subctl uninstall -y --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1

.PHONY: build ci clean generate-clientset

else

# Not running in Dapper

Makefile.dapper:
	@echo Downloading $@
	@curl -sfLO https://raw.githubusercontent.com/submariner-io/shipyard/$(BASE_BRANCH)/$@

include Makefile.dapper

.PHONY: deploy licensecheck

endif

# Disable rebuilding Makefile
Makefile Makefile.inc: ;
