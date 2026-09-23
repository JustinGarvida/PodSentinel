package k8s

import (
	"context"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ownerCache resolves pods' controlling owners for one poll cycle, memoizing each ReplicaSet-to-Deployment lookup.
type ownerCache struct {
	// core is used to fetch a ReplicaSet's own owner.
	core kubernetes.Interface
	// logger reports failed ReplicaSet lookups at Debug.
	logger *slog.Logger
	// replicaSetOwners maps "namespace/replicaset" to its owner's name, or "" if none.
	replicaSetOwners map[string]string
}

// Purpose: constructs an ownerCache for a single poll cycle.
// Params:
//   - core: the core clientset used to fetch ReplicaSets.
//   - logger: structured logger for lookup failures.
//
// Returns: a ready-to-use *ownerCache with an empty memo.
func newOwnerCache(core kubernetes.Interface, logger *slog.Logger) *ownerCache {
	return &ownerCache{
		core:             core,
		logger:           logger,
		replicaSetOwners: make(map[string]string),
	}
}

// Purpose: resolves a pod's controlling owner, following a ReplicaSet up to its Deployment.
// Params:
//   - ctx: used for the ReplicaSet lookup, if one is needed.
//   - namespace: the pod's namespace.
//   - ownerRefs: the pod's OwnerReferences.
//
// Returns: the highest-level owner's (kind, name), or ("", "") for an unowned pod.
func (c *ownerCache) resolve(ctx context.Context, namespace string, ownerRefs []metav1.OwnerReference) (kind, name string) {
	ref := controllerRef(ownerRefs)
	if ref == nil {
		return "", ""
	}
	if ref.Kind != "ReplicaSet" {
		return ref.Kind, ref.Name
	}
	return c.resolveReplicaSetOwner(ctx, namespace, ref.Name)
}

// Purpose: resolves a ReplicaSet's owning Deployment, memoizing the result for the poll cycle.
// Params:
//   - ctx: used for the Get call on a cache miss.
//   - namespace: the ReplicaSet's namespace.
//   - name: the ReplicaSet's name.
//
// Returns: the owner's (kind, name), or ("ReplicaSet", name) if it has none or the lookup failed.
func (c *ownerCache) resolveReplicaSetOwner(ctx context.Context, namespace, name string) (string, string) {
	key := namespace + "/" + name
	if owner, ok := c.replicaSetOwners[key]; ok {
		if owner == "" {
			return "ReplicaSet", name
		}
		return "Deployment", owner
	}

	rs, err := c.core.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		c.logger.Debug("resolving replicaset's owner failed, using replicaset as owner", "namespace", namespace, "replicaset", name, "error", err)
		c.replicaSetOwners[key] = ""
		return "ReplicaSet", name
	}

	owner := controllerRef(rs.OwnerReferences)
	if owner == nil {
		c.replicaSetOwners[key] = ""
		return "ReplicaSet", name
	}

	c.replicaSetOwners[key] = owner.Name
	return owner.Kind, owner.Name
}

// Purpose: returns the OwnerReference marked as the controller, if any.
// Params:
//   - refs: the object's OwnerReferences.
//
// Returns: a pointer into refs for the controller reference, or nil if none.
func controllerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	return nil
}
