/** A title whose `*marked*` words are drawn in lime, as on versatile.sh; each dictionary places the mark where its own word order puts it. */
export function Marked({ text }: { text: string }) {
  return text.split("*").map((part, index) => (index % 2 === 1 ? <span key={`${index}-${part}`} className="text-lime">{part}</span> : part));
}
