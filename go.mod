module github.com/submariner-io/subctl

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.4
	github.com/Azure/go-autorest/autorest v0.11.24 // indirect
	github.com/coreos/go-semver v0.3.0
	github.com/go-logr/logr v0.4.0
	github.com/gophercloud/utils v0.0.0-20210909165623-d7085207ff6d
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.19.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.4.0
	github.com/submariner-io/admiral v0.13.0-m1
	github.com/submariner-io/cloud-prepare v0.13.0-m1
	github.com/submariner-io/lighthouse v0.13.0-m1
	github.com/submariner-io/shipyard v0.13.0-m1
	github.com/submariner-io/submariner v0.13.0-m1
	github.com/urfave/cli v1.22.1 // indirect
	github.com/uw-labs/lichen v0.1.7
	golang.org/x/oauth2 v0.0.0-20220411215720-9780585627b5
	golang.org/x/text v0.3.7
	google.golang.org/api v0.79.0
	k8s.io/api v0.21.11
	k8s.io/apiextensions-apiserver v0.21.11
	k8s.io/apimachinery v0.21.11
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog/v2 v2.40.1 // indirect
	k8s.io/utils v0.0.0-20211116205334-6203023598ed
	sigs.k8s.io/controller-runtime v0.9.7
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/mcs-api v0.1.0
	sigs.k8s.io/yaml v1.3.0
)

// When changing pins, check the dependabot configuration too
// in .github/dependabot.yml

// Pinned to kubernetes-1.21.11
replace (
	k8s.io/client-go => k8s.io/client-go v0.21.11
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.9.0
)
