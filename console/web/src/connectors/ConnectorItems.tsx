import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, errorText, type Access, type Connector } from "../api";
import { ItemSections } from "./ItemSections";

/** An installed connector's instructions, prompts and resources, each choice saved by item name in the body since names hold `/` and `:`. */
export function ConnectorItems({ connector }: { connector: Connector }) {
  const queryClient = useQueryClient();
  const setAccess = useMutation({
    mutationFn: ({ name, access }: { name: string; access: Access }) => api("PUT", `/api/connectors/${encodeURIComponent(connector.id)}/items`, { name, access }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["connectors"] }),
  });
  const pending = (name: string) => setAccess.isPending && setAccess.variables.name === name;
  if ((connector.items ?? []).length === 0) return null;
  return (
    <>
      <p role="alert" className="text-sm text-amber empty:hidden">
        {setAccess.isError ? errorText(setAccess.error) : ""}
      </p>
      <ItemSections
        items={connector.items}
        variant="page"
        accessOf={(item) => (pending(item.name) ? setAccess.variables?.access ?? item.access : item.access)}
        pendingOf={(item) => pending(item.name)}
        choose={(item, access) => setAccess.mutate({ name: item.name, access })}
      />
    </>
  );
}
