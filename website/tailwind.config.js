/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './src/**/*.{js,jsx,ts,tsx,md,mdx}',
    './docs/**/*.{md,mdx}',
  ],
  // CRITICAL: disable Tailwind's CSS reset — Infima must stay as base theme.
  corePlugins: {
    preflight: false,
  },
  theme: {
    extend: {
      colors: {
        // Mirror the graft orange palette (maps to --ifm-color-primary* vars).
        // Use CSS var references so Tailwind utilities and Infima share one palette.
        'gx-primary':          'var(--ifm-color-primary)',
        'gx-primary-light':    'var(--ifm-color-primary-light)',
        'gx-primary-lighter':  'var(--ifm-color-primary-lighter)',
        'gx-primary-lightest': 'var(--ifm-color-primary-lightest)',
        'gx-primary-dark':     'var(--ifm-color-primary-dark)',
        'gx-surface':          'var(--ifm-background-surface-color)',
        'gx-bg':               'var(--ifm-background-color)',
        'gx-emphasis':         'var(--ifm-color-emphasis-200)',
        'gx-accent-teal':      'var(--gx-accent-teal)',
        'gx-accent-purple':    'var(--gx-accent-purple)',
      },
    },
  },
  plugins: [],
};
