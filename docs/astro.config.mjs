// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import react from '@astrojs/react';
import tailwindcss from '@tailwindcss/vite';
import mermaid from 'astro-mermaid';

// https://astro.build/config
export default defineConfig({
  vite: {
    plugins: [tailwindcss()],
  },
  integrations: [
    react(),
    mermaid({
      // Auto-switch theme based on light/dark mode
      mermaidConfig: {
        theme: 'base',
        look: 'neo',
        themeVariables: {
          // Coral brand colors
          primaryColor: '#fff7ed',
          primaryTextColor: '#9a3412',
          primaryBorderColor: '#f97316',
          lineColor: '#f97316',
          secondaryColor: '#fef3c7',
          tertiaryColor: '#fafafa',
        },
      },
    }),
    starlight({
      title: 'DuraGraph',
      logo: {
        light: './src/assets/logo-light.svg',
        dark: './src/assets/logo-dark.svg',
        replacesTitle: false,
      },
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/duragraph/duragraph' },
      ],
      customCss: ['./src/styles/custom.css'],
      // Disable Starlight's default index page so we can use our landing page
      disable404Route: false,
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Introduction', slug: 'docs/introduction' },
            { label: 'Quick Start', slug: 'docs/getting-started' },
          ],
        },
        {
          label: 'User Guide',
          items: [
            {
              label: 'Installation',
              autogenerate: { directory: 'docs/user-guide/installation' },
            },
            {
              label: 'Concepts',
              autogenerate: { directory: 'docs/user-guide/concepts' },
            },
            {
              label: 'Tutorials',
              autogenerate: { directory: 'docs/user-guide/tutorials' },
            },
            {
              label: 'SDKs',
              autogenerate: { directory: 'docs/user-guide/sdks' },
            },
          ],
        },
        {
          label: 'API Reference',
          autogenerate: { directory: 'docs/api-reference' },
        },
        {
          label: 'Architecture',
          autogenerate: { directory: 'docs/architecture' },
        },
        {
          label: 'Operations',
          autogenerate: { directory: 'docs/ops' },
        },
        {
          label: 'Development',
          autogenerate: { directory: 'docs/development' },
        },
        {
          label: 'Project',
          autogenerate: { directory: 'docs/project' },
        },
      ],
    }),
  ],
});
