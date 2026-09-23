package k8s

import (
	"context"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ownerCache resolves each pod's controlling owner for one poll
// cycle. A Deployment-managed pod's direct owner is its ReplicaSet,
// not the Deployment, so resolving "Deployment" needs one extra API
// hop; this cache memoizes that hop per ReplicaSet so pods sharing one
// ReplicaSet cost a single Get, not one per pod.
type ownerCache struct {
	// core is used to fetch a ReplicaSet's own owner.
	core kubernetes.Interface
	// logger reports (at Debug) a failed or absent ReplicaSet lookup.
	logger *slog.Logger
	// replicaSetOwners caches "namespace/replicaset" -> the
	// ReplicaSet's own owner name, or "" if it has none (or the
	// lookup failed) so a repeat isn't retried within the same cycle.
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

// Purpose: resolves a pod's controlling owner, following a
// ReplicaSet owner up to its own owning Deployment when present.
// Params:
//   - ctx: used for the ReplicaSet lookup, if one is needed.
//   - namespace: the pod's namespace, also the ReplicaSet's namespace.
//   - ownerRefs: the pod's OwnerReferences.
//
// Returns: the highest-level owner's (kind, name) — e.g.
// ("Deployment", "web"), ("ReplicaSet", "web-7d9f8c6b5d") if that
// ReplicaSet has no further owner or the lookup failed, ("StatefulSet",
// "web") for a directly-owned pod, or ("", "") for an unowned pod.
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

// Purpose: resolves one ReplicaSet's own controlling owner (its
// Deployment, if any), memoizing the result for the rest of the poll
// cycle.
// Params:
//   - ctx: used for the Get call on a cache miss.
//   - namespace: the ReplicaSet's namespace.
//   - name: the ReplicaSet's name.
//
// Returns: ("Deployment", name) if found, or ("ReplicaSet",
// replicaSetName) if the ReplicaSet has no controller owner or the
// lookup failed (logged at Debug).
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

// Purpose: returns the OwnerReference marked as the controller, if
// any — a Kubernetes object has at most one.
// Params:
//   - refs: the object's OwnerReferences.
//
// Returns: a pointer into refs for the controller reference, or nil
// if none is marked as the controller.
func controllerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	return nil
}
