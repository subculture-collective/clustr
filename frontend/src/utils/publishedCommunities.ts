function stableHash(value: string): number {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

export function publishedCommunityNumericID(id: string): number {
  return stableHash(id);
}

export function publishedCommunityColor(id: string): string {
  return `hsl(${stableHash(id) % 360} 72% 62%)`;
}
