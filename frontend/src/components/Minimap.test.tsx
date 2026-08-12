import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import Minimap from './Minimap';
import type { CommunityResult } from '../utils/communityDetection';

describe('Minimap', () => {
    const mockCameraMove = vi.fn();
    
    const mockNodes = [
        { id: 'node1', x: 0, y: 0, z: 0 },
        { id: 'node2', x: 100, y: 100, z: 100 },
        { id: 'node3', x: -100, y: -100, z: -100 },
    ];

    const mockCommunityResult: CommunityResult = {
        communities: [
            { id: 0, nodes: ['node1'], size: 1, color: '#ff0000', label: 'Community 1' },
            { id: 1, nodes: ['node2', 'node3'], size: 2, color: '#00ff00', label: 'Community 2' },
        ],
        nodeCommunities: new Map([
            ['node1', 0],
            ['node2', 1],
            ['node3', 1],
        ]),
        modularity: 0.5,
    };

    let originalGetContext: typeof HTMLCanvasElement.prototype.getContext;

    beforeEach(() => {
        vi.clearAllMocks();
        // Save original getContext and mock it
        originalGetContext = HTMLCanvasElement.prototype.getContext;
        HTMLCanvasElement.prototype.getContext = vi.fn(() => ({
            clearRect: vi.fn(),
            fillRect: vi.fn(),
            strokeRect: vi.fn(),
            beginPath: vi.fn(),
            moveTo: vi.fn(),
            lineTo: vi.fn(),
            closePath: vi.fn(),
            arc: vi.fn(),
            fill: vi.fn(),
            stroke: vi.fn(),
            set fillStyle(_: string) {},
            set strokeStyle(_: string) {},
            set lineWidth(_: number) {},
        })) as unknown as typeof HTMLCanvasElement.prototype.getContext;
    });

    afterEach(() => {
        // Restore original getContext
        HTMLCanvasElement.prototype.getContext = originalGetContext;
        vi.restoreAllMocks();
    });

    it('renders canvas when visible', () => {
        render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        const canvas = screen.getByLabelText(/graph minimap/i);
        expect(canvas).toBeInTheDocument();
        expect(canvas).toHaveAttribute('width', '200');
        expect(canvas).toHaveAttribute('height', '200');
    });

    it('uses custom size when provided', () => {
        render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
                size={150}
            />
        );

        const canvas = screen.getByLabelText(/graph minimap/i);
        expect(canvas).toHaveAttribute('width', '150');
        expect(canvas).toHaveAttribute('height', '150');
    });

    it('renders community clusters when provided', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                communityResult={mockCommunityResult}
                onCameraMove={mockCameraMove}
            />
        );

        const canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();
    });

    it('calls onCameraMove when canvas is clicked', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        const canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        // Mock getBoundingClientRect
        canvas!.getBoundingClientRect = vi.fn(() => ({
            left: 0,
            top: 0,
            width: 200,
            height: 200,
            right: 200,
            bottom: 200,
            x: 0,
            y: 0,
            toJSON: () => {},
        }));

        fireEvent.mouseDown(canvas!, { clientX: 100, clientY: 100 });

        expect(mockCameraMove).toHaveBeenCalledTimes(1);
        expect(mockCameraMove).toHaveBeenCalledWith(
            expect.objectContaining({
                x: expect.any(Number),
                y: expect.any(Number),
                z: expect.any(Number),
            })
        );
    });

    it('handles drag operations', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        const canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        canvas!.getBoundingClientRect = vi.fn(() => ({
            left: 0,
            top: 0,
            width: 200,
            height: 200,
            right: 200,
            bottom: 200,
            x: 0,
            y: 0,
            toJSON: () => {},
        }));

        // Start drag
        fireEvent.mouseDown(canvas!, { clientX: 50, clientY: 50 });
        expect(mockCameraMove).toHaveBeenCalledTimes(1);

        // Continue drag
        fireEvent.mouseMove(canvas!, { clientX: 75, clientY: 75 });
        expect(mockCameraMove).toHaveBeenCalledTimes(2);

        // End drag
        fireEvent.mouseUp(canvas!);
        
        // Moving after mouseUp should not trigger onCameraMove
        mockCameraMove.mockClear();
        fireEvent.mouseMove(canvas!, { clientX: 100, clientY: 100 });
        expect(mockCameraMove).not.toHaveBeenCalled();
    });

    it('stops dragging when mouse leaves canvas', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        const canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        canvas!.getBoundingClientRect = vi.fn(() => ({
            left: 0,
            top: 0,
            width: 200,
            height: 200,
            right: 200,
            bottom: 200,
            x: 0,
            y: 0,
            toJSON: () => {},
        }));

        // Start drag
        fireEvent.mouseDown(canvas!, { clientX: 50, clientY: 50 });
        expect(mockCameraMove).toHaveBeenCalledTimes(1);

        // Mouse leaves
        fireEvent.mouseLeave(canvas!);

        // Moving after mouse leave should not trigger onCameraMove
        mockCameraMove.mockClear();
        fireEvent.mouseMove(canvas!, { clientX: 100, clientY: 100 });
        expect(mockCameraMove).not.toHaveBeenCalled();
    });

    it('toggles visibility with M key', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        // Should be visible initially
        let canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        // Press M to hide
        fireEvent.keyDown(window, { key: 'm' });

        // Should be hidden now
        canvas = container.querySelector('canvas');
        expect(canvas).not.toBeInTheDocument();

        // Press M again to show
        fireEvent.keyDown(window, { key: 'm' });

        // Should be visible again
        canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();
    });

    it('does not toggle visibility when typing in input fields', () => {
        const { container } = render(
            <div>
                <input type="text" data-testid="test-input" />
                <Minimap
                    cameraPosition={{ x: 0, y: 0, z: 300 }}
                    nodes={mockNodes}
                    onCameraMove={mockCameraMove}
                />
            </div>
        );

        // Should be visible initially
        let canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        const input = screen.getByTestId('test-input');
        
        // Press M while focused on input
        fireEvent.keyDown(input, { key: 'm' });

        // Should still be visible
        canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();
    });

    it('does not toggle with M key when modifiers are pressed', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        // Should be visible initially
        let canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        // Press Ctrl+M (should not toggle)
        fireEvent.keyDown(window, { key: 'm', ctrlKey: true });
        canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        // Press Alt+M (should not toggle)
        fireEvent.keyDown(window, { key: 'm', altKey: true });
        canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        // Press Shift+M (should not toggle)
        fireEvent.keyDown(window, { key: 'm', shiftKey: true });
        canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();
    });

    it('renders nothing when not visible', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        // Initially visible
        expect(container.querySelector('canvas')).toBeInTheDocument();

        // Hide with M key
        fireEvent.keyDown(window, { key: 'm' });

        // Should render nothing
        expect(container.querySelector('canvas')).not.toBeInTheDocument();
    });

    it('handles empty nodes array gracefully', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={[]}
                onCameraMove={mockCameraMove}
            />
        );

        const canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();
    });

    it('handles missing camera position', () => {
        const { container } = render(
            <Minimap
                nodes={mockNodes}
                onCameraMove={mockCameraMove}
            />
        );

        const canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();
    });

    it('does not call onCameraMove when callback is not provided', () => {
        const { container } = render(
            <Minimap
                cameraPosition={{ x: 0, y: 0, z: 300 }}
                nodes={mockNodes}
            />
        );

        const canvas = container.querySelector('canvas');
        expect(canvas).toBeInTheDocument();

        canvas!.getBoundingClientRect = vi.fn(() => ({
            left: 0,
            top: 0,
            width: 200,
            height: 200,
            right: 200,
            bottom: 200,
            x: 0,
            y: 0,
            toJSON: () => {},
        }));

        // Should not throw error
        expect(() => {
            fireEvent.mouseDown(canvas!, { clientX: 100, clientY: 100 });
        }).not.toThrow();
    });
});
