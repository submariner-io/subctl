<!-- markdownlint-disable MD041 -->
The following deprecated commands and variants have been removed:

* `subctl benchmark`’s `--kubecontexts` option (use `--context` and
  `--tocontext` instead)
* `subctl benchmark`’s `--intra-cluster` option (specify a single
  context to run intra-cluster benchmarks)
* `subctl benchmark` with two `kubeconfigs` as command-line arguments
* `subctl cloud`’s `--metrics-ports` option
* `subctl deploy-broker`’s `--broker-namespace` option (use
  `--namespace` instead)
* `subctl diagnose firewall metrics` (this is checked during
  deployment)
* `subctl diagnose firewall intra-cluster` with two `kubeconfigs` as
  command-line arguments
* `subctl diagnose firewall inter-cluster` with two `kubeconfigs` as
  command-line arguments
* `subctl gather`’s `--kubecontexts` option (use `--contexts` instead)
