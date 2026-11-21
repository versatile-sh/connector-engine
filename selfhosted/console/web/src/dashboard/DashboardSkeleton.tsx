import { SKELETON, SURFACE } from "../ui/styles";

const COLUMNS = "grid gap-6 @4xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]";

/** The dashboard's shape while its first read is on its way: four stat cards, the chart over a list, a list over a list; it is drawn once, never again for a scope change or a refresh. */
export function DashboardSkeleton() {
  return (
    <div aria-hidden="true" className="@container flex flex-col gap-6">
      <div className="grid gap-6 @xl:grid-cols-2 @4xl:grid-cols-4">
        {[0, 1, 2, 3].map((card) => (
          <div key={card} className={`${SURFACE} min-h-36`}>
            <div className={`${SKELETON} m-5 h-24`} />
          </div>
        ))}
      </div>
      <div className={COLUMNS}>
        <div className={`${SKELETON} h-80`} />
        <div className={`${SKELETON} h-80`} />
      </div>
      <div className={COLUMNS}>
        <div className={`${SKELETON} h-96`} />
        <div className={`${SKELETON} h-96`} />
      </div>
    </div>
  );
}
