import { NPM_URL, RELEASES_URL, REPO_URL } from "@/lib/site";

const ext = { target: "_blank", rel: "noopener noreferrer" } as const;

/** The one nav both pages share: mark + name on the left, Docs then the
 *  external links on the right. `current` marks the page we are on. */
export default function SiteNav({ current }: { current?: "docs" }) {
  return (
    <header className="nav">
      <div className="container">
        <a className="brand" href="/" aria-label="Tandem home">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/tandem-mark.svg" alt="" width={24} height={24} />
          Tandem
        </a>
        <ul className="nav-links">
          <li>
            <a href="/docs" aria-current={current === "docs" ? "page" : undefined}>
              Docs
            </a>
          </li>
          <li>
            <a href={REPO_URL} {...ext}>
              GitHub
            </a>
          </li>
          <li>
            <a href={NPM_URL} {...ext}>
              npm
            </a>
          </li>
          <li>
            <a href={RELEASES_URL} {...ext}>
              Releases
            </a>
          </li>
        </ul>
      </div>
    </header>
  );
}
