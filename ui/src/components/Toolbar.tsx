import { useState, useCallback } from 'react';

interface ToolbarProps {
  onCopyLink: () => Promise<boolean>;
  onExportSVG: () => Promise<void>;
  onExportPNG: () => Promise<void>;
  disabled: boolean;
}

type ToastType = 'success' | 'error';

export function Toolbar({ onCopyLink, onExportSVG, onExportPNG, disabled }: ToolbarProps) {
  const [toast, setToast] = useState<{ message: string; type: ToastType } | null>(null);
  const [exporting, setExporting] = useState<'svg' | 'png' | null>(null);

  const showToast = useCallback((message: string, type: ToastType) => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 2000);
  }, []);

  const handleCopyLink = useCallback(async () => {
    const success = await onCopyLink();
    if (success) {
      showToast('Link copied to clipboard', 'success');
    } else {
      showToast('Failed to copy link', 'error');
    }
  }, [onCopyLink, showToast]);

  const handleExportSVG = useCallback(async () => {
    setExporting('svg');
    try {
      await onExportSVG();
      showToast('SVG exported', 'success');
    } catch {
      showToast('Failed to export SVG', 'error');
    } finally {
      setExporting(null);
    }
  }, [onExportSVG, showToast]);

  const handleExportPNG = useCallback(async () => {
    setExporting('png');
    try {
      await onExportPNG();
      showToast('PNG exported', 'success');
    } catch {
      showToast('Failed to export PNG', 'error');
    } finally {
      setExporting(null);
    }
  }, [onExportPNG, showToast]);

  return (
    <div className="relative flex items-center gap-2 px-3 py-2 bg-gray-800 border-b border-gray-700">
      <button
        onClick={handleCopyLink}
        disabled={disabled}
        className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-300 bg-gray-700 hover:bg-gray-600 disabled:opacity-50 disabled:cursor-not-allowed rounded transition-colors"
        title="Copy shareable link to clipboard"
      >
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
        </svg>
        Copy Link
      </button>

      <button
        onClick={handleExportSVG}
        disabled={disabled || exporting !== null}
        className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-300 bg-gray-700 hover:bg-gray-600 disabled:opacity-50 disabled:cursor-not-allowed rounded transition-colors"
        title="Export graph as SVG file"
      >
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
        </svg>
        {exporting === 'svg' ? 'Exporting...' : 'Export SVG'}
      </button>

      <button
        onClick={handleExportPNG}
        disabled={disabled || exporting !== null}
        className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-300 bg-gray-700 hover:bg-gray-600 disabled:opacity-50 disabled:cursor-not-allowed rounded transition-colors"
        title="Export graph as PNG file"
      >
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
        {exporting === 'png' ? 'Exporting...' : 'Export PNG'}
      </button>

      {/* Toast notification */}
      {toast && (
        <div
          className={`absolute right-3 top-1/2 -translate-y-1/2 px-3 py-1.5 text-xs font-medium rounded shadow-lg transition-opacity ${
            toast.type === 'success'
              ? 'bg-green-600 text-white'
              : 'bg-red-600 text-white'
          }`}
        >
          {toast.message}
        </div>
      )}
    </div>
  );
}
