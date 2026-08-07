# Anvil Hotline site

React + Vite + nginx marketing site for [Anvil Hotline](https://github.com/HazyForge/anvil-hotline),
served at **https://anvil-hotline.hazyforge.io**.

## Direction

- Product face of the open-source Go CLI ("ask and wait" between agents and authorized humans).
- Visual language: **signal red on graphite** (`#0f1117` / `#ff4b4b` / `#ff8a5c` / `#f4f7ff`) — same
  design family as the Anvil Agents and Call Scribe sites, distinct palette.
- Hero: a Grok Imagine film (cool-ice agent beam halted at a red signal gate, warm human lamp across
  the divide) playing full-bleed behind the copy, poster fallback for reduced-motion/data-saver users.

## Local development

```bash
cd site
pnpm install
pnpm dev
```

## Container

Build from the **repository root**:

```bash
docker build -f site/Dockerfile -t ghcr.io/hazyforge/anvil-hotline-site:dev .
docker run --rm -p 8080:8080 ghcr.io/hazyforge/anvil-hotline-site:dev
```
