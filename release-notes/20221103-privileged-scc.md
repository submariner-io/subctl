<!-- markdownlint-disable MD041 -->
The privileged SCC permission for Submariner components in OCP is set now by
creating separate `ClusterRole` and `ClusterRoleBinding` resources instead of
manipulating the system privileged SCC resource.
