import { PageHeader } from "../components";

type PlaceholderPageProps = {
  title: string;
  subtitle: string;
};

export function PlaceholderPage({ title, subtitle }: PlaceholderPageProps) {
  return (
    <PageHeader title={title} subtitle={subtitle} />
  );
}
