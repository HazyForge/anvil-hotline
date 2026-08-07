const GITHUB = "https://github.com/HazyForge/anvil-hotline";

export default function SiteFooter() {
  return (
    <footer className="site-footer">
      <div className="container site-footer-inner">
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
  );
}
