#!/usr/bin/env node

/**
 * Blog Backlink Validator
 *
 * Validates all external and internal links in MDX blog posts.
 * Run with: pnpm run validate-links
 */

import { readdir, readFile } from 'fs/promises';
import { join, resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = fileURLToPath(new URL('.', import.meta.url));
const BLOG_DIR = resolve(__dirname, '../src/content/blog');
const DOCS_DIR = resolve(__dirname, '../src/content/docs');

// Regex patterns for links
const MARKDOWN_LINK_REGEX = /\[([^\]]*)\]\(([^)]+)\)/g;
const MDX_LINK_REGEX = /href=["']([^"']+)["']/g;

// Link validation results
const results = {
  valid: [],
  invalid: [],
  skipped: [],
  internal: [],
};

// Cache for URL checks to avoid duplicate requests
const urlCache = new Map();

/**
 * Check if a URL is accessible
 */
async function checkUrl(url, timeout = 10000) {
  if (urlCache.has(url)) {
    return urlCache.get(url);
  }

  try {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), timeout);

    const response = await fetch(url, {
      method: 'HEAD',
      signal: controller.signal,
      headers: {
        'User-Agent': 'DuraGraph-Link-Validator/1.0',
      },
    });

    clearTimeout(timeoutId);

    // Follow redirects are handled automatically, check final status
    const isValid = response.ok || response.status === 405; // Some servers don't allow HEAD
    urlCache.set(url, isValid);
    return isValid;
  } catch (error) {
    // If HEAD fails, try GET
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), timeout);

      const response = await fetch(url, {
        method: 'GET',
        signal: controller.signal,
        headers: {
          'User-Agent': 'DuraGraph-Link-Validator/1.0',
        },
      });

      clearTimeout(timeoutId);
      const isValid = response.ok;
      urlCache.set(url, isValid);
      return isValid;
    } catch {
      urlCache.set(url, false);
      return false;
    }
  }
}

/**
 * Check if an internal link exists
 */
async function checkInternalLink(link) {
  // Remove leading slash and .html extension
  let path = link.replace(/^\//, '').replace(/\.html$/, '').replace(/\/$/, '');

  // Handle anchor links
  const [basePath, anchor] = path.split('#');
  path = basePath || 'index';

  // Check if it's a blog link
  if (path === 'blog' || path.startsWith('blog/')) {
    const blogPath = path.replace(/^blog\/?/, '');
    if (blogPath === '' || blogPath === 'index') {
      return true; // /blog or /blog/ is always valid
    }
    const blogPossiblePaths = [
      join(BLOG_DIR, `${blogPath}.mdx`),
      join(BLOG_DIR, `${blogPath}.md`),
    ];
    for (const filePath of blogPossiblePaths) {
      try {
        await readFile(filePath);
        return true;
      } catch {
        // File doesn't exist, try next
      }
    }
  }

  // Try to find the file in docs
  const possiblePaths = [
    join(DOCS_DIR, `${path}.mdx`),
    join(DOCS_DIR, `${path}.md`),
    join(DOCS_DIR, path, 'index.mdx'),
    join(DOCS_DIR, path, 'index.md'),
  ];

  for (const filePath of possiblePaths) {
    try {
      await readFile(filePath);
      return true;
    } catch {
      // File doesn't exist, try next
    }
  }

  return false;
}

/**
 * Extract links from MDX content
 */
function extractLinks(content) {
  const links = new Set();

  // Markdown-style links
  let match;
  while ((match = MARKDOWN_LINK_REGEX.exec(content)) !== null) {
    links.add(match[2]);
  }

  // Reset regex
  MARKDOWN_LINK_REGEX.lastIndex = 0;

  // JSX/MDX href attributes
  while ((match = MDX_LINK_REGEX.exec(content)) !== null) {
    links.add(match[1]);
  }

  MDX_LINK_REGEX.lastIndex = 0;

  return Array.from(links);
}

/**
 * Validate a single blog post
 */
async function validatePost(filePath) {
  const content = await readFile(filePath, 'utf-8');
  const links = extractLinks(content);
  const fileName = filePath.split('/').pop();

  console.log(`\n📄 ${fileName} (${links.length} links)`);

  for (const link of links) {
    // Skip mailto, tel, and anchor-only links
    if (link.startsWith('mailto:') || link.startsWith('tel:') || link.startsWith('#')) {
      results.skipped.push({ file: fileName, link, reason: 'Protocol excluded' });
      continue;
    }

    // External links
    if (link.startsWith('http://') || link.startsWith('https://')) {
      process.stdout.write(`  🔗 ${link.substring(0, 60)}${link.length > 60 ? '...' : ''} `);

      const isValid = await checkUrl(link);

      if (isValid) {
        console.log('✅');
        results.valid.push({ file: fileName, link });
      } else {
        console.log('❌');
        results.invalid.push({ file: fileName, link });
      }
    }
    // Internal links
    else if (link.startsWith('/')) {
      const exists = await checkInternalLink(link);

      if (exists) {
        results.internal.push({ file: fileName, link, status: 'valid' });
      } else {
        console.log(`  📁 ${link} ❌ (internal link not found)`);
        results.invalid.push({ file: fileName, link, type: 'internal' });
      }
    }
    // Relative links
    else {
      results.skipped.push({ file: fileName, link, reason: 'Relative link' });
    }
  }
}

/**
 * Get all MDX files in blog directory
 */
async function getBlogFiles() {
  try {
    const files = await readdir(BLOG_DIR);
    return files
      .filter(f => f.endsWith('.mdx') || f.endsWith('.md'))
      .map(f => join(BLOG_DIR, f));
  } catch (error) {
    console.error('Error reading blog directory:', error.message);
    return [];
  }
}

/**
 * Main validation function
 */
async function main() {
  console.log('🔍 DuraGraph Blog Link Validator\n');
  console.log('================================');

  const blogFiles = await getBlogFiles();

  if (blogFiles.length === 0) {
    console.log('No blog posts found in', BLOG_DIR);
    return;
  }

  console.log(`Found ${blogFiles.length} blog post(s)`);

  for (const file of blogFiles) {
    await validatePost(file);
  }

  // Summary
  console.log('\n================================');
  console.log('📊 Validation Summary\n');
  console.log(`✅ Valid links: ${results.valid.length}`);
  console.log(`❌ Invalid links: ${results.invalid.length}`);
  console.log(`📁 Valid internal links: ${results.internal.filter(l => l.status === 'valid').length}`);
  console.log(`⏭️  Skipped: ${results.skipped.length}`);

  if (results.invalid.length > 0) {
    console.log('\n❌ Invalid Links:\n');
    for (const { file, link, type } of results.invalid) {
      console.log(`  ${file}: ${link}${type === 'internal' ? ' (internal)' : ''}`);
    }
    process.exit(1);
  }

  console.log('\n✨ All links are valid!');
}

main().catch(console.error);
