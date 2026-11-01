import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
export default defineConfig({plugins:[tailwindcss()],build:{outDir:'internal/ui/static/dist',emptyOutDir:true,rollupOptions:{input:'web/src/app.js',output:{entryFileNames:'app.js',assetFileNames:'[name][extname]'}}}})
