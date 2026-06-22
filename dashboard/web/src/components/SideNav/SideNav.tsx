import { NavLink, useLocation } from "react-router-dom";
import { navItems } from "../../routes/nav";
import { NavIconSvg } from "./NavIconSvg";
import "./SideNav.css";

function isNavItemActive(path: string, pathname: string): boolean {
  if (path === "/") {
    return pathname === "/";
  }
  if (path === "/sandboxes/monitoring") {
    return pathname === "/sandboxes/monitoring";
  }
  if (path === "/sandboxes") {
    return (
      pathname === "/sandboxes" ||
      (pathname.startsWith("/sandboxes/") && pathname !== "/sandboxes/monitoring")
    );
  }
  return pathname === path || pathname.startsWith(`${path}/`);
}

export function SideNav() {
  const { pathname } = useLocation();

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
              className={() =>
                `side-nav__link${
                  isNavItemActive(item.path, pathname)
                    ? " side-nav__link--active"
                    : ""
                }`
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
