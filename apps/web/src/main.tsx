import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { LocaleProvider } from "./i18n/LocaleContext";
import "./styles.css";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("missing #root element");
}

createRoot(rootElement).render(
  <StrictMode>
    <LocaleProvider>
      <App />
    </LocaleProvider>
  </StrictMode>,
);
