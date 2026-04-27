import type { Config } from 'tailwindcss'

// Editorial financial-dashboard palette — warm zinc neutrals with a single
// burnished amber accent. Deliberately avoids slate (cold blue cast) and
// generic SaaS purples. Status colours stay perceptually distinct.
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Geist', 'system-ui', 'sans-serif'],
        mono: ['"Geist Mono"', 'ui-monospace', 'SFMono-Regular', 'monospace'],
        display: ['Fraunces', 'Georgia', 'serif'],
      },
      colors: {
        // Surfaces — warm off-white / very dark zinc.
        canvas: '#fafaf7',           // page background, slightly warmer than zinc-50
        surface: '#ffffff',
        muted: '#f4f4ed',
        border: '#e7e5e0',
        // Text
        ink: {
          DEFAULT: '#18181b',         // body
          soft: '#3f3f46',            // labels
          mute: '#71717a',            // captions
          faint: '#a1a1aa',            // tertiary
        },
        // Single brand accent — burnished amber. Used sparingly.
        accent: {
          DEFAULT: '#b45309',         // amber-700
          soft: '#fef3c7',            // amber-100, for backgrounds
          ink: '#78350f',             // amber-900, for text on soft bg
        },
        // Status semantics
        success: { DEFAULT: '#15803d', soft: '#dcfce7', ink: '#14532d' },
        warning: { DEFAULT: '#b45309', soft: '#fef3c7', ink: '#78350f' },
        danger:  { DEFAULT: '#b91c1c', soft: '#fee2e2', ink: '#7f1d1d' },
        info:    { DEFAULT: '#1d4ed8', soft: '#dbeafe', ink: '#1e3a8a' },
      },
      letterSpacing: {
        tightest: '-0.04em',
      },
      fontFeatureSettings: {
        numeric: '"tnum", "lnum"',
      },
    },
  },
  plugins: [],
} satisfies Config
