import { Header as CarbonHeader, HeaderName, HeaderNavigation, HeaderMenuItem, HeaderGlobalBar, HeaderGlobalAction } from '@carbon/react';
import { LogoGithub, Asleep, Light } from '@carbon/icons-react';
import { useTheme } from '../../context/ThemeContext';
import './Header.css';

const Header = () => {
  const { theme, toggleTheme } = useTheme();

  return (
    <CarbonHeader aria-label="DuraGraph">
      <HeaderName href="#" prefix="">
        DuraGraph
      </HeaderName>
      <HeaderNavigation aria-label="DuraGraph">
        <HeaderMenuItem href="#why">Why DuraGraph</HeaderMenuItem>
        <HeaderMenuItem href="#how">How It Works</HeaderMenuItem>
        <HeaderMenuItem href="#cloud">Cloud</HeaderMenuItem>
        <HeaderMenuItem href="#use-cases">Use Cases</HeaderMenuItem>
        <HeaderMenuItem href="#community">Community</HeaderMenuItem>
      </HeaderNavigation>
      <HeaderGlobalBar>
        <HeaderGlobalAction
          aria-label={`Switch to ${theme === 'light' ? 'dark' : 'light'} mode`}
          onClick={toggleTheme}
        >
          {theme === 'light' ? <Asleep size={20} /> : <Light size={20} />}
        </HeaderGlobalAction>
        <HeaderGlobalAction
          aria-label="GitHub"
          onClick={() => window.open('https://github.com/duragraph/duragraph', '_blank')}
        >
          <LogoGithub size={20} />
        </HeaderGlobalAction>
      </HeaderGlobalBar>
    </CarbonHeader>
  );
};

export default Header;
