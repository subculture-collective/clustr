import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import FieldGuide from './FieldGuide';

describe('FieldGuide', () => {
  it('explains the scene pipeline and calculation model', () => {
    render(<FieldGuide open onClose={vi.fn()} />);

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'How to read the universe' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'From crawl to constellation' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Why objects end up where they do' })).toBeInTheDocument();
    expect(screen.getByText('Three-axis layout')).toBeInTheDocument();
    expect(screen.getByText('Publication checks')).toBeInTheDocument();
  });

  it('closes with Escape', async () => {
    const onClose = vi.fn();
    render(<FieldGuide open onClose={onClose} />);
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('renders nothing while closed', () => {
    const { container } = render(<FieldGuide open={false} onClose={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });
});
