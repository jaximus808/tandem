"use client";

import { useEffect, useState } from "react";

export type DocsSection = { id: string; title: string; children?: { id: string; title: string }[] };

/** The section list, rendered twice: a sticky sidebar on wide screens and a
 *  collapsible list above the content on narrow ones (CSS shows one). The
 *  heading nearest the top of the viewport is marked current. */
export default function DocsNav({ sections }: { sections: DocsSection[] }) {
  const [active, setActive] = useState<string>(sections[0]?.id ?? "");
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const ids = sections.flatMap((s) => [s.id, ...(s.children ?? []).map((c) => c.id)]);
    const headings = ids.map((id) => document.getElementById(id)).filter((el): el is HTMLElement => el !== null);
    let frame = 0;
    const update = () => {
      frame = 0;
      // Past the end of the page every heading is above the fold, so the last
      // one wins; otherwise the last heading that has crossed the top line.
      let current = headings[0]?.id ?? "";
      for (const el of headings) {
        if (el.getBoundingClientRect().top <= 112) current = el.id;
        else break;
      }
      setActive(current);
    };
    const onScroll = () => {
      if (!frame) frame = window.requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      if (frame) window.cancelAnimationFrame(frame);
    };
  }, [sections]);

  const activeSection = sections.find((s) => s.id === active || s.children?.some((c) => c.id === active))?.id;

  const list = (
    <ul className="docs-nav-list">
      {sections.map((s) => (
        <li key={s.id}>
          <a href={`#${s.id}`} aria-current={active === s.id ? "location" : undefined} onClick={() => setOpen(false)}>
            {s.title}
          </a>
          {s.children && s.children.length > 0 && (
            <ul className={activeSection === s.id ? "is-open" : undefined}>
              {s.children.map((c) => (
                <li key={c.id}>
                  <a
                    href={`#${c.id}`}
                    aria-current={active === c.id ? "location" : undefined}
                    onClick={() => setOpen(false)}
                  >
                    {c.title}
                  </a>
                </li>
              ))}
            </ul>
          )}
        </li>
      ))}
    </ul>
  );

  return (
    <>
      <aside className="docs-side">
        <nav aria-label="Sections">{list}</nav>
      </aside>
      <details className="docs-side-mobile" open={open} onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
        <summary>
          <span>On this page</span>
          <span className="docs-side-current">{sections.find((s) => s.id === activeSection)?.title}</span>
        </summary>
        <nav aria-label="Sections">{list}</nav>
      </details>
    </>
  );
}
