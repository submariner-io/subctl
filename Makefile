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

PLATFORMS ?= linux/amd64,linux/arm64
IMAGES = subctl
PRELOAD_IMAGES := $(IMAGES) submariner-operator submariner-gateway submariner-globalnet submariner-route-agent lighthouse-agent lighthouse-coredns nettest
MULTIARCH_IMAGES := subctl
undefine SKIP
undefine FOCUS
undefine E2E_TESTDIR

ifneq (,$(filter ovn,$(_using)))
SETTINGS = $(DAPPER_SOURCE)/.shipyard.e2e.ovn.yml
else
SETTINGS = $(DAPPER_SOURCE)/.shipyard.e2e.yml
endif

include $(SHIPYARD_DIR)/Makefile.inc

gotodockerarch = $(patsubst arm,arm/v7,$(1))
dockertogoarch = $(patsubst arm/v7,arm,$(1))

CROSS_TARGETS := linux-amd64 linux-arm64 linux-arm linux-s390x linux-ppc64le windows-amd64.exe darwin-amd64
BINARIES := cmd/bin/subctl
CROSS_BINARIES := $(foreach cross,$(CROSS_TARGETS),$(patsubst %,cmd/bin/subctl-$(VERSION)-%,$(cross)))
CROSS_TARBALLS := $(foreach cross,$(CROSS_TARGETS),$(patsubst %,dist/subctl-$(VERSION)-%.tar.xz,$(cross)))

override E2E_ARGS += cluster1 cluster2
override SYSTEM_ARGS += --settings $(SETTINGS) cluster1 cluster2
export DEPLOY_ARGS
override UNIT_TEST_ARGS += test internal/env
override VALIDATE_ARGS += --skip-dirs pkg/client

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
export PATH := $(CURDIR)/cmd/bin:$(PATH)

# Targets to make

# Build subctl before deploying to ensure we use that
# (with the PATH set above)
deploy: cmd/bin/subctl

# [system-test] runs system level tests for the various `subctl` commands
system-test:
	scripts/test/system.sh $(SYSTEM_ARGS)

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

cmd/bin/subctl: cmd/bin/subctl-$(VERSION)-$(GOOS)-$(GOARCH)$(GOEXE)
	ln -sf $(<F) $@

dist/subctl-%.tar.xz: cmd/bin/subctl-%
	mkdir -p dist
	tar -cJf $@ --transform "s/^cmd.bin/subctl-$(VERSION)/" $<

# Versions may include hyphens so it's easier to use $(VERSION) than to extract them from the target

# Special case for Linux container builds
# Our container builds look for cmd/bin/linux/{amd64,arm64}/subctl,
# this builds the corresponding subctl using our distribution nomenclature
# and links it to the name expected by the container build.
cmd/bin/linux/%/subctl: cmd/bin/subctl-$(VERSION)-linux-%
	mkdir -p $(dir $@)
	ln -sf ../../$(<F) $@

.PRECIOUS: cmd/bin/subctl-%
cmd/bin/subctl-%: $(shell find . -name "*.go") $(VENDOR_MODULES)
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

.PHONY: build ci clean generate-clientset system-test

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
