import { Button } from '@carbon/react';
import { LogoGithub, DocumentBlank } from '@carbon/icons-react';
import './Community.css';

const Community = () => {
  return (
    <div className="community-section">
      <h2 className="section-title">Community + Docs</h2>
      <p className="community-subtitle">
        Join our community, contribute to the project, and explore the documentation
      </p>
      <div className="community-buttons">
        <Button
          kind="tertiary"
          size="lg"
          renderIcon={LogoGithub}
          onClick={() => window.open('https://github.com/duragraph/duragraph', '_blank')}
        >
          Star on GitHub
        </Button>
        <Button
          kind="tertiary"
          size="lg"
          renderIcon={DocumentBlank}
          onClick={() => window.open('#', '_blank')}
        >
          Documentation
        </Button>
      </div>
    </div>
  );
};

export default Community;
