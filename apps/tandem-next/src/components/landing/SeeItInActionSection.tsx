"use client";

import { useState } from "react";
import { Eyebrow } from "@/components/ui";
import { usePrefersReducedMotion } from "./HeroBoardDemo";

/**
 * SeeItInActionSection — the recorded demo, two beats: 01 connect (an agent
 * joins, proposes, and one approval releases the batch) and 02 steer (the plan
 * changes while the fleet is mid-run). The full-demo CTA at the foot opens the
 * same FullDemoModal the hero CTA does.
 */

function DemoClip({ src, poster, caption }: { src: string; poster: string; caption: string }) {
  const reduced = usePrefersReducedMotion();
  const [failed, setFailed] = useState(false);
  if (failed) return null;
  return (
    <figure>
      <div className="overflow-hidden rounded-lg border border-ink/10 bg-surface">
        <video
          ref={(el) => {
            if (el) el.muted = true;
          }}
          className="block h-auto w-full"
          src={src}
          poster={poster}
          preload="metadata"
          muted
          playsInline
          autoPlay={!reduced}
          loop={!reduced}
          controls={reduced}
          aria-label={caption}
          onError={() => setFailed(true)}
        />
      </div>
      <figcaption className="mt-3 max-w-2xl text-sm leading-relaxed text-ink/60">{caption}</figcaption>
    </figure>
  );
}

function BeatHeader({ n, title, lede }: { n: string; title: string; lede: string }) {
  return (
    <div className="mb-5 flex items-start gap-5 sm:gap-7">
      <span className="grid h-9 w-9 shrink-0 place-items-center rounded-md border border-ink/15 bg-surface font-code text-[12px] font-medium text-ink/70 sm:h-10 sm:w-10">
        {n}
      </span>
      <div className="min-w-0 pt-1">
        <h3 className="text-xl font-semibold tracking-tight text-ink">{title}</h3>
        <p className="mt-1 text-sm leading-relaxed text-ink/65">{lede}</p>
      </div>
    </div>
  );
}

export default function SeeItInActionSection({ onWatchFull }: { onWatchFull?: () => void }) {
  return (
    <section id="see-it-in-action" className="mx-auto max-w-6xl px-6 py-24">
      <div className="max-w-2xl">
        <Eyebrow>See it in action</Eyebrow>
        <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[2rem]">
          The loop above, running for real
        </h2>
        <p className="mt-3 leading-relaxed text-ink/65">
          Two unedited screen recordings: a fleet picking up a plan, and the plan changing under it.
          No mock data — this is Tandem planning Tandem.
        </p>
      </div>

      <div className="mt-14 max-w-4xl">
        <BeatHeader
          n="01"
          title="Connect, propose, approve"
          lede="One MCP connect, a proposed epic, one approval on the board — and the whole batch turns pullable, with workers claiming tickets seconds later."
        />
        <DemoClip
          src="/demo/queue-loop.mp4"
          poster="/demo/queue-loop-poster.jpg"
          caption="An agent connects, proposes a batch, and waits. The approval is the go signal — no second prompt, no copy-pasting task lists between sessions."
        />
      </div>

      <div className="mt-16 max-w-4xl">
        <BeatHeader n="02" title="Steer mid-run" lede="You change your mind while the fleet is working — and nothing has to stop." />
        <DemoClip
          src="/demo/steer-loop.mp4"
          poster="/demo/steer-loop-poster.jpg"
          caption="Reject a ticket, amend the plan, add work mid-batch. Sessions already in flight pick the change up on their next pull — you don't stop the fleet to steer it."
        />
      </div>

      {onWatchFull && (
        <div className="mt-12 max-w-4xl">
          <button
            onClick={onWatchFull}
            className="inline-flex items-center gap-2 rounded-md border border-ink/15 bg-surface px-4 py-2.5 text-sm font-medium text-ink transition-colors hover:border-ink/25"
          >
            Watch the full demo
            <span className="font-code text-[11px] text-ink/50">4 min · with audio</span>
          </button>
        </div>
      )}
    </section>
  );
}
