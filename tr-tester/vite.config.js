import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3001
  },
  resolve: {
    alias: {
      '@mqtt_messages': path.resolve(__dirname, 'mqtt_messages')
    }
  }
});
