# Zenith Frontend Showcase

A polished React frontend that visually demonstrates all Zenith backend capabilities through interactive visualizations, real-time performance monitoring, and live demos.

## Features

- **Interactive Permission Graph** - Visualize permission relationships as an interactive force-directed graph
- **Permission Checker** - Check permissions with expansion path visualization
- **Batch Checker** - Demonstrate batch operation efficiency
- **Tuple Manager** - CRUD interface for managing permission tuples
- **Performance Dashboard** - Real-time metrics and monitoring
- **Temporal Audit Trail** - View permissions at any point in time

## Tech Stack

- React 18 + TypeScript + Vite
- TanStack Query for data fetching
- Zustand for state management
- Tailwind CSS for styling
- Visx for graph visualization
- Recharts for performance charts

## Getting Started

### Prerequisites

- Node.js 18+ and npm/yarn/pnpm
- Zenith backend running (default: http://localhost:50051)

### Installation

```bash
cd frontend
npm install
```

### Development

```bash
npm run dev
```

The app will be available at http://localhost:3000

### Build

```bash
npm run build
```

### Preview Production Build

```bash
npm run preview
```

## Environment Variables

Create a `.env` file:

```
VITE_API_URL=http://localhost:50051
VITE_METRICS_URL=http://localhost:9090/metrics
```

## Deployment

### Vercel

1. Push code to GitHub
2. Import project in Vercel
3. Set environment variables
4. Deploy

The frontend will automatically deploy on every push to main.

## Project Structure

```
frontend/
├── src/
│   ├── components/     # React components
│   ├── hooks/          # Custom React hooks
│   ├── services/       # API services
│   ├── types/          # TypeScript types
│   ├── stores/         # Zustand stores
│   └── styles/         # Global styles
├── public/             # Static assets
└── package.json
```

## API Integration

The frontend expects a REST/JSON gateway wrapping the gRPC backend. If the backend doesn't have a REST gateway yet, the API service includes fallback mock responses for development.

## License

Same as Zenith project

