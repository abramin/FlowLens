import { useCallback, useEffect, useRef, useState } from 'react';
import type { URLState } from '../utils/urlState';
import {
  getURLState,
  setURLState,
  clearURLState,
  copyShareLink as copyShareLinkUtil,
} from '../utils/urlState';

interface UseURLStateReturn {
  initialState: URLState | null;
  updateURL: (state: URLState) => void;
  copyShareLink: (state: URLState) => Promise<boolean>;
  clearURL: () => void;
}

export function useURLState(): UseURLStateReturn {
  const [initialState] = useState<URLState | null>(() => getURLState());
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, []);

  const updateURL = useCallback((state: URLState) => {
    // Debounce URL updates to avoid excessive history entries
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }
    debounceRef.current = setTimeout(() => {
      // Only update URL if there's an entrypoint selected
      if (state.entrypointId !== null) {
        setURLState(state);
      } else {
        clearURLState();
      }
    }, 300);
  }, []);

  const copyShareLink = useCallback(async (state: URLState): Promise<boolean> => {
    try {
      await copyShareLinkUtil(state);
      return true;
    } catch {
      return false;
    }
  }, []);

  const clearURL = useCallback(() => {
    clearURLState();
  }, []);

  return {
    initialState,
    updateURL,
    copyShareLink,
    clearURL,
  };
}
