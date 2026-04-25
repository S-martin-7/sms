import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // status colors used by the Badge component
        status: {
          queued: '#94a3b8',      // slate-400
          sending: '#60a5fa',     // blue-400
          sent: '#34d399',        // emerald-400
          delivered: '#10b981',   // emerald-500
          undelivered: '#f59e0b', // amber-500
          rejected: '#ef4444',    // red-500
          failed: '#dc2626',      // red-600
          pending: '#94a3b8',
          in_flight: '#60a5fa',
          success: '#10b981',
          dead: '#7f1d1d',        // red-900
        },
      },
    },
  },
  plugins: [],
} satisfies Config
