import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'
import './index.css'

const qc = new QueryClient({
  defaultOptions: {
    queries: {
      // Admin tooling — fresh on focus is fine, but don't refetch on every
      // mount inside a tab to keep the UI snappy.
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 10_000,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={qc}>
      <App />
    </QueryClientProvider>
  </React.StrictMode>,
)
