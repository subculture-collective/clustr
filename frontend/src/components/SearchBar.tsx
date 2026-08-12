import { useState, useEffect, useRef, useCallback, forwardRef, useImperativeHandle } from 'react';

interface SearchBarProps {
  onSelectNode: (nodeId: string) => void;
  className?: string;
}

export interface SearchBarHandle {
  focus: () => void;
}

interface SearchResult {
  id: string;
  name: string;
  val?: string;
  type?: string;
}

// Backend API response structure from sqlc-generated code
interface ApiSearchResultRow {
  ID: string;
  Name: string;
  Val: string;
  Type: {
    String: string;
    Valid: boolean;
  } | null;
  PosX?: { Float64: number; Valid: boolean } | null;
  PosY?: { Float64: number; Valid: boolean } | null;
  PosZ?: { Float64: number; Valid: boolean } | null;
}

const SearchBar = forwardRef<SearchBarHandle, SearchBarProps>(({ onSelectNode, className = '' }, ref) => {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Expose focus method via ref
  useImperativeHandle(ref, () => ({
    focus: () => {
      inputRef.current?.focus();
    }
  }), []);

  // Debounced search function using API
  const performSearch = useCallback(
    async (searchQuery: string) => {
      if (!searchQuery.trim()) {
        setResults([]);
        setIsOpen(false);
        return;
      }

      // Cancel previous request if still pending
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      abortControllerRef.current = new AbortController();
      setIsLoading(true);
      // Clear previous results to avoid showing stale data
      setResults([]);

      try {
        const startTime = performance.now();
        const apiUrl = (import.meta.env.VITE_API_URL || '/api').replace(/\/$/, '');
        const response = await fetch(
          `${apiUrl}/search?node=${encodeURIComponent(searchQuery)}&limit=10`,
          { signal: abortControllerRef.current.signal }
        );

        if (response.ok) {
          const data = await response.json();
          const endTime = performance.now();
          
          if (import.meta.env.DEV) {
            console.log(`Search completed in ${(endTime - startTime).toFixed(2)}ms`);
          }

          // Map backend response to frontend format
          const rawResults = (data.results || []) as ApiSearchResultRow[];
          const apiResults: SearchResult[] = rawResults.map((row) => ({
            id: row.ID,
            name: row.Name,
            val: row.Val,
            type: row.Type && row.Type.Valid ? row.Type.String : undefined,
          }));

          setResults(apiResults);
          setIsOpen(true); // Always open to show results or "no results" message
          setSelectedIndex(0);
        } else {
          // Handle non-OK responses by clearing results and closing dropdown
          setResults([]);
          setIsOpen(false);
          console.error(`Search API returned status ${response.status}`);
        }
      } catch (error) {
        if (error instanceof Error && error.name !== 'AbortError') {
          console.error('API search failed:', error);
          // Clear results on error
          setResults([]);
          setIsOpen(false);
        }
      } finally {
        setIsLoading(false);
      }
    },
    []
  );

  // Select a node and focus on it
  const selectNode = useCallback(
    (result: SearchResult) => {
      // Clear pending timers and requests before selecting
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current);
        debounceTimer.current = undefined;
      }
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
        abortControllerRef.current = null;
      }

      onSelectNode(result.id);
      setQuery('');
      setIsOpen(false);
      setResults([]);
      inputRef.current?.blur();
    },
    [onSelectNode]
  );

  // Handle input change with debouncing
  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      setQuery(value);

      // Clear previous timer
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current);
      }

      // Set new timer
      debounceTimer.current = setTimeout(() => {
        performSearch(value);
      }, 150);
    },
    [performSearch]
  );

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (!isOpen) return;

      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          setSelectedIndex((prev) => (prev < results.length - 1 ? prev + 1 : prev));
          break;
        case 'ArrowUp':
          e.preventDefault();
          setSelectedIndex((prev) => (prev > 0 ? prev - 1 : 0));
          break;
        case 'Enter':
          e.preventDefault();
          if (results[selectedIndex]) {
            selectNode(results[selectedIndex]);
          }
          break;
        case 'Escape':
          e.preventDefault();
          // Clear pending timers and requests on Escape
          if (debounceTimer.current) {
            clearTimeout(debounceTimer.current);
            debounceTimer.current = undefined;
          }
          if (abortControllerRef.current) {
            abortControllerRef.current.abort();
            abortControllerRef.current = null;
          }
          setIsOpen(false);
          setQuery('');
          setResults([]);
          inputRef.current?.blur();
          break;
      }
    },
    [isOpen, results, selectedIndex, selectNode]
  );

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        inputRef.current &&
        !inputRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      // Clear pending timer
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current);
      }
      // Abort pending requests
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  // Get node color based on type
  const getNodeColor = (type?: string) => {
    switch (type) {
      case 'subreddit':
        return 'bg-[#b6ff62]';
      case 'user':
        return 'bg-[#68dcff]';
      case 'post':
        return 'bg-[#ffc75f]';
      case 'comment':
        return 'bg-[#ff7096]';
      default:
        return 'bg-[#d8e7e3]';
    }
  };

  // Get node icon based on type
  const getNodeIcon = (type?: string) => {
    switch (type) {
      case 'subreddit':
        return 'SR';
      case 'user':
        return 'US';
      case 'post':
        return 'PO';
      case 'comment':
        return 'CO';
      default:
        return 'ID';
    }
  };

  return (
    <div className={`relative ${className}`} role="search">
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          placeholder="Search nodes in the universe"
          aria-label="Search communities, people, posts, and comments"
          className="instrument-panel h-12 w-full rounded-full px-5 pr-20 text-sm text-white placeholder:text-[#61706e] md:h-14"
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={isOpen && results.length > 0}
          aria-controls="search-results-listbox"
          aria-activedescendant={
            isOpen && selectedIndex >= 0 && selectedIndex < results.length
              ? `search-result-${results[selectedIndex].id}`
              : undefined
          }
        />
        {isLoading && (
          <div className="absolute right-3 top-1/2 -translate-y-1/2">
            <div className="h-4 w-4 animate-spin rounded-full border border-[#b6ff62]/30 border-t-[#b6ff62]"></div>
          </div>
        )}
        {!isLoading && query && (
          <button
            onClick={() => {
              // Clear pending timers and requests on clear
              if (debounceTimer.current) {
                clearTimeout(debounceTimer.current);
                debounceTimer.current = undefined;
              }
              if (abortControllerRef.current) {
                abortControllerRef.current.abort();
                abortControllerRef.current = null;
              }
              setQuery('');
              setResults([]);
              setIsOpen(false);
            }}
            className="instrument-button absolute right-3 top-1/2 h-8 w-8 -translate-y-1/2 rounded-full text-[#9aaba8]"
            aria-label="Clear search"
          >
            ✕
          </button>
        )}
      </div>

      {isOpen && results.length > 0 && (
        <div
          ref={dropdownRef}
          id="search-results-listbox"
          role="listbox"
          className="instrument-panel absolute top-full z-50 mt-2 max-h-96 w-full overflow-y-auto rounded-2xl p-2"
        >
          {results.map((result, index) => {
            const isSelected = index === selectedIndex;

            return (
              <button
                key={result.id}
                id={`search-result-${result.id}`}
                role="option"
                aria-selected={isSelected}
                onClick={() => selectNode(result)}
                onMouseEnter={() => setSelectedIndex(index)}
                className={`flex w-full items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors ${
                  isSelected ? 'bg-white/[.08]' : 'hover:bg-white/[.04]'
                }`}
              >
                <div className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full font-mono text-[9px] font-semibold text-[#030506] ${getNodeColor(result.type)}`}>{getNodeIcon(result.type)}</div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium text-white">{result.name}</span>
                    <span className="instrument-label flex-shrink-0">
                      {result.type || 'unknown'}
                    </span>
                  </div>
                  <div className="mt-1 flex items-center gap-2 font-mono text-[10px] text-[#61706e]">
                    <span className="truncate">{result.id}</span>
                    {result.val !== undefined && (
                      <>
                        <span>•</span>
                        <span>weight: {result.val}</span>
                      </>
                    )}
                  </div>
                </div>
                {isSelected && (
                  <div className="flex-shrink-0 font-mono text-xs text-[#b6ff62]">ENTER</div>
                )}
              </button>
            );
          })}
        </div>
      )}

      {isOpen && query && results.length === 0 && !isLoading && (
        <div
          ref={dropdownRef}
          className="instrument-panel absolute top-full z-50 mt-2 w-full rounded-2xl px-5 py-4 text-sm text-[#9aaba8]"
        >
          No results found for "{query}"
        </div>
      )}
    </div>
  );
});

SearchBar.displayName = 'SearchBar';

export default SearchBar;
