import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        canvas: 'var(--bg-primary)',
        surface: 'var(--bg-surface)',
        subtle: 'var(--bg-secondary)',
        ink: 'var(--text-primary)',
        'ink-secondary': 'var(--text-secondary)',
        'ink-tertiary': 'var(--text-tertiary)',
        accent: 'var(--accent)',
        'on-accent': 'var(--on-accent)',
        overlay: 'var(--overlay)',
        danger: 'var(--danger)',
        success: 'var(--success)',
        hairline: 'var(--separator)',
        glass: 'var(--glass-bg)'
      },
      fontSize: {
        'large-title': ['32px', { lineHeight: '1.2', fontWeight: '700', letterSpacing: '-0.01em' }],
        'title-1': ['26px', { lineHeight: '1.24', fontWeight: '700', letterSpacing: '-0.01em' }],
        'title-2': ['21px', { lineHeight: '1.3', fontWeight: '600', letterSpacing: '-0.01em' }],
        headline: ['17px', { lineHeight: '1.35', fontWeight: '600' }],
        body: ['17px', { lineHeight: '1.65', fontWeight: '400' }],
        callout: ['16px', { lineHeight: '1.5', fontWeight: '400' }],
        subheadline: ['15px', { lineHeight: '1.45', fontWeight: '400' }],
        caption: ['13px', { lineHeight: '1.4', fontWeight: '400' }],
        'caption-medium': ['13px', { lineHeight: '1.4', fontWeight: '500' }]
      },
      borderRadius: {
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        full: '9999px'
      },
      maxWidth: {
        app: '720px'
      },
      boxShadow: {
        control: 'var(--shadow-control)',
        sheet: 'var(--shadow-sheet)'
      },
      backdropBlur: {
        glass: 'var(--glass-blur)'
      },
      transitionTimingFunction: {
        ios: 'cubic-bezier(.2,.8,.2,1)'
      },
      transitionDuration: {
        fast: '150ms',
        normal: '220ms',
        slow: '300ms'
      },
      keyframes: {
        'fade-in': {
          from: { opacity: '0' },
          to: { opacity: '1' }
        },
        'slide-up': {
          from: { transform: 'translateY(24px)', opacity: '0' },
          to: { transform: 'translateY(0)', opacity: '1' }
        }
      },
      animation: {
        'slide-up': 'slide-up 300ms cubic-bezier(.2,.8,.2,1) both'
      }
    }
  },
  plugins: []
} satisfies Config
