# DuraGraph Landing Page

A modern landing page for DuraGraph built with React, Vite, and Carbon Design System.

## Tech Stack

- **React 18** - UI library
- **Vite 7** - Build tool and dev server
- **TypeScript** - Type safety
- **Carbon Design System** - IBM's open-source design system
  - @carbon/react - React components
  - @carbon/styles - Design tokens and styles
  - @carbon/icons-react - Icon library
- **Lucide React** - Additional icons
- **Sass** - CSS preprocessing

## Color Palette

The landing page uses DuraGraph's custom color palette:

- **Deep Graphite**: `#0d1117` (Main background)
- **Dark Slate**: `#161b22` (Secondary background, cards)
- **Light Gray**: `#c9d1d9` (Primary text)
- **Muted Gray**: `#8b949e` (Secondary text)
- **Indigo**: `#6c63ff` (Primary accent - OSS features)
- **Cyan Mint**: `#00c6a7` (Secondary accent - Cloud features)
- **Coral**: `#ff7a59` (Tertiary accent - Completion states)
- **Success Green**: `#3fb950` (Success indicators)
- **Warning Yellow**: `#f1e05a` (Warning states)
- **Error Red**: `#f85149` (Error states)

## Getting Started

### Prerequisites

- Node.js 20.19+ or 22.12+
- npm or yarn

### Installation

```bash
npm install
```

### Development

Start the development server:

```bash
npm run dev
```

The site will be available at `http://localhost:3000`

### Build

Build for production:

```bash
npm run build
```

The built files will be in the `dist` directory.

### Preview

Preview the production build:

```bash
npm run preview
```

## Project Structure

```
website/
├── public/              # Static assets
├── src/
│   ├── components/
│   │   ├── layout/     # Layout components (Header, Footer, Section)
│   │   └── sections/   # Page sections (Hero, WhyDuragraph, etc.)
│   ├── styles/
│   │   ├── theme.scss  # Carbon theme customization
│   │   └── animations.css  # Custom animations
│   ├── App.tsx         # Main app component
│   └── main.tsx        # Entry point
├── index.html
├── vite.config.ts      # Vite configuration
└── package.json
```

## Sections

The landing page includes the following sections:

1. **Hero** - Main tagline with animated workflow diagram
2. **Why DuraGraph** - Three key value propositions (Resilient, Developer-Friendly, Cloud-Scale)
3. **How It Works** - Four-step process (Define, Generate, Run, Monitor)
4. **OSS vs Cloud** - Comparison between open-source and cloud offerings
5. **Use Cases** - Four primary use cases (AI Agents, Data Pipelines, Research Labs, Enterprise Apps)
6. **Community** - Links to GitHub, Discord, and Documentation

## Customization

### Changing Colors

Edit `src/styles/theme.scss` to modify the Carbon theme and custom color variables.

### Adding Sections

1. Create a new component in `src/components/sections/`
2. Import and use it in `src/App.tsx`
3. Wrap it in a `<Section>` component for consistent spacing

### Animations

Custom animations are defined in `src/styles/animations.css`:
- `flow` - Data flowing animation
- `shimmer` - Shimmer effect
- `glow` - Glowing pulse
- `pulse` - Standard pulse
- `processing` - Rotating animation

## Performance

The build is optimized with:
- Code splitting (separate vendor chunks for React and Carbon)
- Minification with esbuild
- Tree shaking
- CSS optimization

## License

Same as DuraGraph project
