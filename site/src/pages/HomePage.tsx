import { useEffect, useState } from "react";

const GITHUB = "https://github.com/HazyForge/anvil-hotline";
const README = "https://github.com/HazyForge/anvil-hotline#readme";

const ACTS = [
  { id: "ask", label: "ASK", title: "One narrow question", 
    panels: [
      { k: "one sentence", v: "Agents ask exactly what they need — with run context, never a conversation." },
      { k: "allowed verbs", v: "proceed · abort · retry — the verbs you define, nothing loose." },
      { k: "forbidden scope", v: "No browsing, no guessing, no unsafe defaults when evidence isn't enough." },
    ] },
  { id: "route", label: "ROUTE", title: "Authorization gate",
    panels: [
      { k: "allowlist", v: "ANVIL_HOTLINE_ALLOWED_USER_IDS — only authorized Discord users may answer." },
      { k: "private room", v: "ANVIL_HOTLINE_ALLOW_ANY_USER=true — any member, private channels only." },
      { k: "scope tokens", v: "The question carries context: run id, target, and what the agent already knows." },
    ] },
  { id: "wait", label: "WAIT", title: "The wait",
    panels: [
      { k: "queued", v: "The question is posted to the channel. The line is open." },
      { k: "ringing", v: "An authorized human is online. The handset glows." },
      { k: "typing", v: "A reply is forming — typed, or a signal flag on the message." },
    ] },
  { id: "seal", label: "SEAL", title: "Seal & audit",
    panels: [
      { k: "canonical", v: "Stdout carries only the mapped reply — yes, no, retry. Nothing else." },
      { k: "audit", v: "Every ask, responder, and answer is logged. The loop stays observable." },
      { k: "non-zero", v: "Timeout means exit non-zero. The agent never defaults." },
    ] },
];

const POLICY = [
  { k: "retention", v: "Ask/answer pairs are log lines — append-only, no secrets in stdout, no tokens in messages." },
  { k: "who can be paged", v: "The allowlist is the only roster. Private channels may permit any non-bot member." },
  { k: "timeout SLA", v: "ANVIL_HOTLINE_TIMEOUT=30m default. On timeout the run fails closed, loudly." },
  { k: "transport", v: "Discord is the first transport. The ask-and-wait contract is transport-agnostic." },
];

