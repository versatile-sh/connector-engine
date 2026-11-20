import { useMutation } from "@tanstack/react-query";
import { errorText, type Connector } from "../api";
import { setItemAccess } from "../mutations";
import { ItemSections } from "./ItemSections";

/** An installed connector's instructions, prompts and resources, each with its choice. */
export function ConnectorItems({ connector }: { connector: Connector }) {
  const setAccess = useMutation(setItemAccess);
  const pending = (name: string) => setAccess.isPending && setAccess.variables.name === name;
  if ((connector.items ?? []).length === 0) return null;
  return (
    <>
      <p role="alert" className="text-sm text-amber-400 empty:hidden">
        {setAccess.isError ? errorText(setAccess.error) : ""}
      </p>
      <ItemSections
        items={connector.items}
        variant="page"
        accessOf={(item) => (pending(item.name) ? setAccess.variables?.access ?? item.access : item.access)}
        pendingOf={(item) => pending(item.name)}
        choose={(item, access) => setAccess.mutate({ connector: connector.id, name: item.name, access })}
      />
    </>
  );
}
