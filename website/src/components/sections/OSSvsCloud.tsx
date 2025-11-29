import { Checkmark } from '@carbon/icons-react';
import './OSSvsCloud.css';

const OSSvsCloud = () => {
  const ossFeatures = [
    'Apache-licensed, self-host',
    'Core orchestration engine',
    'Multi-language SDKs',
    'LangGraph compatibility',
    'Community support',
  ];

  const cloudFeatures = [
    'Managed hosting & scaling',
    'Enterprise security & RBAC',
    'Advanced monitoring & alerts',
    'SLA-backed uptime',
  ];

  return (
    <div className="oss-vs-cloud">
      <h2 className="section-title">Open-Source Core + Cloud Upgrade</h2>
      <div className="comparison-container">
        <div className="comparison-section oss-section">
          <div className="section-header">
            <h3 className="card-title">DuraGraph OSS</h3>
            <div className="accent-bar accent-indigo"></div>
          </div>
          <ul className="feature-list">
            {ossFeatures.map((feature, index) => (
              <li key={index} className="feature-item">
                <Checkmark size={20} className="check-icon indigo" />
                <span>{feature}</span>
              </li>
            ))}
          </ul>
        </div>

        <div className="comparison-divider"></div>

        <div className="comparison-section cloud-section">
          <div className="section-header">
            <h3 className="card-title">DuraGraph Cloud</h3>
            <div className="accent-bar accent-cyan"></div>
          </div>
          <ul className="feature-list">
            {cloudFeatures.map((feature, index) => (
              <li key={index} className="feature-item">
                <Checkmark size={20} className="check-icon cyan" />
                <span>{feature}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  );
};

export default OSSvsCloud;
