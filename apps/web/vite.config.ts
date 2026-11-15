/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    // API_URL points the dev proxy at a server on another port. `ws` forwards the chat WebSocket.
    proxy: { "/api": { target: process.env.API_URL ?? "http://localhost:8080", ws: true } },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
  },
});
