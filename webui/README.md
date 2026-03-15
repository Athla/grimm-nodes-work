# graph-info / webui

React + TypeScript frontend for graph-info. Built with Vite and React Flow.

## Development

```bash
npm install
npm run dev      # http://localhost:5173
npm run build    # Production build to dist/
npm run lint     # ESLint
```

The dev server proxies API requests to the backend at `localhost:8080`. If the backend is unavailable, the UI falls back to built-in mock data.
