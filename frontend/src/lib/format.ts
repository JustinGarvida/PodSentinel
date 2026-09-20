/** Formats CPU cores as Kubernetes-style millicores below 1 core, cores above. */
export function formatCPU(cores: number): string {
  if (cores < 1) return `${Math.round(cores * 1000)}m`
  return `${cores.toFixed(2)}`
}

/** Formats a byte count as MiB or GiB, matching Kubernetes' binary units. */
export function formatMemory(bytes: number): string {
  const mib = bytes / (1024 * 1024)
  if (mib >= 1024) return `${(mib / 1024).toFixed(2)}Gi`
  return `${mib.toFixed(1)}Mi`
}
