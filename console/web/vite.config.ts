import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist/app", emptyOutDir: true, assetsInlineLimit: 0 },
  server: { port: 5173, proxy: { "/api": "http://127.0.0.1:7465", "/oauth": "http://127.0.0.1:7465" } },
});
