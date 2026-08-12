export type AttentionState = { hovered?: string; selected?: string; pinned: ReadonlySet<string> };

export function createAttention(): AttentionState { return { pinned: new Set() }; }
export function withHover(state: AttentionState, hovered?: string): AttentionState { return { ...state, hovered }; }
export function withSelection(state: AttentionState, selected?: string): AttentionState { return { ...state, selected }; }
export function togglePinned(state: AttentionState, id: string): AttentionState {
  const pinned = new Set(state.pinned);
  if (pinned.has(id)) pinned.delete(id); else pinned.add(id);
  return { ...state, pinned };
}
export function attentionIds(state: AttentionState): ReadonlySet<string> {
  return new Set([state.hovered, state.selected, ...state.pinned].filter((id): id is string => Boolean(id)));
}
