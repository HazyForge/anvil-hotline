const GITHUB = "https://github.com/HazyForge/anvil-hotline";
const README = "https://github.com/HazyForge/anvil-hotline#readme";

const FLAGS = [
  { code: "HOIST", color: "red", title: "One narrow question", text: "Agents hoist exactly one question, with run context, to the authorized line. Never a conversation." },
  { code: "HOLD", color: "yellow", title: "Wait for the reply", text: "The halyard stays up until an authorized human answers — typed, or a signal flag (emoji) on the message." },
  { code: "REPLY", color: "navy", title: "Authorized humans only", text: "An allowlist of Discord user IDs — or any member in a private channel. Strangers cannot answer." },
  { code: "LOWER", color: "canvas", title: "Proceed on the answer", text: "The agent continues only with the reply. Timeout means non-zero exit — never a silent default." },
];

const SIGNALS = [
  { mode: "typed", label: "typed reply", value: "proceed with the rollout" },
  { mode: "pennant", label: "✅ / ❌ / 🔄", value: "yes · no · retry" },
  { mode: "pennant", label: "custom signals", value: "✅=proceed, ❌=abort, 🔄=retry" },
  { mode: "audit", label: "every exchange logged", value: "ask · responder · answer" },
];

export default function HomePage() {
  return (
    <main className="semaphore">
      {/* Yardarm header */}
      <header className="yardarm">
        <div className="yardarm-inner">
          <span className="yardarm-brand">Anvil Hotline</span>
          <span className="yardarm-rule" aria-hidden="true" />
          <span className="yardarm-meta mono">Semaphore Station · ask one, then proceed</span>
          <span className="yardarm-rule" aria-hidden="true" />
          <span className="yardarm-links mono"><a href={GITHUB}>GitHub</a> <a href="https://hazyforge.io">Hazy Forge</a></span>
        </div>
      </header>

      {/* HOIST hero */}
      <section className="band band-hoist">
        <div className="band-inner">
          <p className="kicker mono">Open source · Go CLI</p>
          <h1 className="band-title">
            Hoist a question.<br />
            <span className="band-accent">Hold for a human.</span>
          </h1>
          <p className="band-lead">
            Anvil Hotline is the signal mast between autonomous agents and the people who answer to no one.
            When an agent has evidence but not certainty, it hoists one narrow question to Discord and waits —
            then proceeds only on the reply.
          </p>
          <div className="band-cta">
            <a className="btn btn-primary" href={README}>Read the docs</a>
            <a className="btn btn-ghost" href={GITHUB}>View on GitHub</a>
          </div>
        </div>
        <figure className="hoist">
          <div className="hoist-media">
            <img src="/hero/hero-poster.jpg" alt="" loading="eager" />
            <video autoPlay muted loop playsInline poster="/hero/hero-poster.jpg" tabIndex={-1}>
              <source src="/hero/hero.mp4" type="video/mp4" />
            </video>
          </div>
          <figcaption className="mono">fig. 01 — the hoist, photographed from the yard</figcaption>
        </figure>
      </section>

      {/* Alphabet grid */}
      <section className="section alphabet">
        <div className="section-label mono">The code — four signals</div>
        <div className="alphabet-grid">
          {FLAGS.map((f) => (
            <article key={f.code} className={`flag flag-${f.color}`}>
              <span className="flag-code mono">{f.code}</span>
              <h3 className="flag-title">{f.title}</h3>
              <p className="flag-text">{f.text}</p>
            </article>
          ))}
        </div>
      </section>

      {/* Reply signals */}
      <section className="section signals">
        <div className="section-label mono">Reply signals — typed or a pennant</div>
        <div className="signal-grid">
          {SIGNALS.map((s, i) => (
            <div key={i} className={`signal signal-${s.mode}`}>
              <span className="signal-label mono">{s.label}</span>
              <span className="signal-value mono">{s.value}</span>
            </div>
          ))}
        </div>
      </section>

      {/* Authorization manifest */}
      <section className="section manifest">
        <div className="section-label mono">Manifest — who may answer</div>
        <div className="manifest-rows">
          <div className="manifest-row"><span className="m-tag mono">allowlist</span><span className="m-text">ANVIL_HOTLINE_ALLOWED_USER_IDS=… — only those Discord users may answer.</span></div>
          <div className="manifest-row"><span className="m-tag mono">private</span><span className="m-text">ANVIL_HOTLINE_ALLOW_ANY_USER=true — any non-bot member, private channels only.</span></div>
          <div className="manifest-row"><span className="m-tag mono">timeout</span><span className="m-text">ANVIL_HOTLINE_TIMEOUT=30m — no reply means non-zero exit; the agent never defaults.</span></div>
          <div className="manifest-row"><span className="m-tag mono">audit</span><span className="m-text">Every ask, responder, and answer is logged — the loop stays observable.</span></div>
        </div>
      </section>

      {/* Rigging / install */}
      <section className="rigging">
        <div className="rigging-inner">
          <div>
            <h2 className="rigging-title">Rig the line.</h2>
            <p className="rigging-sub">Apache-2.0, swappable transport, one bot token and an allowlist.</p>
          </div>
          <div className="rigging-cmd mono"><span>$</span> go install github.com/hazyforge/anvil-hotline/cmd/anvil-hotline@latest</div>
        </div>
      </section>

      {/* Footer */}
      <footer className="foot">
        <span className="mono">anvil-hotline v0.1 · {new Date().getFullYear()} Hazy Forge</span>
        <span className="foot-links mono"><a href={GITHUB}>github.com/HazyForge/anvil-hotline</a> <a href="https://hazyforge.io">hazyforge.io</a></span>
      </footer>
    </main>
  );
}
