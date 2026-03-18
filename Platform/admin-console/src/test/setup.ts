import '@testing-library/jest-dom/vitest';
import { vi } from 'vitest';

const originalConsoleWarn = console.warn;
const originalConsoleError = console.error;

vi.spyOn(console, 'warn').mockImplementation((...args: unknown[]) => {
  const [firstArg] = args;
  if (typeof firstArg === 'string' && firstArg.includes('React Router Future Flag Warning')) {
    return;
  }

  originalConsoleWarn(...args);
});

vi.spyOn(console, 'error').mockImplementation((...args: unknown[]) => {
  const [firstArg] = args;
  if (typeof firstArg === 'string' && firstArg.includes('not wrapped in act')) {
    return;
  }

  originalConsoleError(...args);
});
