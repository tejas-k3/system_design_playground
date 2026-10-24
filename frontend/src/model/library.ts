const GROUPS: { title: string; kinds: string[] }[] = [
  { title: 'Traffic', kinds: ['client', 'loadBalancer', 'cdn', 'rateLimiter'] },
  { title: 'Compute', kinds: ['appServer', 'worker'] },
  { title: 'Data', kinds: ['database', 'readReplica', 'cache', 'redis', 'shardRouter', 'kvStore', 'coordinator', 'index'] },
  { title: 'Async and external', kinds: ['queue', 'topic', 'log', 'stream', 'objectStore', 'externalApi'] },
]

/** Component kinds grouped for the library panel; the order of `kinds` is kept inside a group and unknown kinds go in 'Other'. */
export function groupKinds(kinds: string[]): { title: string; kinds: string[] }[] {
  const known = new Set(GROUPS.flatMap((g) => g.kinds))
  const out = GROUPS.map((g) => ({ title: g.title, kinds: kinds.filter((k) => g.kinds.includes(k)) }))
  out.push({ title: 'Other', kinds: kinds.filter((k) => !known.has(k)) })
  return out.filter((g) => g.kinds.length > 0)
}
