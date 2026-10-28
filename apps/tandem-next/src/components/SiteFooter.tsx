import { AUTHOR_URL, NPM_URL, REPO_URL } from "@/lib/site";

const ext = { target: "_blank", rel: "noopener noreferrer" } as const;

export default function SiteFooter() {
  return (
    <footer className="footer">
      <div className="container">
        <p>
          Built by{" "}
          <a href={AUTHOR_URL} {...ext}>
            Jaxon Poentis
          </a>
          . MIT licensed.
        </p>
        <ul>
          <li>
            <a href="/docs">Docs</a>
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
            <a href={`${REPO_URL}/blob/main/LICENSE`} {...ext}>
              License
            </a>
          </li>
        </ul>
      </div>
    </footer>
  );
}
