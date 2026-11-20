/** The id of the skill open in its dialog: `?skill=` when that skill is installed, otherwise none. */
export const expandedOf = (query: URLSearchParams, skills: readonly { id: string }[]) => skills.find((skill) => skill.id === query.get("skill"))?.id;

/** The Skills page's address with `id`'s dialog open, or every dialog closed without `id`, keeping the rest of the query. */
export function skillsHref(query: URLSearchParams, id?: string): string {
  const next = new URLSearchParams(query);
  if (id) next.set("skill", id);
  else next.delete("skill");
  const search = next.toString();
  return search ? `/skills?${search}` : "/skills";
}
