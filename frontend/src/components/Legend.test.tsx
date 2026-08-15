import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import Legend from './Legend';

describe('Legend', () => {
  const mockFilters = {
    subreddit: true,
    user: true,
    post: false,
    comment: false,
  };

  it('renders legend title', () => {
    render(<Legend filters={mockFilters} />);
    expect(screen.getByText('Legend')).toBeInTheDocument();
  });

  it('renders node type colors when not using community colors', () => {
    render(<Legend filters={mockFilters} useCommunityColors={false} />);
    
    expect(screen.getByText('Subreddit')).toBeInTheDocument();
    expect(screen.getByText('User')).toBeInTheDocument();
    expect(screen.queryByText('Post')).not.toBeInTheDocument();
    expect(screen.queryByText('Comment')).not.toBeInTheDocument();
  });

  it('filters node types based on filters prop', () => {
    const allFilters = {
      subreddit: true,
      user: true,
      post: true,
      comment: true,
    };
    
    render(<Legend filters={allFilters} />);
    
    expect(screen.getByText('Subreddit')).toBeInTheDocument();
    expect(screen.getByText('User')).toBeInTheDocument();
    expect(screen.getByText('Post')).toBeInTheDocument();
    expect(screen.getByText('Comment')).toBeInTheDocument();
  });

  it('renders community colors section when enabled', () => {
    render(
      <Legend
        filters={mockFilters}
        useCommunityColors={true}
        communityCount={5}
      />
    );
    
    expect(screen.getByText('5 loaded groups')).toBeInTheDocument();
    expect(screen.getByText(/Hue = macro-group/)).toBeInTheDocument();
    expect(screen.queryByText('Subreddit')).not.toBeInTheDocument();
  });

  it('renders communities without count', () => {
    render(
      <Legend
        filters={mockFilters}
        useCommunityColors={true}
      />
    );
    
    expect(screen.getByText('Macro-group hue')).toBeInTheDocument();
  });

  it('always renders size legend', () => {
    render(<Legend filters={mockFilters} />);
    expect(screen.getByText(/Node size = degree/)).toBeInTheDocument();
  });

  it('shows only filtered node types', () => {
    const singleFilter = {
      subreddit: false,
      user: true,
      post: false,
      comment: false,
    };
    
    render(<Legend filters={singleFilter} />);
    
    expect(screen.queryByText('Subreddit')).not.toBeInTheDocument();
    expect(screen.getByText('User')).toBeInTheDocument();
  });
});
