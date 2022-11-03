<!-- markdownlint-disable MD041 -->
The `subctl show` command now lists the versions of all deployed Submariner components, instead of just the custom resources.

Example output:

```bash
   ✓ Showing versions
  COMPONENT               REPOSITORY           VERSION
  submariner-gateway      quay.io/submariner   0.13.1
  submariner-routeagent   quay.io/submariner   0.13.1
  submariner-globalnet    quay.io/submariner   0.13.1
  submariner-operator     quay.io/submariner   0.13.1
```
