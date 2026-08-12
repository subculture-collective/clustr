import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import VirtualList from './VirtualList';

describe('VirtualList', () => {
  const mockItems = Array.from({ length: 100 }, (_, i) => ({
    id: `item-${i}`,
    value: `Item ${i}`,
  }));

  const mockRenderItem = (item: typeof mockItems[0]) => (
    <div>{item.value}</div>
  );

  const mockItemKey = (item: typeof mockItems[0]) => item.id;

  it('renders virtual list container', () => {
    const { container } = render(
      <VirtualList
        items={mockItems}
        itemHeight={30}
        containerHeight={300}
        renderItem={mockRenderItem}
        itemKey={mockItemKey}
      />
    );

    expect(container.firstChild).toHaveStyle({
      height: '300px',
      overflow: 'auto',
    });
  });

  it('renders only visible items', () => {
    render(
      <VirtualList
        items={mockItems}
        itemHeight={30}
        containerHeight={300}
        renderItem={mockRenderItem}
        itemKey={mockItemKey}
      />
    );

    // Should render items within viewport + overscan
    // With 300px height and 30px items, we can see ~10 items + 3 overscan on each side
    const visibleCount = screen.getAllByText(/Item \d+/).length;
    expect(visibleCount).toBeLessThan(mockItems.length);
    expect(visibleCount).toBeGreaterThan(0);
  });

  it('applies custom className', () => {
    const { container } = render(
      <VirtualList
        items={mockItems}
        itemHeight={30}
        containerHeight={300}
        renderItem={mockRenderItem}
        itemKey={mockItemKey}
        className="custom-class"
      />
    );

    expect(container.firstChild).toHaveClass('custom-class');
  });

  it('uses custom overscan value', () => {
    render(
      <VirtualList
        items={mockItems}
        itemHeight={30}
        containerHeight={300}
        overscan={5}
        renderItem={mockRenderItem}
        itemKey={mockItemKey}
      />
    );

    // Just verify it renders without error
    expect(screen.getAllByText(/Item \d+/).length).toBeGreaterThan(0);
  });

  it('handles scroll events', () => {
    const { container } = render(
      <VirtualList
        items={mockItems}
        itemHeight={30}
        containerHeight={300}
        renderItem={mockRenderItem}
        itemKey={mockItemKey}
      />
    );

    const scrollContainer = container.firstChild as HTMLElement;
    
    // Scroll down
    fireEvent.scroll(scrollContainer, { target: { scrollTop: 300 } });
    
    // Should still have items rendered
    expect(screen.getAllByText(/Item \d+/).length).toBeGreaterThan(0);
  });

  it('handles empty items array', () => {
    const { container } = render(
      <VirtualList
        items={[]}
        itemHeight={30}
        containerHeight={300}
        renderItem={mockRenderItem}
        itemKey={mockItemKey}
      />
    );

    expect(container.firstChild).toBeInTheDocument();
  });

  it('calculates total height correctly', () => {
    const { container } = render(
      <VirtualList
        items={mockItems}
        itemHeight={30}
        containerHeight={300}
        renderItem={mockRenderItem}
        itemKey={mockItemKey}
      />
    );

    const scrollContainer = container.firstElementChild as HTMLElement;
    const spacer = scrollContainer.firstElementChild as HTMLElement;
    const visibleItems = spacer.firstElementChild as HTMLElement;

    expect(spacer).toHaveStyle({ height: '3000px' });
    expect(visibleItems).toHaveStyle({
      position: 'absolute',
    });
  });

  it('uses itemKey for React keys', () => {
    const customKeyFn = vi.fn((item) => `custom-${item.id}`);
    
    render(
      <VirtualList
        items={mockItems.slice(0, 5)}
        itemHeight={30}
        containerHeight={300}
        renderItem={mockRenderItem}
        itemKey={customKeyFn}
      />
    );

    expect(customKeyFn).toHaveBeenCalled();
  });
});
