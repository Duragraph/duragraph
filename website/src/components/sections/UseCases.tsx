import { Rag, Pipelines, Microscope, DevicesApps } from '@carbon/icons-react';
import './UseCases.css';

const UseCases = () => {
  const useCases = [
    {
      icon: <Rag size={48} />,
      title: 'AI Agents',
      description: 'Build autonomous agents with memory, tool use, and complex reasoning',
      color: 'red',
    },
    {
      icon: <Pipelines size={48} />,
      title: 'Data Pipelines',
      description: 'Orchestrate ETL workflows with retries, monitoring, and fault tolerance',
      color: 'red',
    },
    {
      icon: <Microscope size={48} />,
      title: 'Research Labs',
      description: 'Run reproducible experiments with full observability and lineage tracking',
      color: 'red',
    },
    {
      icon: <DevicesApps size={48} />,
      title: 'Enterprise Apps',
      description: 'Power mission-critical workflows with SLA-backed reliability',
      color: 'red',
    },
  ];

  return (
    <div className="use-cases">
      <h2 className="section-title">Use Cases</h2>
      <div className="use-cases-container">
        {useCases.map((useCase, index) => (
          <div
            key={index}
            className={`use-case-section
              ${index % 2 === 0 ? 'divider-right' : ''}
              ${index < 2 ? 'divider-bottom' : ''}`}
          >
            <div className={`use-case-icon icon-${useCase.color}`}>
              {useCase.icon}
            </div>
            <h3 className="use-case-title">{useCase.title}</h3>
            <p className="use-case-description">{useCase.description}</p>
          </div>
        ))}
      </div>
    </div>
  );
};

export default UseCases;
