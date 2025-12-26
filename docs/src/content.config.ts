import { defineCollection, z } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { glob } from 'astro/loaders';

// Blog collection schema
const blogCollection = defineCollection({
	loader: glob({ pattern: '**/*.{md,mdx}', base: './src/content/blog' }),
	schema: z.object({
		title: z.string(),
		date: z.coerce.date(),
		excerpt: z.string(),
		authors: z.array(z.string()).default(['duragraph']),
		tags: z.array(z.string()).default([]),
		image: z.string().optional(),
		draft: z.boolean().default(false),
	}),
});

export const collections = {
	docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
	blog: blogCollection,
};
