import type { ReactNode } from "react";
import "./MarbleCard.css";

type MarbleCardProps = {
  title?: string;
  children: ReactNode;
  className?: string;
  footer?: ReactNode;
};

export function MarbleCard({
  title,
  children,
  className = "",
  footer,
}: MarbleCardProps) {
  return (
    <section className={`marble-card ${className}`.trim()}>
      {title ? <header className="marble-card__header">{title}</header> : null}
      <div className="marble-card__body">{children}</div>
      {footer ? <footer className="marble-card__footer">{footer}</footer> : null}
    </section>
  );
}
