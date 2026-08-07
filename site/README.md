# Anvil Hotline site

Marketing site for [Anvil Hotline](https://github.com/HazyForge/anvil-hotline), served at
**https://anvil-hotline.hazyforge.io**.

## Direction (Ask Once redesign)

- Dark terminal-session theme: prompt bar, split hero (headline + live terminal window with the
  ask/wait/answer session), protocol rules, reply matrix (typed vs emoji), failure/audit rows, install.
- Bricolage Grotesque + Fira Code. Palette: terminal black #090B0D, command green #A6FF4D,
  amber #FFC857, violet #7B61FF.
- Hero film: gpt-image-1 terminal still + Grok image-to-video (locked-off macro, typing lines, keycap press).

## Local / container

```bash
cd site && pnpm install && pnpm dev
docker build -f site/Dockerfile -t ghcr.io/hazyforge/anvil-hotline-site:dev .
```
