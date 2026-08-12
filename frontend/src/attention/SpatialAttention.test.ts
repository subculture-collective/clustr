import { describe, expect, it } from 'vitest';
import { attentionIds, createAttention, togglePinned, withSelection } from './SpatialAttention';

describe('spatial attention', () => {
  it('keeps selected and pinned entities through visibility transitions', () => {
    let state = withSelection(createAttention(), 'selected');
    state = togglePinned(state, 'pinned');
    expect([...attentionIds(state)].sort()).toEqual(['pinned', 'selected']);
  });
});
