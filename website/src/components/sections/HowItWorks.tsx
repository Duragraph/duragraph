import './HowItWorks.css';

const HowItWorks = () => {
  const steps = [
    {
      number: '01',
      title: 'Define Graph',
      description: 'Design your workflow as a graph with nodes and edges',
      color: 'indigo',
    },
    {
      number: '02',
      title: 'Generate Code',
      description: 'Auto-generate type-safe workflow code from your graph',
      color: 'cyan',
    },
    {
      number: '03',
      title: 'Run Workflows',
      description: 'Execute workflows with full observability and fault tolerance',
      color: 'coral',
    },
    {
      number: '04',
      title: 'Monitor',
      description: 'Track execution with real-time metrics and OpenTelemetry',
      color: 'success',
    },
  ];

  return (
    <div className="how-it-works">
      <h2 className="section-title">How It Works</h2>
      <div className="steps-container">
        {steps.map((step, index) => (
          <div key={index} className={`step-section ${index !== steps.length - 1 ? 'step-divider' : ''}`}>
            <div className={`step-number step-number-${step.color}`}>{step.number}</div>
            <h3 className="step-title">{step.title}</h3>
            <p className="step-description">{step.description}</p>
          </div>
        ))}
      </div>
    </div>
  );
};

export default HowItWorks;
