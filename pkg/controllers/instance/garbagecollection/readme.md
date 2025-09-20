## instance garbage collection controller

- background

When agentpool is in the Creating process, if user starts to delete NodeClaim such as `kubectl delete nodeclaim {NodeClaimName}`, [nodeclaim termination controller](https://github.com/kubernetes-sigs/karpenter/blob/v1.0.4/pkg/controllers/nodeclaim/termination/controller.go) will remove the termination finalizer of NodeClaim, then NodeClaim will be removed from the cluster. But agentpoll is still in the Creating process, this means agentpool and related node will be leaked.

- solution

A new garbage collection controller named [instance garbage collection] is used for garbaging leaked agentpool and node.

  1. if agentpool related NodeClaim is removed in the cluster, and agentpool is created more than 30s, [instance garbage collection] controller will delete the agentpool resource.
  2. if the leaked agentpool has related nodes, [instance garbage collection] controller will also delete node resource.

## others

[nodeclaim.garbagecollection controller](https://github.com/kubernetes-sigs/karpenter/blob/v1.0.4/pkg/controllers/nodeclaim/garbagecollection/controller.go) will not take effect in our scenario. When the backend agent pool is removed, it triggers the [node termination controller], which in turn triggers the [nodeclaim termination controller]. As a result, no NodeClaims will be leaked when backend agent pools are removed.