const GITHUB = "https://github.com/HazyForge/anvil-hotline";
const README = "https://github.com/HazyForge/anvil-hotline#readme";

const SESSION = [
  { type: "meta", text: "anvil-hotline v0.1 — ask once, then proceed" },
  { type: "cmd", text: "$ anvil-hotline ask \\" },
  { type: "cmd", text: '    --question "May I flip flag X in prod?" \\' },
  { type: "cmd", text: '    --context "run=anvil-codex-7f3k9" \\' },
  { type: "cmd", text: "    --yes-no-reactions --timeout 30m" },
  { type: "ask", text: "agent → #ops-human-line" },
  { type: "ask", text: '  "May I flip flag X in prod? ✅ yes / ❌ no"' },
  { type: "wait", text: "waiting for an authorized reply…" },
  { type: "wait", text: "waiting…" },
  { type: "answer", text: '✅ yes — replied by austin (2m 14s)' },
  { type: "out", text: "yes" },
  { type: "meta", text: "exit 0 — proceeding with the reply" },
];

const RULES = [
  {
    n: "01",
    title: "One narrow question",
    detail:
      "Agents ask exactly what they need, with run context. Never a sprawling conversation; never a request for a lecture.",
  },
  {
    n: "02",
    title: "Authorized humans only",
    detail:
      "An allowlist of Discord user IDs — or any member in a private channel. Only people you trust can answer the line.",
  },
  {
    n: "03",
    title: "Typed, or one tap",
    detail:
      "Reply with text, or tap an emoji — ✅ / ❌ / 🔄 or your own choices. The bot pre-applies the reactions on the message.",
  },
  {
    n: "04",
    title: "Wait, then proceed",
    detail:
      "The CLI blocks until an authorized reply arrives, with a configurable timeout, then continues with that answer.",
  },
];

const COMPARE = [
  { mode: "typed", label: "typed reply", value: '"proceed with the rollout"' },
  { mode: "emoji", label: "emoji reply", value: "✅ → yes" },
  { mode: "emoji", label: "emoji reply", value: "❌ → no" },
  { mode: "emoji", label: "custom", value: "🔄 → retry" },
];

const EDGE = [
  { tag: "timeout", text: "No reply before the timeout → anvil-hotline exits non-zero; the agent does NOT default." },
  { tag: "unauthorized", text: "A stranger replies → ignored. Only allowlisted IDs (or any member in private channels) count." },
  { tag: "audit", text: "Every ask, responder, and answer is logged — the loop stays observable." },
];

export default function HomePage() {
  return (
    <main className="askonce">
      {/* Prompt line */}
      <header className="promptbar">
        <div className="promptbar-inner">
          <span className="prompt-caret" aria-hidden="true" />
          <span className="prompt-text">
            anvil-hotline — ask one narrow question to an authorized human, then proceed
          </span>
        </div>
      </header>

      {/* Split-pane hero */}
      <section className="split">
        <div className="split-left">
          <p className="kicker mono">Open source · Go CLI</p>
          <h1 className="split-title">
            Ask once.
            <br />
            <span className="split-accent">Then proceed.</span>
          </h1>
          <p className="split-lead">
            Anvil Hotline is the checkpoint between autonomous work and judgment. When an agent has evidence
            but not certainty, it posts one narrow question to Discord and waits — then continues only with the
            human's answer.
          </p>
          <div className="split-cta">
            <a className="btn btn-primary" href={README}>Read the docs</a>
            <a className="btn btn-ghost" href={GITHUB}>View on GitHub</a>
          </div>
        </div>
        <div className="split-right">
          <div className="terminal">
            <div className="terminal-titlebar mono">
              <span className="term-dot term-dot-r" />
              <span className="term-dot term-dot-y" />
              <span className="term-dot term-dot-g" />
              <span className="term-name">agent:anvil-codex-7f3k9 — live session</span>
            </div>
            <div className="terminal-body">
              <div className="terminal-video">
                <img src="/hero/hero-poster.jpg" alt="" loading="eager" />
                <video autoPlay muted loop playsInline poster="/hero/hero-poster.jpg" tabIndex={-1}>
                  <source src="/hero/hero.mp4" type="video/mp4" />
                </video>
              </div>
              <div className="terminal-lines">
                {SESSION.map((l, i) => (
                  <p key={i} className={`tline t-${l.type}`}>
                    {l.type === "out" ? <span className="tprompt">$ </span> : null}
                    {l.text}
                  </p>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Rules */}
      <section className="section rules container">
        <div className="section-label mono">Protocol — four rules</div>
        <div className="rules-grid">
          {RULES.map((r) => (
            <article key={r.n} className="rule">
              <span className="rule-n mono">{r.n}</span>
              <h3 className="rule-title">{r.title}</h3>
              <p className="rule-detail">{r.detail}</p>
            </article>
          ))}
        </div>
      </section>

      {/* Reply matrix */}
      <section className="section matrix container">
        <div className="section-label mono">Reply protocol — typed or one tap</div>
        <div className="matrix-grid">
          {COMPARE.map((c, i) => (
            <div key={i} className={`matrix-cell matrix-${c.mode}`}>
              <span className="mono matrix-label">{c.label}</span>
              <span className="matrix-value mono">{c.value}</span>
            </div>
          ))}
        </div>
      </section>

      {/* Edge cases */}
      <section className="section edge container">
        <div className="section-label mono">Failure & audit</div>
        <div className="edge-rows">
          {EDGE.map((e) => (
            <div key={e.tag} className="edge-row">
              <span className={`edge-tag mono tag-${e.tag}`}>{e.tag}</span>
              <span className="edge-text">{e.text}</span>
            </div>
          ))}
        </div>
      </section>

      {/* Install */}
      <section className="section install">
        <div className="container install-row">
          <div>
            <h2 className="install-title">Open the line.</h2>
            <p className="install-sub">
              Apache-2.0, swappable transport, one bot token and an allowlist. Your agents get a safe line to
              the people who matter.
            </p>
          </div>
          <pre className="install-cmd"><code><span className="ic-prompt">$</span> go install github.com/hazyforge/anvil-hotline/cmd/anvil-hotline@latest</code></pre>
        </div>
      </section>

      {/* Footer */}
      <footer className="foot container">
        <span className="mono">anvil-hotline v0.1 · {new Date().getFullYear()} Hazy Forge</span>
        <span className="foot-links mono">
          <a href={GITHUB}>github.com/HazyForge/anvil-hotline</a>
          <a href="https://hazyforge.io">hazyforge.io</a>
        </span>
      </footer>
    </main>
  );
}
