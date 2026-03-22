import { defineConfig } from 'tsup'

export default defineConfig({
  entry: {
    'index': 'src/index.ts',
    'integrations/vercel-ai': 'src/integrations/vercel-ai.ts',
    'integrations/openai': 'src/integrations/openai.ts',
    'integrations/langchain': 'src/integrations/langchain.ts',
  },
  format: ['cjs', 'esm'],
  dts: true,
  splitting: false,
  sourcemap: true,
  clean: true,
  external: [
    '@opentelemetry/api',
    'ai',
    'openai',
    '@langchain/core',
  ],
})
