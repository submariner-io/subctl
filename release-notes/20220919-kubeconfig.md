<!-- markdownlint-disable MD041 -->
kubeconfig handling has been revamped to be consistent across all
`subctl` commands and to match `kubectl`’s behaviour.

The single-context commands, `cloud-prepare`, `deploy-broker`, `export`,
 `join`, `unexport` and `uninstall`, now all support a `--context` argument
to specify the kubeconfig context to use. kubeconfig files can be
specified using either the `KUBECONFIG` environment variable or the
`--kubeconfig` argument; `kubectl` defaults will be applied if
configured. If no context is specified, the kubeconfig default context
will be used.

Multiple-context commands which operate on all contexts by default,
`show` and `gather`, support a `--contexts` argument which can be used
to select one or more contexts; they also support the `--context` argument
to select a single context.

Multiple-context commands which operate on specific contexts,
`benchmark` and `verify`, support a `--context` argument to specify the
originating context, and a `--tocontext` argument to specify the target
context.

`diagnose` operates on all accessible contexts by default, except
`diagnose firewall inter-cluster` and `diagnose firewall nat-traversal`
which rely on an originating context specified by `--context` and a
remote context specified by `--remotecontext`.

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
