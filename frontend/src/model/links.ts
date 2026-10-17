/** What a location hash asks the app to open. */
export type Link = { kind: 'challenge'; id: string } | { kind: 'design'; id: string }

const CHALLENGE = /(?:^|[#&])challenge=([a-z0-9-]{1,60})(?:&|$)/
const DESIGN = /(?:^|[#&])design=([a-z2-7]{13})(?:&|$)/

/** Parse a location hash such as "#challenge=hello-world" or "#design=3ibupvsc57zfg". */
export function parseHash(hash: string): Link | null {
  const design = DESIGN.exec(hash)
  if (design) return { kind: 'design', id: design[1] }
  const challenge = CHALLENGE.exec(hash)
  if (challenge) return { kind: 'challenge', id: challenge[1] }
  return null
}

/** The shareable address of a saved design. */
export function designUrl(origin: string, id: string): string {
  return `${origin}/#design=${id}`
}
