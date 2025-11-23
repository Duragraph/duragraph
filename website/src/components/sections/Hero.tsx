import { Button } from '@carbon/react';
import './Hero.css';

const Hero = () => {
  return (
    <div className="hero-section">
      <div className="hero-content">
        <h1 className="hero-title">
          Durable Workflows. Open Source. Cloud Ready.
        </h1>
        <p className="hero-subtitle">
          Duragraph is an open-core orchestration platform for AI and data workflows — self-host or run on Duragraph Cloud.
        </p>
        <div className="hero-cta">
          <Button size="lg">
            Get Started (OSS)
          </Button>
          <Button size="lg" kind="secondary">
            Try Duragraph Cloud
          </Button>
        </div>
      </div>
    </div>
  );
};

export default Hero;
