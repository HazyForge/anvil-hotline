# Anvil Hotline site

Marketing site for Anvil Hotline — **https://anvil-hotline.hazyforge.io**.

## Direction (Semaphore Station)

Poster-flat maritime signal-flag theme: yardarm header, yellow hoist hero with flag-hoist video plate,
flat flag cards (HOIST/HOLD/REPLY/LOWER), reply signals (typed vs pennant), authorization manifest,
navy rigging install, footer. Anton + Hanken Grotesk + Red Hat Mono. Palette #F4EFE3 / #C8102E / #FFC72C / #00308B.

## Local / container

```bash
cd site && pnpm install && pnpm dev
docker build -f site/Dockerfile -t ghcr.io/hazyforge/anvil-hotline-site:dev .
```
