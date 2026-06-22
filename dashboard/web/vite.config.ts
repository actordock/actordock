import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api/platform": {
        target: process.env.VITE_PLATFORM_URL ?? "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/platform/, ""),
        configure: (proxy) => {
          proxy.on("proxyReq", (proxyReq) => {
            proxyReq.setHeader(
              "X-API-KEY",
              process.env.VITE_API_KEY ?? "dev",
            );
          });
        },
      },
      "/api/router": {
        target: process.env.VITE_ROUTER_URL ?? "http://localhost:8081",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/router/, ""),
        configure: (proxy) => {
          proxy.on("proxyRes", (proxyRes) => {
            proxyRes.headers["x-accel-buffering"] = "no";
          });
        },
      },
    },
  },
});
