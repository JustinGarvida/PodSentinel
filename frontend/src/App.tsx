import { MonitorCard } from './components/MonitorCard'
import { sampleTraces } from './data/sampleTraces'
import { Dashboard } from './pages/Dashboard'

const pipeline = [
  {
    n: '01',
    title: 'Poll',
    body: 'The Go agent polls the Kubernetes Metrics API every 15–30s for pod-level CPU and memory.',
  },
  {
    n: '02',
    title: 'Record',
    body: 'Every sample writes straight to Postgres/TimescaleDB — independent of the queue, so recording survives an outage.',
  },
  {
    n: '03',
    title: 'Publish',
    body: "Each pod's sample becomes its own RabbitMQ message. One malformed payload only affects one pod.",
  },
  {
    n: '04',
    title: 'Detect',
    body: "Python tracks each pod's rolling baseline and flags z-score/EWMA deviation — no training data required.",
  },
  {
    n: '05',
    title: 'Alert',
    body: 'Anomaly events flow back through RabbitMQ. Go persists them and the dashboard lights up.',
  },
]

const stack = [
  {
    name: 'Go',
    role: 'ingestion + API',
    why: 'client-go for the Kubernetes API, fast enough to run in-cluster. One binary handles polling, writes, pub/sub, and the REST API.',
  },
  {
    name: 'Python',
    role: 'anomaly detection',
    why: 'Its statistics/ML ecosystem, and a deliberate experiment with Python for the detection side.',
  },
  {
    name: 'RabbitMQ',
    role: 'event bus',
    why: 'Per-pod messages with dead-letter queues — an experiment in queue-driven architecture, not a scale requirement.',
  },
  {
    name: 'Postgres + TimescaleDB',
    role: 'storage',
    why: 'Hypertables for efficient time-series metrics and anomaly history.',
  },
  {
    name: 'React + TypeScript',
    role: 'dashboard',
    why: "Reads straight from the Go agent's REST API.",
  },
]

function App() {
  if (window.location.pathname === '/dashboard') {
    return <Dashboard />
  }

  return (
    <div
      className="min-h-screen bg-scope-bg text-ink"
      style={{
        backgroundImage:
          'linear-gradient(var(--color-grid) 1px, transparent 1px), linear-gradient(90deg, var(--color-grid) 1px, transparent 1px)',
        backgroundSize: '32px 32px',
      }}
    >
      <header className="mx-auto flex max-w-6xl items-center justify-between px-6 py-6">
        <div className="font-mono text-sm tracking-widest uppercase">
          <span className="text-ink">pod</span>
          <span className="text-trace">sentinel</span>
        </div>
        <div className="flex items-center gap-4">
          <a
            href="/dashboard"
            className="font-mono text-[11px] tracking-widest text-ink-dim uppercase hover:text-ink"
          >
            pod list →
          </a>
          <div className="rounded-full border border-grid px-3 py-1 font-mono text-[11px] tracking-widest text-ink-dim uppercase">
            design → build · v0.1
          </div>
        </div>
      </header>

      <main>
        <section className="mx-auto max-w-6xl px-6 pt-12 pb-20 md:pt-20 md:pb-28">
          <div className="grid gap-12 md:grid-cols-[1.1fr_0.9fr] md:items-center">
            <div>
              <p className="font-mono text-xs tracking-[0.25em] text-trace uppercase">
                kubernetes · pod-level anomaly detection
              </p>
              <h1 className="mt-5 font-display text-4xl leading-[1.05] tracking-tight text-ink sm:text-5xl lg:text-6xl">
                every pod has a baseline.
                <br />
                podsentinel watches for the break.
              </h1>
              <p className="mt-6 max-w-md text-base leading-relaxed text-ink-dim">
                PodSentinel polls CPU and memory for every pod in your cluster,
                learns each one's normal rhythm, and flags the moment a pod
                strays from it — before it becomes an incident.
              </p>
              <div className="mt-8 flex flex-wrap items-center gap-4">
                <span className="inline-flex items-center gap-2 rounded-full border border-grid px-4 py-2 font-mono text-xs text-ink-dim">
                  <span className="h-1.5 w-1.5 rounded-full bg-stable" />
                  statistical baseline, not ML — explainable by design
                </span>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {sampleTraces.map((trace) => (
                <MonitorCard key={trace.pod} {...trace} />
              ))}
            </div>
          </div>
        </section>

        <section className="border-t border-grid">
          <div className="mx-auto max-w-6xl px-6 py-20">
            <h2 className="font-display text-xs tracking-[0.25em] text-ink-dim uppercase">
              how the signal travels
            </h2>
            <div className="mt-10 grid gap-8 md:grid-cols-5 md:gap-6">
              {pipeline.map((step) => (
                <div
                  key={step.n}
                  className="border-l border-grid pl-5 md:border-t md:border-l-0 md:pt-5 md:pl-0"
                >
                  <span className="font-mono text-sm text-trace">{step.n}</span>
                  <h3 className="mt-2 font-mono text-sm tracking-wide text-ink uppercase">
                    {step.title}
                  </h3>
                  <p className="mt-2 text-sm leading-relaxed text-ink-dim">
                    {step.body}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="border-t border-grid bg-panel/40">
          <div className="mx-auto max-w-6xl px-6 py-20">
            <h2 className="font-display text-xs tracking-[0.25em] text-ink-dim uppercase">
              what it's built from
            </h2>
            <div className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
              {stack.map((item) => (
                <div
                  key={item.name}
                  className="rounded-lg border border-grid bg-panel p-4"
                >
                  <p className="font-mono text-sm text-trace">{item.name}</p>
                  <p className="mt-1 font-mono text-[11px] tracking-widest text-ink-faint uppercase">
                    {item.role}
                  </p>
                  <p className="mt-3 text-xs leading-relaxed text-ink-dim">
                    {item.why}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="border-t border-grid">
          <div className="mx-auto max-w-6xl px-6 py-20">
            <div className="mx-auto max-w-xl rounded-lg border border-grid bg-panel p-6">
              <div className="flex items-center justify-between">
                <p className="font-mono text-[11px] tracking-widest text-ink-faint uppercase">
                  project vitals
                </p>
                <span className="h-2 w-2 animate-pulse rounded-full bg-trace" />
              </div>
              <dl className="mt-4 space-y-2 font-mono text-sm">
                <div className="flex justify-between border-b border-grid pb-2">
                  <dt className="text-ink-dim">stage</dt>
                  <dd className="text-ink">design → build</dd>
                </div>
                <div className="flex justify-between border-b border-grid pb-2">
                  <dt className="text-ink-dim">scope</dt>
                  <dd className="text-ink">single cluster</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-ink-dim">status</dt>
                  <dd className="text-ink">
                    in development
                    <span className="ml-1 animate-pulse text-trace">_</span>
                  </dd>
                </div>
              </dl>
            </div>
          </div>
        </section>
      </main>

      <footer className="border-t border-grid">
        <div className="mx-auto max-w-6xl px-6 py-8">
          <p className="font-mono text-xs text-ink-faint">
            podsentinel — an experimental, single-cluster project. go · python
            · rabbitmq · postgres · react.
          </p>
        </div>
      </footer>
    </div>
  )
}

export default App
