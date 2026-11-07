import { describe, expect, it } from 'vitest';

import { filterLinkedOnly } from './linkedOnlyFilter';

const landmarks = [
    { id: 'c:alpha', type: 'community' },
    { id: 'c:beta', type: 'community' },
    { id: 'c:gamma', type: 'community' },
];

describe('filterLinkedOnly', () => {
    it('returns the scene unchanged when it carries no links', () => {
        const nodes = [{ id: 'subreddit_one', type: 'subreddit' }, { id: 'user_two', type: 'user' }];
        expect(filterLinkedOnly(nodes, [], false)).toBe(nodes);
    });

    it('drops isolated entities from a scene that carries links', () => {
        const nodes = [
            { id: 'subreddit_one', type: 'subreddit' },
            { id: 'user_two', type: 'user' },
            { id: 'post_three', type: 'post' },
        ];
        const links = [{ source: 'user_two', target: 'post_three' }];
        expect(filterLinkedOnly(nodes, links, true).map(node => node.id)).toEqual(['user_two', 'post_three']);
    });

    it('keeps community landmarks that the overview link budget left isolated', () => {
        const nodes = [...landmarks, { id: 'user_reader', type: 'user' }];
        const links = [{ source: 'c:alpha', target: 'c:beta' }];
        expect(filterLinkedOnly(nodes, links, true).map(node => node.id)).toEqual(['c:alpha', 'c:beta', 'c:gamma']);
    });
});
