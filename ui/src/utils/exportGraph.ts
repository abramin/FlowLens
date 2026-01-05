import { toSvg, toPng } from 'html-to-image';

export interface ExportOptions {
  backgroundColor?: string;
  padding?: number;
  quality?: number;
}

const defaultOptions: ExportOptions = {
  backgroundColor: '#111827', // gray-900
  padding: 20,
  quality: 1,
};

function downloadFile(dataUrl: string, filename: string): void {
  const link = document.createElement('a');
  link.download = filename;
  link.href = dataUrl;
  link.click();
}

function generateFilename(prefix: string, extension: string): string {
  const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
  return `${prefix}-${timestamp}.${extension}`;
}

export async function exportToSVG(
  element: HTMLElement,
  filenamePrefix: string = 'flowlens-graph',
  options: ExportOptions = {}
): Promise<void> {
  const opts = { ...defaultOptions, ...options };

  const dataUrl = await toSvg(element, {
    backgroundColor: opts.backgroundColor,
    style: {
      padding: `${opts.padding}px`,
    },
    filter: (node) => {
      // Exclude controls and minimap from export
      if (node instanceof HTMLElement) {
        const classList = node.classList;
        if (
          classList.contains('react-flow__controls') ||
          classList.contains('react-flow__minimap') ||
          classList.contains('react-flow__background')
        ) {
          return false;
        }
      }
      return true;
    },
  });

  downloadFile(dataUrl, generateFilename(filenamePrefix, 'svg'));
}

export async function exportToPNG(
  element: HTMLElement,
  filenamePrefix: string = 'flowlens-graph',
  options: ExportOptions = {}
): Promise<void> {
  const opts = { ...defaultOptions, ...options };

  const dataUrl = await toPng(element, {
    backgroundColor: opts.backgroundColor,
    quality: opts.quality,
    pixelRatio: 2, // Higher resolution for better quality
    style: {
      padding: `${opts.padding}px`,
    },
    filter: (node) => {
      // Exclude controls and minimap from export
      if (node instanceof HTMLElement) {
        const classList = node.classList;
        if (
          classList.contains('react-flow__controls') ||
          classList.contains('react-flow__minimap') ||
          classList.contains('react-flow__background')
        ) {
          return false;
        }
      }
      return true;
    },
  });

  downloadFile(dataUrl, generateFilename(filenamePrefix, 'png'));
}

export function getReactFlowContainer(): HTMLElement | null {
  return document.querySelector('.react-flow') as HTMLElement | null;
}