function useActiveAct() {
  const [active, setActive] = useState(0);
  useEffect(() => {
    const els = ACTS.map((a) => document.getElementById(`act-${a.id}`));
    const onScroll = () => {
      const probe = window.innerHeight * 0.35;
      let best = 0;
      let bestDist = Infinity;
      els.forEach((el, i) => {
        if (!el) return;
        const r = el.getBoundingClientRect();
        if (r.top <= probe && r.bottom >= probe) {
          setActive(i);
          return;
        }
        const d = Math.min(Math.abs(r.top - probe), Math.abs(r.bottom - probe));
        if (d < bestDist) {
          bestDist = d;
          best = i;
        }
      });
      // containment takes priority; otherwise nearest
      const contains = els.findIndex((el) => {
        if (!el) return false;
        const r = el.getBoundingClientRect();
        return r.top <= probe && r.bottom >= probe;
      });
      setActive(contains >= 0 ? contains : best);
    };
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);
  return active;
}

export default function HomePage() {
  const activeAct = useActiveAct();
  return (
    <main>
      {/* Cinematic hero */}
      <section className="hero">
        <div className="hero-media" aria-hidden="true">
          <img className="hero-poster" src="/hero/hero-poster.jpg" alt="" fetchPriority="high" decoding="async" />
          <video className="hero-video" autoPlay muted loop playsInline preload="auto" poster="/hero/hero-poster.jpg" tabIndex={-1}>
            <source src="/hero/hero.mp4" type="video/mp4" />
          </video>
        </div>
        <div className="hero-scrim" aria-hidden="true" />
        <div className="hero-inner">
          <div className="hero-copy">
            <div className="eyebrow">Open source · Go CLI</div>
            <h1 className="display hero-title">
              <span>Anvil</span>
              <span className="hero-accent">Hotline</span>
            </h1>
            <p className="hero-tagline">When agents need a <em>human</em></p>
            <p className="hero-lead">
              Agents hoist one narrow question to an authorized human on Discord, wait for
              the reply — typed or an emoji — and proceed only on that answer.
            </p>
            <div className="hero-cta">
              <a className="btn btn-primary" href={README}>Read the docs</a>
              <a className="btn btn-ghost" href={GITHUB}>View on GitHub</a>
            </div>
          </div>
        </div>
      </section>

      {/* Act rail intro */}
      <section className="actrail">
        <div className="actrail-inner">
          <p className="actrail-kicker mono">The night line — four acts</p>
          <div className="actrail-acts">
            {ACTS.map((a, i) => (
              <span key={a.id} className={"actrail-word mono" + (i === activeAct ? " is-active" : "")}>{a.label}</span>
            ))}
          </div>
        </div>
      </section>

      {/* Acts */}
      <section className="acts">
        {ACTS.map((a, ai) => (
          <article key={a.id} id={`act-${a.id}`} className={"act act-" + a.id}>
            <div className="act-head">
              <div className="mono act-num">ACT {ai + 1} / {ACTS.length}</div>
              <h2 className="display act-title">{a.title}</h2>
            </div>
            <div className="act-stage-wrap">
              <div className="act-stages">
                {a.panels.map((p, pi) => (
                  <div key={p.k} className="stage">
                    <span className="mono stage-k">{p.k}</span>
                    <span className="stage-v">{p.v}</span>
                    <span className="mono stage-idx">{String(pi + 1).padStart(2, "0")}</span>
                  </div>
                ))}
              </div>
              <span className="mono stage-hint">more ▸</span>
            </div>
          </article>
        ))}
      </section>

      {/* Policy appendix */}
      <section className="policy">
        <div className="policy-head mono">Policy appendix — engineering spec</div>
        <div className="policy-grid">
          {POLICY.map((p) => (
            <div key={p.k} className="policy-item">
              <span className="mono policy-k">{p.k}</span>
              <span className="policy-v">{p.v}</span>
            </div>
          ))}
        </div>
      </section>

      {/* Dispatch CTA */}
      <section className="dispatch-cta">
        <div className="dispatch-inner">
          <p className="eyebrow">Open source · Apache-2.0</p>
          <h2 className="display dispatch-title">
            Open the <span className="dispatch-accent">channel</span>
          </h2>
          <p className="dispatch-sub">
            One bot token, one channel, an allowlist. Your agents get a safe line to the
            people who matter.
          </p>
          <div className="dispatch-actions">
            <a className="btn btn-primary" href={README}>Read the docs</a>
            <a className="btn btn-ghost" href={GITHUB}>View on GitHub</a>
          </div>
        </div>
      </section>

      <footer className="site-footer">
        <div className="site-footer-inner">
          <div className="mono footer-meta">
            <span>© {new Date().getFullYear()} Hazy Forge</span>
            <span>anvil-hotline.hazyforge.io</span>
          </div>
          <div className="footer-links mono">
            <a href={GITHUB}>GitHub</a>
            <a href="https://hazyforge.io">Hazy Forge</a>
          </div>
        </div>
      </footer>

      {/* Fixed dispatch dock */}
      <div className="dock" aria-hidden="true">
        <div className="dock-ticket mono">TKT-7F3K9 · anvil-codex</div>
        <div className="dock-pips">
          {ACTS.map((a, i) => (
            <span key={a.id} className={"dock-pip mono" + (i === activeAct ? " is-active" : "")}>{a.label}</span>
          ))}
        </div>
        <div className="dock-human">
          <span className="dock-dot" />
          <span className="mono dock-latency">human online · 2m 14s</span>
        </div>
      </div>
    </main>
  );
}
