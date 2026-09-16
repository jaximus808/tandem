interface Props {
  size?: number;
  animate?: boolean;
  className?: string;
  /** Mark color — teal on light backgrounds (default), "#FFFFFF" on dark. */
  color?: string;
}

// The Tandem mark: two riders hitched to one orbit — a solid bead (the agent)
// and an outlined bead (the human) on a shared path. Geometry comes straight
// from assets/svg/tandem-mark-*.svg (source of truth). When `animate` is on,
// the mark assembles once on mount (keyframes in app/globals.css).
export default function TandemLogo({
  size = 64,
  animate = true,
  className = "",
  color = "#0D6E66",
}: Props) {
  const a = animate;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 120 120"
      fill="none"
      className={className}
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <g>
        <path
          className={a ? "tandem-arc tandem-arc-1" : undefined}
          pathLength={100}
          d="M85.3 37.3 A34 34 0 0 1 52.9 93.3"
          stroke={color}
          strokeWidth="8"
          strokeLinecap="butt"
        />
        <path
          className={a ? "tandem-arc tandem-arc-2" : undefined}
          pathLength={100}
          d="M34.7 82.7 A34 34 0 0 1 67.1 26.7"
          stroke={color}
          strokeWidth="8"
          strokeLinecap="butt"
        />
        <line
          className={a ? "tandem-arc tandem-bar" : undefined}
          pathLength={100}
          x1="48.6"
          y1="79.5"
          x2="71.4"
          y2="40.5"
          stroke={color}
          strokeWidth="4.5"
          strokeLinecap="round"
        />
        {/* agent bead — solid */}
        <circle className={a ? "tandem-bead tandem-bead-agent" : undefined} cx="77" cy="31" r="11" fill={color} />
        {/* human bead — outlined */}
        <circle
          className={a ? "tandem-bead tandem-bead-human" : undefined}
          cx="43"
          cy="89"
          r="11"
          stroke={color}
          strokeWidth="6"
        />
      </g>
    </svg>
  );
}
