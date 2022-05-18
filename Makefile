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

# Running in Dapper

IMAGES = subctl
PRELOAD_IMAGES := $(IMAGES) submariner-operator submariner-gateway submariner-globalnet submariner-route-agent lighthouse-agent lighthouse-coredns
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
BINARIES := cmd/bin/subctl
CROSS_BINARIES := $(foreach cross,$(CROSS_TARGETS),$(patsubst %,cmd/bin/subctl-$(VERSION)-%,$(cross)))
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
deploy: cmd/bin/subctl

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

package/Dockerfile.subctl: cmd/bin/subctl

cmd/bin/subctl: cmd/bin/subctl-$(VERSION)-$(GOOS)-$(GOARCH)$(GOEXE)
	ln -sf $(<F) $@

dist/subctl-%.tar.xz: cmd/bin/subctl-%
	mkdir -p dist
	tar -cJf $@ --transform "s/^cmd.bin/subctl-$(VERSION)/" $<

# Versions may include hyphens so it's easier to use $(VERSION) than to extract them from the target
cmd/bin/subctl-%: $(shell find cmd/ -name "*.go") $(VENDOR_MODULES)
	mkdir -p cmd/bin
	target=$@; \
	target=$${target%.exe}; \
	components=($$(echo $${target//-/ })); \
	GOOS=$${components[-2]}; \
	GOARCH=$${components[-1]}; \
	export GOARCH GOOS; \
	$(SCRIPTS_DIR)/compile.sh \
		--ldflags "-X 'github.com/submariner-io/subctl/pkg/version.Version=$(VERSION)' \
		       -X 'github.com/submariner-io/submariner-operator/api/submariner/v1alpha1.DefaultSubmarinerOperatorVersion=$${DEFAULT_IMAGE_VERSION#v}'" \
        --noupx $@ ./cmd $(BUILD_ARGS)

ci: golangci-lint markdownlint unit build images

# Test as many of the config/context-dependent subctl commands as possible
test-subctl: cmd/bin/subctl deploy
# benchmark
	cmd/bin/subctl benchmark latency --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1:$(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster2 \
		--kubecontexts cluster1,cluster2
	cmd/bin/subctl benchmark latency --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 \
		--kubecontexts cluster1 --intra-cluster
	cmd/bin/subctl benchmark throughput --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1:$(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster2 \
		--kubecontexts cluster1,cluster2
	cmd/bin/subctl benchmark throughput --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 \
		--kubecontexts cluster1 --intra-cluster
# cloud
	cmd/bin/subctl cloud prepare generic --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 --kubecontext cluster1
# deploy-broker is tested by the deploy target
# diagnose
	cmd/bin/subctl diagnose all --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1
	cmd/bin/subctl diagnose firewall inter-cluster $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1 $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster2
# export TBD
# gather
	cmd/bin/subctl gather $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1
# join is tested by the deploy target
# show
	cmd/bin/subctl show all --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1
# verify is tested by the e2e target (run elsewhere)
	cmd/bin/subctl uninstall -y --kubeconfig $(DAPPER_OUTPUT)/kubeconfigs/kind-config-cluster1

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
