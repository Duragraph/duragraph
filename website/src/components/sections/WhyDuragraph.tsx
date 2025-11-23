import { Security, Code, CloudApp } from '@carbon/icons-react';
import './WhyDuragraph.css';

const WhyDuragraph = () => {
  const features = [
    {
      icon: <Security size={48} />,
      title: 'Resilient',
      description: 'Fault-tolerant, stateful workflows',
      color: 'success',
    },
    {
      icon: <Code size={48} />,
      title: 'Developer-Friendly',
      description: 'Graph APIs, LangGraph compatible, multi-language SDKs',
      color: 'success',
    },
    {
      icon: <CloudApp size={48} />,
      title: 'Cloud-Scale',
      description: 'Managed hosting, real-time monitoring, OpenTelemetry tracing',
      color: 'success',
    },
  ];

  return (
    <div className="why-duragraph">
      <h2 className="section-title">Why DuraGraph</h2>
      <div className="features-container">
        {features.map((feature, index) => (
          <div key={index} className={`feature-section ${index !== features.length - 1 ? 'feature-divider' : ''}`}>
            <div className={`feature-icon icon-${feature.color}`}>
              {feature.icon}
            </div>
            <h3 className="feature-title">{feature.title}</h3>
            <p className="feature-description">{feature.description}</p>
          </div>
        ))}
      </div>
    </div>
  );
};

export default WhyDuragraph;
