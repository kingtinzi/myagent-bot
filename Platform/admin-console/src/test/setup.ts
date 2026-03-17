import '@testing-library/jest-dom/vitest';
import { vi } from 'vitest';

const originalConsoleWarn = console.warn;

vi.spyOn(console, 'warn').mockImplementation((...args: unknown[]) => {
  const [firstArg] = args;
  if (typeof firstArg === 'string' && firstArg.includes('React Router Future Flag Warning')) {
    return;
  }

  originalConsoleWarn(...args);
});
