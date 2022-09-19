<!-- markdownlint-disable MD041 -->
kubeconfig handling has been revamped to be consistent across all
single-context commands and to match `kubectl`’s behaviour.

The `cloud-prepare`, `deploy-broker`, `export`, `join`, `unexport` and
`uninstall` `subctl` commands now all support a `--context` argument
to specify the kubeconfig context to use. kubeconfig files can be
specified using either the `KUBECONFIG` environment variable or the
`--kubeconfig` argument; `kubectl` defaults will be applied if
configured. If no context is specified, the kubeconfig default context
will be used.

Namespace-based commands such as `export` will use the namespace given
using `--namespace` (`-n`), if any, or the current namespace in the
selected context, if there is one, rather than the `default`
namespace.

These commands also support all connection options supported by
`kubectl`, so connections can be configured using command arguments
instead of kubeconfigs.

Existing options (`--kubecontext` etc.) are preserved for backwards
compatibility, but are deprecated and will be removed in the next
release.
