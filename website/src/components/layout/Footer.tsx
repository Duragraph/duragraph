import './Footer.css';

const Footer = () => {
  return (
    <footer className="duragraph-footer">
      <div className="footer-content">
        <div className="footer-brand">
          <h3>DuraGraph</h3>
          <p className="footer-tagline">Durable Workflows. Open Source. Cloud Ready.</p>
        </div>
        <div className="footer-license">
          <p>
            <strong>DuraGraph Core</strong> is Apache 2.0 licensed.{' '}
            <strong>DuraGraph Cloud</strong> is enterprise-ready hosting.
          </p>
        </div>
        <div className="footer-links">
          <a href="#" className="footer-link">Docs</a>
          <a href="https://github.com/duragraph/duragraph" target="_blank" rel="noopener noreferrer" className="footer-link">GitHub</a>
          <a href="#cloud" className="footer-link">Cloud</a>
          <a href="#" className="footer-link">Blog</a>
        </div>
      </div>
      <div className="footer-copyright">
        <p>&copy; {new Date().getFullYear()} DuraGraph. All rights reserved.</p>
      </div>
    </footer>
  );
};

export default Footer;
