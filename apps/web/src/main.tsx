import React, { useEffect, useState } from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { ConfigContext, loadConfig, type AppConfig } from "./lib/config";
// Inter (variable) — the one UI + display face (Design v2). Self-hosted via
// fontsource so the app doesn't depend on Google Fonts for its primary face.
import "@fontsource-variable/inter";
import "./index.css";

// The boot gate: nothing renders until /api/config has answered, so a local
// server never flashes the hosted landing page (or a sign-in button) before the
// app learns it is local. The wait is one same-origin request against a local
// process — and on failure it falls over to the hosted shape, so an old server
// without the route still gets exactly the app it always had.
function ConfigProvider({ children }: { children: React.ReactNode }) {
  const [config, setConfig] = useState<AppConfig | null>(null);
  useEffect(() => {
    let cancelled = false;
    loadConfig().then((cfg) => {
      if (!cancelled) setConfig(cfg);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  if (!config) {
    // Neutral shell: the page background only. Sub-100ms locally.
    return <div className="h-app bg-paper" aria-busy="true" />;
  }
  return <ConfigContext.Provider value={config}>{children}</ConfigContext.Provider>;
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ConfigProvider>
      <App />
    </ConfigProvider>
  </React.StrictMode>
);
