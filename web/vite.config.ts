import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// Proxy API + websocket traffic to the Go server so the dev client needs no
// CORS setup: `npm run dev` here, `go run ./cmd/server` in server/.
// The server address comes from SERVER_URL in web/.env (see .env.example).
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const serverURL = env.SERVER_URL || 'http://localhost:8080'

  return {
    plugins: [react()],
    server: {
      proxy: {
        '/queue': { target: serverURL.replace(/^http/, 'ws'), ws: true },
        // /api/* is stripped to /* so the SPA's /room/:id page route doesn't
        // collide with the server's /room/:id endpoint on hard refresh.
        '/api': {
          target: serverURL,
          changeOrigin: true,
          // ws: true lets the room session socket upgrade through this
          // entry (its URL is /api/room/:id/session — see roomSessionURL).
          ws: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
        },
      },
    },
  }
})
