import HeroCinematic from "../components/HeroCinematic";

const GITHUB = "https://github.com/HazyForge/anvil-hotline";
const README = "https://github.com/HazyForge/anvil-hotline#readme";

const WHY = [
  {
    label: "One narrow question",
    detail:
      "Agents ask exactly what they need — with run context, not a sprawling conversation. The human never has to read a novel.",
  },
  {
    label: "Authorized answers",
    detail:
      "An allowlist of Discord user IDs (or any member, in private channels). Only people you trust can answer the line.",
  },
  {
    label: "Typed or one click",
    detail:
      "Reply with text, or tap an emoji — ✅ / ❌ / 🔄, or your own choices. The bot pre-applies the reactions on the message.",
  },
  {
    label: "Ask and wait",
    detail:
      "The CLI blocks until an authorized reply arrives — with a configurable timeout — then continues with that answer.",
  },
];

const PIPELINE = [
  {
    name: "Ask",
    detail:
      "anvil-hotline ask posts one narrow question with context: what the agent knows and what it needs the human to decide.",
  },
  {
    name: "Wait",
    detail:
      "The line stays open until an authorized user answers — typed text or a pre-applied emoji reaction — or the timeout expires.",
  },
  {
    name: "Answer",
    detail:
      "Stdout carries only the mapped reply — yes, no, proceed, retry — never secrets, never the surrounding noise.",
  },
  {
    name: "Continue",
    detail:
      "The agent proceeds with the human's decision. No guessing, no unsafe defaults when the evidence isn't enough.",
  },
];

export default function HomePage() {
  return (
    <main>
      <section className="hero">
        <HeroCinematic />
        <div className="hero-scrim" aria-hidden="true" />
        <div className="container hero-inner">
          <div className="hero-copy">
            <div className="eyebrow">Open source · Go CLI</div>
            <h1 className="display hero-title">
              <span>Anvil</span>
              <span className="hero-title-accent">Hotline</span>
            </h1>
            <p className="hero-tagline">
              When agents need a <em>human</em>
            </p>
            <p className="hero-lead">
              Anvil Hotline gives your agents one narrow question to ask when
              they can't safely choose a next action. It posts to Discord,
              waits for an authorized reply — typed or a reaction — and
              continues only with that answer.
            </p>
            <div className="hero-chips">
              <span className="chip">
                <span className="chip-dot" />
                v0.1 live
              </span>
              <span className="chip">Discord native</span>
              <span className="chip">Emoji answers</span>
              <span className="chip">Ask &amp; wait</span>
            </div>
            <div className="hero-cta">
              <a className="btn btn-primary" href={README}>
                Read the docs
              </a>
              <a className="btn btn-ghost" href={GITHUB}>
                View on GitHub
              </a>
            </div>
          </div>
        </div>
        <div className="hero-ticker" aria-hidden="true">
          <div className="container hero-ticker-inner">
            <span className="mono hero-ticker-label">Line</span>
            <div className="hero-ticker-runs">
              <span className="ticker-run">
                <span className="ticker-dot live" />
                <span className="ticker-name mono">anvil-codex-7f3k9</span>
                <span className="ticker-phase">Awaiting reply</span>
              </span>
              <span className="ticker-run">
                <span className="ticker-dot" />
                <span className="ticker-name mono">release gate</span>
                <span className="ticker-phase">Answered ✅</span>
              </span>
              <span className="ticker-run">
                <span className="ticker-dot" />
                <span className="ticker-name mono">rollback check</span>
                <span className="ticker-phase">Answered ❌</span>
              </span>
              <span className="ticker-run">
                <span className="ticker-dot" />
                <span className="ticker-name mono">credential rotate</span>
                <span className="ticker-phase">Answered ✅</span>
              </span>
            </div>
          </div>
        </div>
      </section>

      <section id="why" className="section">
        <div className="container">
          <div className="section-head">
            <div className="eyebrow">Why we built it</div>
            <h2 className="display section-title">
              Agents shouldn't
              <span className="soft"> guess</span>
            </h2>
            <p className="section-lead">
              Autonomous work is only safe when the loop knows where the human
              is. Anvil Hotline is the narrow checkpoint: when an agent has
              evidence but not certainty, it asks one question and waits —
              instead of improvising a dangerous default.
            </p>
          </div>
          <div className="highlight-grid">
            {WHY.map((item, index) => (
              <article key={item.label} className="panel highlight-card">
                <div className="mono highlight-index">
                  {String(index + 1).padStart(2, "0")}
                </div>
                <h3 className="display highlight-title">{item.label}</h3>
                <p>{item.detail}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section id="pipeline" className="section section-alt">
        <div className="container">
          <div className="section-head">
            <div className="eyebrow">Pipeline</div>
            <h2 className="display section-title">
              Ask. Wait.
              <span className="soft"> Proceed.</span>
            </h2>
            <p className="section-lead">
              One CLI, one contract: stdout carries only the human's decision.
              The transport is swappable — Discord is the first implementation.
            </p>
          </div>
          <div className="composition-grid">
            {PIPELINE.map((item) => (
              <article key={item.name} className="panel composition-card">
                <h3 className="display composition-title">{item.name}</h3>
                <p>{item.detail}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section id="docs" className="section">
        <div className="container panel cta-panel">
          <div>
            <div className="eyebrow">Open source</div>
            <h2 className="display section-title">
              Install, ask,
              <span className="soft"> proceed</span>
            </h2>
            <p className="section-lead">
              Apache-2.0 and swappable: one Discord bot token, one channel, an
              allowlist — and your agents get a safe line to the people who
              matter.
            </p>
          </div>
          <div className="cta-actions">
            <a className="btn btn-primary" href={README}>
              Read the docs
            </a>
            <a className="btn btn-ghost" href={GITHUB}>
              View on GitHub
            </a>
            <a className="btn btn-ghost" href="https://github.com/HazyForge/anvil-hotline/releases">
              Releases
            </a>
          </div>
        </div>
      </section>
    </main>
  );
}
