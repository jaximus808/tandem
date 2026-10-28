import { Settings } from "lucide-react";
import { SURFACE_ITEMS, type Surface } from "../lib/sidebar";

/* ─────────────────────────────────────────────────────────────────────────────
   WorkspaceNav — the labeled left rail: the canvas's primary navigation.

   Two kinds of things live here, visually separated:

   · Top: the SURFACES — Board, Agents, Documents, Summary — icon + label, one active
     (ink label on an ink/[0.06] wash, plus a 2px accent bar at the rail edge —
     DESIGN.md v3 nav/chrome). Clicking
     switches the whole content area; it never toggles panels.
   · Bottom: Settings — secondary chrome. It opens/closes the settings side
     panel (a toggle, hence aria-pressed), styled quieter than the surfaces.

   Desktop-only (`hidden sm:flex`); MobileNavDrawer mirrors it on phones.
   ──────────────────────────────────────────────────────────────────────────── */

export default function WorkspaceNav({
  surface,
  onSelectSurface,
  boardBadge = 0,
  settingsOpen,
  onToggleSettings,
}: {
  surface: Surface;
  onSelectSurface: (s: Surface) => void;
  /** Count bubble on Board (tasks awaiting your approval). */
  boardBadge?: number;
  /** Whether the settings side panel is showing (lights the gear). */
  settingsOpen: boolean;
  onToggleSettings: () => void;
}) {
  return (
    <nav
      aria-label="Canvas"
      className="z-20 hidden w-[4.25rem] shrink-0 flex-col border-r border-ink/10 bg-paper py-2 sm:flex"
    >
      <div className="flex flex-col gap-1">
        {SURFACE_ITEMS.map((item) => {
          const Icon = item.icon;
          const isActive = surface === item.id;
          const badge = item.id === "board" ? boardBadge : 0;
          return (
            <div key={item.id} className="relative">
              {isActive && (
                <span className="absolute left-0 top-1/2 h-8 w-0.5 -translate-y-1/2 bg-accent" />
              )}
              <button
                onClick={() => onSelectSurface(item.id)}
                aria-current={isActive ? "page" : undefined}
                className={[
                  "relative mx-1.5 flex w-[calc(100%-0.75rem)] flex-col items-center gap-1 rounded py-2 transition-colors",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40",
                  isActive
                    ? "bg-ink/[0.06] text-ink"
                    : "text-ink/50 hover:bg-ink/[0.05] hover:text-ink",
                ].join(" ")}
              >
                <span className="relative">
                  <Icon size={19} strokeWidth={isActive ? 2 : 1.75} />
                  {badge > 0 && (
                    <span className="absolute -right-2 -top-1.5 flex h-4 min-w-4 items-center justify-center rounded-sm bg-state-proposed px-1 text-[10px] font-semibold tabular-nums text-paper">
                      {badge}
                    </span>
                  )}
                </span>
                <span
                  className={[
                    "text-[10px] leading-none tracking-tight",
                    isActive ? "font-semibold" : "font-medium",
                  ].join(" ")}
                >
                  {item.label}
                </span>
              </button>
            </div>
          );
        })}
      </div>

      <div className="mt-auto flex flex-col">
        <button
          onClick={onToggleSettings}
          title="Canvas settings"
          aria-label="Canvas settings"
          aria-pressed={settingsOpen}
          className={[
            "mx-1.5 flex flex-col items-center gap-1 rounded py-2 transition-colors",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40",
            settingsOpen
              ? "bg-ink/[0.06] text-ink"
              : "text-ink/50 hover:bg-ink/[0.05] hover:text-ink",
          ].join(" ")}
        >
          <Settings size={18} strokeWidth={1.75} />
          <span className="text-[10px] font-medium leading-none tracking-tight">Settings</span>
        </button>
      </div>
    </nav>
  );
}
