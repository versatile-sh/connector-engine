import { useQuery } from "@tanstack/react-query";
import { sessionQuery } from "../queries";
import { viewerOf, type Viewer } from "./viewer";

/** The signed-in viewer, from the session the console already holds. */
export function useViewer(): Viewer {
  return viewerOf(useQuery(sessionQuery).data);
}
