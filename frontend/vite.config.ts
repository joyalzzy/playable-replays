import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const apiTarget = process.env.VITE_API_TARGET ?? "http://127.0.0.1:8080";
const proxy = {
  "/api": apiTarget,
  "/healthz": apiTarget
};

export default defineConfig({
  plugins: [react()],
  server: { proxy },
  preview: { proxy }
});
