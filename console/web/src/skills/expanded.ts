/** The id of the skill whose card is maximized: `?skill=` when that skill is installed, otherwise none. */
export const expandedOf = (query: URLSearchParams, skills: readonly { id: string }[]) => skills.find((skill) => skill.id === query.get("skill"))?.id;

/** The Skills page's address with `id`'s card maximized, or every card compact without `id`, keeping the rest of the query. */
export function skillsHref(query: URLSearchParams, id?: string): string {
  const next = new URLSearchParams(query);
  if (id) next.set("skill", id);
  else next.delete("skill");
  const search = next.toString();
  return search ? `/skills?${search}` : "/skills";
}

/** The card maximized once `id`'s button is pressed: that card, or none when it was the maximized one. */
export const toggled = (open: string | undefined, id: string) => (open === id ? undefined : id);
