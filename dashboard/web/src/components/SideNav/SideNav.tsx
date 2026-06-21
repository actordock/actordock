import { NavLink } from "react-router-dom";
import { navItems } from "../../routes/nav";
import { NavIconSvg } from "./NavIconSvg";
import "./SideNav.css";

export function SideNav() {
  return (
    <nav className="side-nav" aria-label="Main navigation">
      <div className="side-nav__brand">
        <span className="side-nav__brand-mark" aria-hidden="true" />
        <span className="side-nav__brand-text">Actordock</span>
      </div>
      <ul className="side-nav__list">
        {navItems.map((item) => (
          <li key={item.path}>
            <NavLink
              to={item.path}
              end={item.path === "/"}
              className={({ isActive }) =>
                `side-nav__link${isActive ? " side-nav__link--active" : ""}`
              }
            >
              <NavIconSvg name={item.icon} className="side-nav__icon" />
              <span>{item.label}</span>
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
