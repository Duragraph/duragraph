import type { ReactNode } from 'react';
import './Section.css';

interface SectionProps {
  id?: string;
  className?: string;
  children: ReactNode;
  background?: 'default' | 'dark' | 'grid';
}

const Section = ({ id, className = '', children, background = 'default' }: SectionProps) => {
  const backgroundClass = background === 'grid' ? 'grid-pattern' : background === 'dark' ? 'section-dark' : '';

  return (
    <section id={id} className={`duragraph-section ${backgroundClass} ${className}`}>
      <div className="section-container">
        {children}
      </div>
    </section>
  );
};

export default Section;
