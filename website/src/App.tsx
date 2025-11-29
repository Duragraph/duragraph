import Header from './components/layout/Header';
import Footer from './components/layout/Footer';
import Section from './components/layout/Section';
import Hero from './components/sections/Hero';
import WhyDuragraph from './components/sections/WhyDuragraph';
import HowItWorks from './components/sections/HowItWorks';
import OSSvsCloud from './components/sections/OSSvsCloud';
import UseCases from './components/sections/UseCases';
import Community from './components/sections/Community';
import './App.css';

function App() {
  return (
    <div className="app">
      <Header />

      <main>
        <Section background="grid">
          <Hero />
        </Section>

        <Section id="why" background="dark">
          <WhyDuragraph />
        </Section>

        <Section id="how">
          <HowItWorks />
        </Section>

        <Section id="cloud" background="dark">
          <OSSvsCloud />
        </Section>

        <Section id="use-cases">
          <UseCases />
        </Section>

        <Section id="community" background="dark">
          <Community />
        </Section>
      </main>

      <Footer />
    </div>
  );
}

export default App;
